package postgres

import (
	"context"

	"github.com/google/uuid"
	"github.com/sipratama/alumnibridge/apps/api/internal/profiles"
)

// ActivatePhotoConditional owns the staging row before the profile row. The
// profile-content comparison and pointer swap occur in the same transaction;
// a stale upload never marks its staged object active or replaces a newer photo.
func (s *Store) ActivatePhotoConditional(ctx context.Context, owner uuid.UUID, key string, expected int64) (int64, error) {
	tx, err := s.beginActiveUser(ctx, owner)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	var photo profiles.Photo
	err = tx.QueryRow(ctx, `SELECT content_type,size_bytes FROM profile_photo_objects
		WHERE object_key=$1 AND owner_id=$2 AND state='staging' AND expires_at>now() FOR UPDATE`, key, owner).Scan(&photo.ContentType, &photo.Size)
	if err != nil {
		return 0, missing(err)
	}
	var old *string
	var revision int64
	err = tx.QueryRow(ctx, `SELECT photo_object_key,content_revision FROM profiles WHERE user_id=$1 FOR UPDATE`, owner).Scan(&old, &revision)
	if err != nil {
		return 0, missing(err)
	}
	if revision != expected {
		return 0, profiles.ErrStale
	}
	err = tx.QueryRow(ctx, `UPDATE profiles SET photo_object_key=$2,photo_content_type=$3,
		photo_size_bytes=$4,updated_at=now() WHERE user_id=$1 AND content_revision=$5
		RETURNING content_revision`, owner, key, photo.ContentType, photo.Size, expected).Scan(&revision)
	if err != nil {
		return 0, dbError(err)
	}
	if _, err = tx.Exec(ctx, `UPDATE profile_photo_objects SET state='active' WHERE object_key=$1 AND owner_id=$2`, key, owner); err != nil {
		return 0, err
	}
	if old != nil {
		if _, err = tx.Exec(ctx, `UPDATE profile_photo_objects SET state='garbage',expires_at=now() WHERE object_key=$1`, *old); err != nil {
			return 0, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return 0, err
	}
	return revision, nil
}

func (s *Store) RemovePhotoConditional(ctx context.Context, owner uuid.UUID, expected int64) (int64, error) {
	tx, err := s.beginActiveUser(ctx, owner)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	var old *string
	var revision int64
	err = tx.QueryRow(ctx, `SELECT photo_object_key,content_revision FROM profiles WHERE user_id=$1 FOR UPDATE`, owner).Scan(&old, &revision)
	if err != nil {
		return 0, missing(err)
	}
	if revision != expected {
		return 0, profiles.ErrStale
	}
	if old == nil {
		if err = tx.Commit(ctx); err != nil {
			return 0, err
		}
		return revision, nil
	}
	err = tx.QueryRow(ctx, `UPDATE profiles SET photo_object_key=NULL,photo_content_type=NULL,
		photo_size_bytes=NULL,updated_at=now() WHERE user_id=$1 AND content_revision=$2
		RETURNING content_revision`, owner, expected).Scan(&revision)
	if err != nil {
		return 0, dbError(err)
	}
	if _, err = tx.Exec(ctx, `UPDATE profile_photo_objects SET state='garbage',expires_at=now() WHERE object_key=$1`, *old); err != nil {
		return 0, err
	}
	if err = tx.Commit(ctx); err != nil {
		return 0, err
	}
	return revision, nil
}

// The staging row remains durable until the cleanup worker deletes the object.
// An owner-scoped state transition cannot mark an active or foreign photo for
// deletion, even if cleanup races activation on another connection.
func (s *Store) DiscardStagedPhoto(ctx context.Context, owner uuid.UUID, key string) error {
	_, err := s.db.Exec(ctx, `UPDATE profile_photo_objects SET state='garbage',expires_at=now()
		WHERE object_key=$1 AND owner_id=$2 AND state='staging'`, key, owner)
	return err
}
