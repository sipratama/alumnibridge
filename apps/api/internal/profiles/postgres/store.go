package postgres

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sipratama/alumnibridge/apps/api/internal/profiles"
)

type Store struct{ db *pgxpool.Pool }

func New(db *pgxpool.Pool) *Store { return &Store{db: db} }
func dbError(err error) error {
	var p *pgconn.PgError
	if errors.As(err, &p) && p.Code == "23505" {
		return profiles.ErrConflict
	}
	return err
}
func missing(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return profiles.ErrNotFound
	}
	return err
}

func collectionSlot(ctx context.Context, tx pgx.Tx, id uuid.UUID, table string) error {
	if table != "profile_experiences" && table != "profile_educations" && table != "profile_skills" {
		return profiles.ErrInvalid
	}
	var locked uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT id FROM users WHERE id=$1 FOR UPDATE`, id).Scan(&locked); err != nil {
		return missing(err)
	}
	var count int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM `+table+` WHERE user_id=$1`, id).Scan(&count); err != nil {
		return err
	}
	if count >= 100 {
		return profiles.ErrConflict
	}
	return nil
}
func (s *Store) Profile(ctx context.Context, id uuid.UUID) (profiles.Profile, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return profiles.Profile{}, err
	}
	defer tx.Rollback(ctx)
	var p profiles.Profile
	err = tx.QueryRow(ctx, `SELECT COALESCE(u.display_name,''),p.slug,p.headline,p.bio,p.location,p.current_company,p.current_job_title,p.content_revision FROM profiles p JOIN users u ON u.id=p.user_id WHERE p.user_id=$1`, id).Scan(&p.DisplayName, &p.Slug, &p.Headline, &p.Bio, &p.Location, &p.CurrentCompany, &p.CurrentJobTitle, &p.Revision)
	if err == nil {
		return p, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return p, err
	}
	var name string
	if err = tx.QueryRow(ctx, `SELECT COALESCE(display_name,'') FROM users WHERE id=$1 FOR UPDATE`, id).Scan(&name); err != nil {
		return p, missing(err)
	}
	// A second request may have initialized the profile while this transaction waited for the user lock.
	err = tx.QueryRow(ctx, `SELECT slug,headline,bio,location,current_company,current_job_title,content_revision FROM profiles WHERE user_id=$1`, id).Scan(&p.Slug, &p.Headline, &p.Bio, &p.Location, &p.CurrentCompany, &p.CurrentJobTitle, &p.Revision)
	if err == nil {
		p.DisplayName = name
		return p, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return p, err
	}
	base := profiles.NormalizeSlug(name)
	if len(base) > 48 {
		base = strings.Trim(base[:48], "-")
	}
	if !profiles.ValidSlug(base) {
		base = "member"
	}
	for attempt := 0; attempt < 10; attempt++ {
		candidate := base
		if attempt > 0 || base == "member" {
			candidate = base + "-" + uuid.NewString()[:8]
		}
		var owner uuid.UUID
		err = tx.QueryRow(ctx, `INSERT INTO profile_slug_history(slug,user_id) VALUES($1,$2) ON CONFLICT DO NOTHING RETURNING user_id`, candidate, id).Scan(&owner)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return p, err
		}
		_, err = tx.Exec(ctx, `INSERT INTO profiles(user_id,slug) VALUES($1,$2)`, id, candidate)
		if err != nil {
			return p, dbError(err)
		}
		p.DisplayName = name
		p.Slug = candidate
		p.Revision = 1
		return p, tx.Commit(ctx)
	}
	return p, profiles.ErrConflict
}
func (s *Store) UpdateProfile(ctx context.Context, id uuid.UUID, change func(profiles.Profile) (profiles.Profile, error)) (profiles.Profile, error) {
	var p profiles.Profile
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return p, err
	}
	defer tx.Rollback(ctx)
	// Match onboarding and account deletion: lock the parent before its profile.
	var locked uuid.UUID
	if err = tx.QueryRow(ctx, `SELECT id FROM users WHERE id=$1 FOR UPDATE`, id).Scan(&locked); err != nil {
		return p, missing(err)
	}
	var old string
	if err = tx.QueryRow(ctx, `SELECT p.slug,COALESCE(u.display_name,''),p.headline,p.bio,p.location,p.current_company,p.current_job_title,p.content_revision FROM profiles p JOIN users u ON u.id=p.user_id WHERE p.user_id=$1 FOR UPDATE OF p`, id).Scan(&p.Slug, &p.DisplayName, &p.Headline, &p.Bio, &p.Location, &p.CurrentCompany, &p.CurrentJobTitle, &p.Revision); err != nil {
		return p, missing(err)
	}
	old = p.Slug
	p, err = change(p)
	if err != nil {
		return p, err
	}
	if old != p.Slug {
		var owner uuid.UUID
		err = tx.QueryRow(ctx, `INSERT INTO profile_slug_history(slug,user_id) VALUES($1,$2) ON CONFLICT(slug) DO UPDATE SET slug=EXCLUDED.slug WHERE profile_slug_history.user_id=EXCLUDED.user_id RETURNING user_id`, p.Slug, id).Scan(&owner)
		if errors.Is(err, pgx.ErrNoRows) {
			return p, profiles.ErrConflict
		}
		if err != nil {
			return p, dbError(err)
		}
	}
	err = tx.QueryRow(ctx, `UPDATE profiles SET slug=$2,headline=$3,bio=$4,location=$5,current_company=$6,current_job_title=$7,content_revision=content_revision+1,updated_at=now() WHERE user_id=$1 RETURNING content_revision`, id, p.Slug, p.Headline, p.Bio, p.Location, p.CurrentCompany, p.CurrentJobTitle).Scan(&p.Revision)
	if err != nil {
		return p, dbError(err)
	}
	_, err = tx.Exec(ctx, `UPDATE users SET display_name=$2,updated_at=now() WHERE id=$1`, id, p.DisplayName)
	if err != nil {
		return p, err
	}
	return p, tx.Commit(ctx)
}
func (s *Store) Counts(ctx context.Context, id uuid.UUID) ([3]int, error) {
	var c [3]int
	err := s.db.QueryRow(ctx, `SELECT (SELECT count(*) FROM profile_experiences WHERE user_id=$1),(SELECT count(*) FROM profile_educations WHERE user_id=$1),(SELECT count(*) FROM profile_skills WHERE user_id=$1)`, id).Scan(&c[0], &c[1], &c[2])
	return c, err
}
func (s *Store) Experiences(ctx context.Context, id uuid.UUID) ([]profiles.Experience, error) {
	rows, err := s.db.Query(ctx, `SELECT id,company,title,employment_type,location,start_date::text,end_date::text,is_current,description,revision FROM profile_experiences WHERE user_id=$1 ORDER BY start_date DESC,id LIMIT 100`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []profiles.Experience{}
	for rows.Next() {
		var e profiles.Experience
		if err = rows.Scan(&e.ID, &e.Company, &e.JobTitle, &e.EmploymentType, &e.Location, &e.StartDate, &e.EndDate, &e.IsCurrent, &e.Description, &e.Revision); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
func (s *Store) SaveExperience(ctx context.Context, id uuid.UUID, e profiles.Experience, update bool) (profiles.Experience, error) {
	var err error
	if update {
		err = s.db.QueryRow(ctx, `UPDATE profile_experiences SET company=$3,title=$4,employment_type=$5,location=$6,start_date=$7,end_date=$8,is_current=$9,description=$10,updated_at=now() WHERE id=$1 AND user_id=$2 RETURNING id,revision`, e.ID, id, e.Company, e.JobTitle, e.EmploymentType, e.Location, e.StartDate, e.EndDate, e.IsCurrent, e.Description).Scan(&e.ID, &e.Revision)
	} else {
		tx, beginErr := s.db.Begin(ctx)
		if beginErr != nil {
			return e, beginErr
		}
		defer tx.Rollback(ctx)
		if err = collectionSlot(ctx, tx, id, "profile_experiences"); err != nil {
			return e, err
		}
		err = tx.QueryRow(ctx, `INSERT INTO profile_experiences(user_id,company,title,employment_type,location,start_date,end_date,is_current,description) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id,revision`, id, e.Company, e.JobTitle, e.EmploymentType, e.Location, e.StartDate, e.EndDate, e.IsCurrent, e.Description).Scan(&e.ID, &e.Revision)
		if err != nil {
			return e, dbError(err)
		}
		return e, tx.Commit(ctx)
	}
	return e, missing(dbError(err))
}
func (s *Store) DeleteExperience(ctx context.Context, id, item uuid.UUID) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM profile_experiences WHERE id=$1 AND user_id=$2`, item, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return profiles.ErrNotFound
	}
	return nil
}
func (s *Store) Educations(ctx context.Context, id uuid.UUID) ([]profiles.Education, error) {
	rows, err := s.db.Query(ctx, `SELECT id,institution_name,degree,field_of_study,start_date::text,end_date::text,description,revision FROM profile_educations WHERE user_id=$1 ORDER BY start_date DESC NULLS LAST,id LIMIT 100`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []profiles.Education{}
	for rows.Next() {
		var e profiles.Education
		if err = rows.Scan(&e.ID, &e.Institution, &e.Degree, &e.FieldOfStudy, &e.StartDate, &e.EndDate, &e.Description, &e.Revision); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
func (s *Store) SaveEducation(ctx context.Context, id uuid.UUID, e profiles.Education, update bool) (profiles.Education, error) {
	var err error
	if update {
		err = s.db.QueryRow(ctx, `UPDATE profile_educations SET institution_name=$3,degree=$4,field_of_study=$5,start_date=$6,end_date=$7,description=$8,updated_at=now() WHERE id=$1 AND user_id=$2 RETURNING id,revision`, e.ID, id, e.Institution, e.Degree, e.FieldOfStudy, e.StartDate, e.EndDate, e.Description).Scan(&e.ID, &e.Revision)
	} else {
		tx, beginErr := s.db.Begin(ctx)
		if beginErr != nil {
			return e, beginErr
		}
		defer tx.Rollback(ctx)
		if err = collectionSlot(ctx, tx, id, "profile_educations"); err != nil {
			return e, err
		}
		err = tx.QueryRow(ctx, `INSERT INTO profile_educations(user_id,institution_name,degree,field_of_study,start_date,end_date,description) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id,revision`, id, e.Institution, e.Degree, e.FieldOfStudy, e.StartDate, e.EndDate, e.Description).Scan(&e.ID, &e.Revision)
		if err != nil {
			return e, dbError(err)
		}
		return e, tx.Commit(ctx)
	}
	return e, missing(dbError(err))
}
func (s *Store) DeleteEducation(ctx context.Context, id, item uuid.UUID) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM profile_educations WHERE id=$1 AND user_id=$2`, item, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return profiles.ErrNotFound
	}
	return nil
}
func (s *Store) SearchSkills(ctx context.Context, q string, limit int) ([]profiles.Skill, error) {
	rows, err := s.db.Query(ctx, `SELECT id,name FROM skills WHERE normalized_name LIKE $1 ESCAPE '\' ORDER BY normalized_name,id LIMIT $2`, strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(q, "\\", "\\\\"), "%", "\\%"), "_", "\\_"))+"%", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []profiles.Skill{}
	for rows.Next() {
		var x profiles.Skill
		if err = rows.Scan(&x.ID, &x.Name); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
func (s *Store) Skills(ctx context.Context, id uuid.UUID) ([]profiles.Skill, error) {
	rows, err := s.db.Query(ctx, `SELECT ps.id,s.name FROM profile_skills ps JOIN skills s ON s.id=ps.skill_id WHERE ps.user_id=$1 ORDER BY s.normalized_name,ps.id LIMIT 100`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []profiles.Skill{}
	for rows.Next() {
		var x profiles.Skill
		if err = rows.Scan(&x.ID, &x.Name); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
func (s *Store) AddSkill(ctx context.Context, id uuid.UUID, name, key string) (profiles.Skill, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return profiles.Skill{}, err
	}
	defer tx.Rollback(ctx)
	var skillID uuid.UUID
	var canonical string
	err = tx.QueryRow(ctx, `INSERT INTO skills(name,normalized_name) VALUES($1,$2) ON CONFLICT(normalized_name) DO UPDATE SET normalized_name=EXCLUDED.normalized_name RETURNING id,name`, name, key).Scan(&skillID, &canonical)
	if err != nil {
		return profiles.Skill{}, dbError(err)
	}
	// Serialize this owner's set operations before checking whether a link
	// already exists. Otherwise an add can return success for a link that a
	// concurrent delete removed before the add committed.
	var locked uuid.UUID
	if err = tx.QueryRow(ctx, `SELECT id FROM users WHERE id=$1 FOR UPDATE`, id).Scan(&locked); err != nil {
		return profiles.Skill{}, missing(err)
	}
	var linkID uuid.UUID
	err = tx.QueryRow(ctx, `SELECT id FROM profile_skills WHERE user_id=$1 AND skill_id=$2`, id, skillID).Scan(&linkID)
	if err == nil {
		return profiles.Skill{ID: linkID, Name: canonical}, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return profiles.Skill{}, err
	}
	if err = collectionSlot(ctx, tx, id, "profile_skills"); err != nil {
		return profiles.Skill{}, err
	}
	err = tx.QueryRow(ctx, `INSERT INTO profile_skills(user_id,skill_id) VALUES($1,$2) ON CONFLICT(user_id,skill_id) DO UPDATE SET skill_id=EXCLUDED.skill_id RETURNING id`, id, skillID).Scan(&linkID)
	if err != nil {
		return profiles.Skill{}, dbError(err)
	}
	return profiles.Skill{ID: linkID, Name: canonical}, tx.Commit(ctx)
}
func (s *Store) DeleteSkill(ctx context.Context, id, item uuid.UUID) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var locked uuid.UUID
	if err = tx.QueryRow(ctx, `SELECT id FROM users WHERE id=$1 FOR UPDATE`, id).Scan(&locked); err != nil {
		return missing(err)
	}
	tag, err := tx.Exec(ctx, `DELETE FROM profile_skills WHERE id=$1 AND user_id=$2`, item, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return profiles.ErrNotFound
	}
	return tx.Commit(ctx)
}

func (s *Store) UpdateProfileRevision(ctx context.Context, id uuid.UUID, expected int64, change func(profiles.Profile) (profiles.Profile, error)) (profiles.Profile, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return profiles.Profile{}, err
	}
	defer tx.Rollback(ctx)
	var p profiles.Profile
	var locked uuid.UUID
	if err = tx.QueryRow(ctx, `SELECT id FROM users WHERE id=$1 FOR UPDATE`, id).Scan(&locked); err != nil {
		return p, missing(err)
	}
	err = tx.QueryRow(ctx, `SELECT p.slug,COALESCE(u.display_name,''),p.headline,p.bio,p.location,p.current_company,p.current_job_title,p.content_revision FROM profiles p JOIN users u ON u.id=p.user_id WHERE p.user_id=$1 FOR UPDATE OF p`, id).Scan(&p.Slug, &p.DisplayName, &p.Headline, &p.Bio, &p.Location, &p.CurrentCompany, &p.CurrentJobTitle, &p.Revision)
	if err != nil {
		return p, missing(err)
	}
	if p.Revision != expected {
		return p, profiles.ErrStale
	}
	old := p.Slug
	p, err = change(p)
	if err != nil {
		return p, err
	}
	if old != p.Slug {
		var reserved uuid.UUID
		err = tx.QueryRow(ctx, `INSERT INTO profile_slug_history(slug,user_id) VALUES($1,$2) ON CONFLICT(slug) DO UPDATE SET slug=EXCLUDED.slug WHERE profile_slug_history.user_id=EXCLUDED.user_id RETURNING user_id`, p.Slug, id).Scan(&reserved)
		if errors.Is(err, pgx.ErrNoRows) {
			return p, profiles.ErrConflict
		}
		if err != nil {
			return p, dbError(err)
		}
	}
	tag, err := tx.Exec(ctx, `UPDATE profiles SET slug=$2,headline=$3,bio=$4,location=$5,current_company=$6,current_job_title=$7,content_revision=content_revision+1,updated_at=now() WHERE user_id=$1 AND content_revision=$8`, id, p.Slug, p.Headline, p.Bio, p.Location, p.CurrentCompany, p.CurrentJobTitle, expected)
	if err != nil {
		return p, dbError(err)
	}
	if tag.RowsAffected() != 1 {
		return p, profiles.ErrStale
	}
	_, err = tx.Exec(ctx, `UPDATE users SET display_name=$2,updated_at=now() WHERE id=$1`, id, p.DisplayName)
	if err != nil {
		return p, err
	}
	if err = tx.Commit(ctx); err != nil {
		return p, err
	}
	p.Revision = expected + 1
	return p, nil
}
func (s *Store) SaveExperienceRevision(ctx context.Context, id uuid.UUID, e profiles.Experience, expected int64) (profiles.Experience, error) {
	err := s.db.QueryRow(ctx, `UPDATE profile_experiences SET company=$3,title=$4,employment_type=$5,location=$6,start_date=$7,end_date=$8,is_current=$9,description=$10,updated_at=now() WHERE id=$1 AND user_id=$2 AND revision=$11 RETURNING id,revision`, e.ID, id, e.Company, e.JobTitle, e.EmploymentType, e.Location, e.StartDate, e.EndDate, e.IsCurrent, e.Description, expected).Scan(&e.ID, &e.Revision)
	if errors.Is(err, pgx.ErrNoRows) {
		return e, s.itemRevisionError(ctx, "profile_experiences", id, e.ID)
	}
	return e, dbError(err)
}
func (s *Store) SaveEducationRevision(ctx context.Context, id uuid.UUID, e profiles.Education, expected int64) (profiles.Education, error) {
	err := s.db.QueryRow(ctx, `UPDATE profile_educations SET institution_name=$3,degree=$4,field_of_study=$5,start_date=$6,end_date=$7,description=$8,updated_at=now() WHERE id=$1 AND user_id=$2 AND revision=$9 RETURNING id,revision`, e.ID, id, e.Institution, e.Degree, e.FieldOfStudy, e.StartDate, e.EndDate, e.Description, expected).Scan(&e.ID, &e.Revision)
	if errors.Is(err, pgx.ErrNoRows) {
		return e, s.itemRevisionError(ctx, "profile_educations", id, e.ID)
	}
	return e, dbError(err)
}
func (s *Store) itemRevisionError(ctx context.Context, table string, owner, item uuid.UUID) error {
	if table != "profile_educations" && table != "profile_experiences" {
		return profiles.ErrInvalid
	}
	var exists bool
	err := s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM `+table+` WHERE user_id=$1 AND id=$2)`, owner, item).Scan(&exists)
	if err != nil {
		return err
	}
	if exists {
		return profiles.ErrStale
	}
	return profiles.ErrNotFound
}
func (s *Store) DeleteExperienceRevision(ctx context.Context, id, item uuid.UUID, revision int64) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM profile_experiences WHERE id=$1 AND user_id=$2 AND revision=$3`, item, id, revision)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return s.itemRevisionError(ctx, "profile_experiences", id, item)
	}
	return nil
}
func (s *Store) DeleteEducationRevision(ctx context.Context, id, item uuid.UUID, revision int64) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM profile_educations WHERE id=$1 AND user_id=$2 AND revision=$3`, item, id, revision)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return s.itemRevisionError(ctx, "profile_educations", id, item)
	}
	return nil
}
