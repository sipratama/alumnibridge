package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/sipratama/alumnibridge/apps/api/internal/profiles"
)

func (s *Store) Public(ctx context.Context, slug string) (profiles.Publication, error) {
	var p profiles.Publication
	var body []byte
	var key, kind *string
	var size *int64
	err := s.db.QueryRow(ctx, `SELECT jsonb_strip_nulls(jsonb_build_object(
 'slug',p.slug,'display_name',COALESCE(u.display_name,''),'headline',p.headline,
 'verified_alumni',EXISTS(SELECT 1 FROM memberships m WHERE m.user_id=p.user_id AND m.role='alumni' AND m.status='verified'),
 'bio',CASE WHEN p.show_bio THEN p.bio END,'location',CASE WHEN p.show_location THEN p.location END,
 'current_company',CASE WHEN p.show_current_company THEN p.current_company END,
 'current_job_title',CASE WHEN p.show_current_job_title THEN p.current_job_title END,
 'experiences',CASE WHEN p.show_experiences THEN (SELECT COALESCE(jsonb_agg(x.data ORDER BY x.start_date DESC,x.id),'[]') FROM
 (SELECT e.id,e.start_date,jsonb_build_object('company',company,'job_title',title,'employment_type',employment_type,'location',location,'start_date',start_date::text,'end_date',end_date::text,'is_current',is_current,'description',description) data FROM profile_experiences e WHERE e.user_id=p.user_id ORDER BY start_date DESC,id LIMIT 100) x) END,
 'educations',CASE WHEN p.show_educations THEN (SELECT COALESCE(jsonb_agg(x.data ORDER BY x.start_date DESC NULLS LAST,x.id),'[]') FROM
 (SELECT e.id,e.start_date,jsonb_build_object('institution',institution_name,'degree',degree,'field_of_study',field_of_study,'start_date',start_date::text,'end_date',end_date::text,'description',description) data FROM profile_educations e WHERE e.user_id=p.user_id ORDER BY start_date DESC NULLS LAST,id LIMIT 100) x) END,
 'skills',CASE WHEN p.show_skills THEN (SELECT COALESCE(jsonb_agg(x.name ORDER BY x.normalized_name,x.id),'[]') FROM
 (SELECT s.name,s.normalized_name,s.id FROM profile_skills ps JOIN skills s ON s.id=ps.skill_id WHERE ps.user_id=p.user_id ORDER BY s.normalized_name,s.id LIMIT 100) x) END)),
 p.visibility,p.moderation_status,CASE WHEN p.show_photo THEN p.photo_object_key END,
 CASE WHEN p.show_photo THEN p.photo_content_type END,CASE WHEN p.show_photo THEN p.photo_size_bytes END
 FROM profile_slug_history h JOIN profiles p ON p.user_id=h.user_id JOIN users u ON u.id=p.user_id
 WHERE h.slug=$1 AND p.visibility='public' AND p.moderation_status='visible'`, slug).Scan(&body, &p.Visibility, &p.Moderation, &key, &kind, &size)
	if err != nil {
		return p, missing(err)
	}
	if err = json.Unmarshal(body, &p.Profile); err != nil {
		return p, err
	}
	if key != nil && kind != nil && size != nil {
		p.Photo = &profiles.Photo{Key: *key, ContentType: *kind, Size: *size}
		url := "/api/v1/profiles/" + p.Profile.Slug + "/photo"
		p.Profile.PhotoURL = &url
	}
	return p, nil
}

const privacyColumns = `visibility,show_bio,show_location,show_current_company,show_current_job_title,show_experiences,show_educations,show_skills,show_photo,privacy_revision`

func scanPrivacy(row pgx.Row) (profiles.Privacy, error) {
	var p profiles.Privacy
	err := row.Scan(&p.Visibility, &p.ShowBio, &p.ShowLocation, &p.ShowCurrentCompany, &p.ShowCurrentJobTitle, &p.ShowExperiences, &p.ShowEducations, &p.ShowSkills, &p.ShowPhoto, &p.Revision)
	return p, missing(err)
}
func (s *Store) Privacy(ctx context.Context, id uuid.UUID) (profiles.Privacy, error) {
	return scanPrivacy(s.db.QueryRow(ctx, `SELECT `+privacyColumns+` FROM profiles WHERE user_id=$1`, id))
}
func (s *Store) UpdatePrivacy(ctx context.Context, id uuid.UUID, p profiles.PrivacyPatch) (profiles.Privacy, error) {
	return scanPrivacy(s.db.QueryRow(ctx, `UPDATE profiles SET visibility=$2,show_bio=COALESCE($3,show_bio),show_location=COALESCE($4,show_location),show_current_company=COALESCE($5,show_current_company),show_current_job_title=COALESCE($6,show_current_job_title),show_experiences=COALESCE($7,show_experiences),show_educations=COALESCE($8,show_educations),show_skills=COALESCE($9,show_skills),show_photo=COALESCE($10,show_photo),privacy_revision=privacy_revision+1,updated_at=now() WHERE user_id=$1 RETURNING `+privacyColumns, id, p.Visibility, p.ShowBio, p.ShowLocation, p.ShowCurrentCompany, p.ShowCurrentJobTitle, p.ShowExperiences, p.ShowEducations, p.ShowSkills, p.ShowPhoto))
}
// SQL predicate and revision increment are one statement: no stale side effects.
func (s *Store) UpdatePrivacyConditional(ctx context.Context, id uuid.UUID, p profiles.PrivacyPatch, expected int64) (profiles.Privacy, error) {
 out, err := scanPrivacy(s.db.QueryRow(ctx, `UPDATE profiles SET visibility=$2,show_bio=COALESCE($3,show_bio),show_location=COALESCE($4,show_location),show_current_company=COALESCE($5,show_current_company),show_current_job_title=COALESCE($6,show_current_job_title),show_experiences=COALESCE($7,show_experiences),show_educations=COALESCE($8,show_educations),show_skills=COALESCE($9,show_skills),show_photo=COALESCE($10,show_photo),privacy_revision=privacy_revision+1,updated_at=now() WHERE user_id=$1 AND privacy_revision=$11 RETURNING `+privacyColumns, id,p.Visibility,p.ShowBio,p.ShowLocation,p.ShowCurrentCompany,p.ShowCurrentJobTitle,p.ShowExperiences,p.ShowEducations,p.ShowSkills,p.ShowPhoto,expected))
 if err == profiles.ErrNotFound {
  var exists bool
  if qerr := s.db.QueryRow(ctx,`SELECT EXISTS(SELECT 1 FROM profiles WHERE user_id=$1)`,id).Scan(&exists); qerr != nil {return out,qerr}
  if exists {return out,profiles.ErrStale}
 }
 return out,err
}
func (s *Store) OwnerPhoto(ctx context.Context, id uuid.UUID) (profiles.Photo, error) {
	var p profiles.Photo
	err := s.db.QueryRow(ctx, `SELECT photo_object_key,photo_content_type,photo_size_bytes FROM profiles WHERE user_id=$1 AND photo_object_key IS NOT NULL`, id).Scan(&p.Key, &p.ContentType, &p.Size)
	return p, missing(err)
}
func (s *Store) StagePhoto(ctx context.Context, id uuid.UUID, p profiles.Photo) error {
	_, err := s.db.Exec(ctx, `INSERT INTO profile_photo_objects(object_key,owner_id,state,content_type,size_bytes) VALUES($1,$2,'staging',$3,$4)`, p.Key, id, p.ContentType, p.Size)
	return err
}
func (s *Store) ActivatePhoto(ctx context.Context, id uuid.UUID, key string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var p profiles.Photo
	p.Key = key
	err = tx.QueryRow(ctx, `SELECT content_type,size_bytes FROM profile_photo_objects WHERE object_key=$1 AND owner_id=$2 AND state='staging' AND expires_at>now() FOR UPDATE`, key, id).Scan(&p.ContentType, &p.Size)
	if err != nil {
		return missing(err)
	}
	var old *string
	err = tx.QueryRow(ctx, `SELECT photo_object_key FROM profiles WHERE user_id=$1 FOR UPDATE`, id).Scan(&old)
	if err != nil {
		return missing(err)
	}
	_, err = tx.Exec(ctx, `UPDATE profiles SET photo_object_key=$2,photo_content_type=$3,photo_size_bytes=$4,updated_at=now() WHERE user_id=$1`, id, key, p.ContentType, p.Size)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE profile_photo_objects SET state='active' WHERE object_key=$1`, key)
	if err != nil {
		return err
	}
	if old != nil {
		_, err = tx.Exec(ctx, `UPDATE profile_photo_objects SET state='garbage',expires_at=now() WHERE object_key=$1`, *old)
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
func (s *Store) RemovePhoto(ctx context.Context, id uuid.UUID) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var old *string
	err = tx.QueryRow(ctx, `SELECT photo_object_key FROM profiles WHERE user_id=$1 FOR UPDATE`, id).Scan(&old)
	if err != nil {
		return missing(err)
	}
	_, err = tx.Exec(ctx, `UPDATE profiles SET photo_object_key=NULL,photo_content_type=NULL,photo_size_bytes=NULL,updated_at=now() WHERE user_id=$1`, id)
	if err != nil {
		return err
	}
	if old != nil {
		_, err = tx.Exec(ctx, `UPDATE profile_photo_objects SET state='garbage',expires_at=now() WHERE object_key=$1`, *old)
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
func (s *Store) CleanupPhotos(ctx context.Context, remove func(context.Context, string) error) error {
	for i := 0; i < 20; i++ {
		tx, err := s.db.Begin(ctx)
		if err != nil {
			return err
		}
		var key string
		err = tx.QueryRow(ctx, `SELECT object_key FROM profile_photo_objects o WHERE expires_at<=now() AND NOT EXISTS(SELECT 1 FROM profiles p WHERE p.photo_object_key=o.object_key) ORDER BY expires_at,object_key FOR UPDATE SKIP LOCKED LIMIT 1`).Scan(&key)
		if errors.Is(err, pgx.ErrNoRows) {
			tx.Rollback(ctx)
			return nil
		}
		if err != nil {
			tx.Rollback(ctx)
			return err
		}
		// Recheck after obtaining the lock; activation holds this same row lock.
		var used bool
		err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM profiles WHERE photo_object_key=$1)`, key).Scan(&used)
		if err != nil {
			tx.Rollback(ctx)
			return err
		}
		if used {
			tx.Rollback(ctx)
			continue
		}
		if err = remove(ctx, key); err != nil {
			if _, retryErr := tx.Exec(ctx, `UPDATE profile_photo_objects SET expires_at=now()+interval '1 minute' WHERE object_key=$1`, key); retryErr != nil {
				tx.Rollback(ctx)
				return errors.Join(err, retryErr)
			}
			if commitErr := tx.Commit(ctx); commitErr != nil {
				return errors.Join(err, commitErr)
			}
			return err
		}
		_, err = tx.Exec(ctx, `DELETE FROM profile_photo_objects WHERE object_key=$1`, key)
		if err != nil {
			tx.Rollback(ctx)
			return err
		}
		if err = tx.Commit(ctx); err != nil {
			return err
		}
	}
	return nil
}
