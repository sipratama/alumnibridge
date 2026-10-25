package postgres

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/sipratama/alumnibridge/apps/api/internal/profiles"
)

// A single snapshot supplies eligibility, current consent and the owner's choices.
const discoverySQL = `SELECT p.directory_discoverable,p.directory_consented_at,p.privacy_revision,
 p.visibility='public' AND p.moderation_status='visible' AND EXISTS (
    SELECT 1 FROM memberships WHERE user_id=p.user_id AND role='alumni' AND status='verified'),
 COALESCE((SELECT jsonb_agg(jsonb_build_object('membership_id',m.id,'institution',i.slug,
    'study_program',sp.slug,'graduation_year',m.graduation_year,
    'show_institution',COALESCE(a.show_institution,false),'show_study_program',COALESCE(a.show_study_program,false),
    'show_graduation_year',COALESCE(a.show_graduation_year,false)) ORDER BY i.slug COLLATE "C",sp.slug COLLATE "C")
    FROM memberships m JOIN study_programs sp ON sp.id=m.study_program_id JOIN institutions i ON i.id=sp.institution_id
    LEFT JOIN profile_directory_affiliations a ON a.membership_id=m.id
    WHERE m.user_id=p.user_id AND m.role='alumni' AND m.status='verified'),'[]')
 FROM profiles p WHERE p.user_id=$1`

func scanDiscovery(row pgx.Row) (profiles.Discovery, error) {
	var out profiles.Discovery
	var memberships []byte
	if err := row.Scan(&out.DirectoryDiscoverable, &out.DirectoryConsentedAt, &out.Revision, &out.Eligible, &memberships); err != nil {
		return out, missing(err)
	}
	if err := json.Unmarshal(memberships, &out.Memberships); err != nil {
		return out, err
	}
	out.EffectiveDiscoverable = out.DirectoryDiscoverable && out.Eligible
	return out, nil
}
func (s *Store) Discovery(ctx context.Context, owner uuid.UUID) (profiles.Discovery, error) {
	return scanDiscovery(s.db.QueryRow(ctx, discoverySQL, owner))
}
func (s *Store) UpdateDiscovery(ctx context.Context, owner uuid.UUID, patch profiles.DiscoveryPatch) (profiles.Discovery, error) {
	return s.updateDiscovery(ctx, owner, patch, 0)
}
func (s *Store) UpdateDiscoveryConditional(ctx context.Context, owner uuid.UUID, patch profiles.DiscoveryPatch, expected int64) (profiles.Discovery, error) {
	if expected < 1 {
		return profiles.Discovery{}, profiles.ErrInvalid
	}
	return s.updateDiscovery(ctx, owner, patch, expected)
}
func (s *Store) updateDiscovery(ctx context.Context, owner uuid.UUID, patch profiles.DiscoveryPatch, expected int64) (profiles.Discovery, error) {
	// Validate at the persistence boundary as well as the service boundary.
	if err := patch.Validate(); err != nil {
		return profiles.Discovery{}, err
	}
	tx, err := s.beginActiveUser(ctx, owner)
	if err != nil {
		return profiles.Discovery{}, err
	}
	defer tx.Rollback(ctx)
	// Membership lifecycle triggers use membership -> profile -> affiliation order.
	rows, err := tx.Query(ctx, `SELECT id FROM memberships WHERE user_id=$1 ORDER BY id FOR UPDATE`, owner)
	if err != nil {
		return profiles.Discovery{}, err
	}
	for rows.Next() {
		var id uuid.UUID
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return profiles.Discovery{}, err
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return profiles.Discovery{}, err
	}
	var visibility, moderation string
	var consent bool
	var revision int64
	if err = tx.QueryRow(ctx, `SELECT visibility,moderation_status,directory_discoverable,privacy_revision FROM profiles WHERE user_id=$1 FOR UPDATE`, owner).Scan(&visibility, &moderation, &consent, &revision); err != nil {
		return profiles.Discovery{}, missing(err)
	}
	if expected > 0 && revision != expected {
		return profiles.Discovery{}, profiles.ErrStale
	}
	var verified bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM memberships WHERE user_id=$1 AND role='alumni' AND status='verified')`, owner).Scan(&verified); err != nil {
		return profiles.Discovery{}, err
	}
	if patch.DirectoryDiscoverable != nil {
		consent = *patch.DirectoryDiscoverable
	}
	if consent && !profiles.DiscoveryEligible(visibility, moderation, verified) {
		return profiles.Discovery{}, profiles.ErrConflict
	}
	if patch.Affiliations != nil {
		// Validate all references before deleting/replacing the existing set.
		for _, a := range *patch.Affiliations {
			var eligible bool
			err = tx.QueryRow(ctx, `SELECT role='alumni' AND status='verified' FROM memberships WHERE id=$1 AND user_id=$2`, a.MembershipID, owner).Scan(&eligible)
			if err != nil {
				return profiles.Discovery{}, missing(err)
			}
			if !eligible {
				return profiles.Discovery{}, profiles.ErrConflict
			}
			if !consent && (a.ShowInstitution || a.ShowStudyProgram || a.ShowGraduationYear) {
				return profiles.Discovery{}, profiles.ErrConflict
			}
		}
	}
	if tag, updateErr := tx.Exec(ctx, `UPDATE profiles SET directory_discoverable=$2,
        directory_consented_at=CASE WHEN $2 THEN COALESCE(directory_consented_at,now()) ELSE NULL END,
        privacy_revision=privacy_revision+1,updated_at=now() WHERE user_id=$1 AND ($3=0 OR privacy_revision=$3)`, owner, consent, expected); updateErr != nil {
		return profiles.Discovery{}, updateErr
	} else if tag.RowsAffected() != 1 {
		return profiles.Discovery{}, profiles.ErrStale
	}
	if patch.Affiliations != nil {
		if _, err = tx.Exec(ctx, `DELETE FROM profile_directory_affiliations a USING memberships m WHERE a.membership_id=m.id AND m.user_id=$1`, owner); err != nil {
			return profiles.Discovery{}, err
		}
		if consent {
			for _, a := range *patch.Affiliations {
				if _, err = tx.Exec(ctx, `INSERT INTO profile_directory_affiliations(membership_id,show_institution,show_study_program,show_graduation_year)
                SELECT id,$3,$4,$5 FROM memberships WHERE id=$1 AND user_id=$2 AND role='alumni' AND status='verified'`, a.MembershipID, owner, a.ShowInstitution, a.ShowStudyProgram, a.ShowGraduationYear); err != nil {
					return profiles.Discovery{}, err
				}
			}
		}
	}
	out, err := scanDiscovery(tx.QueryRow(ctx, discoverySQL, owner))
	if err != nil {
		return out, err
	}
	return out, tx.Commit(ctx)
}
