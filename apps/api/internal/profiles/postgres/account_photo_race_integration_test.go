package postgres

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sipratama/alumnibridge/apps/api/internal/profiles"
)

func TestAccountDeletionVersusPhotoActivation(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	for _, holdProfile := range []bool{false, true} {
		name := "delete_before_activation"
		if holdProfile {
			name = "user_lock_serializes_activation_before_profile"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			config, err := pgxpool.ParseConfig(url)
			if err != nil {
				t.Fatal(err)
			}
			config.MaxConns = 1 // The activation backend PID is stable for lock observation.
			uploadDB, err := pgxpool.NewWithConfig(ctx, config)
			if err != nil {
				t.Fatal(err)
			}
			defer uploadDB.Close()
			deleteDB, err := pgxpool.New(ctx, url)
			if err != nil {
				t.Fatal(err)
			}
			defer deleteDB.Close()
			owner := uuid.New()
			if _, err := uploadDB.Exec(ctx, `INSERT INTO users(id,oidc_subject) VALUES($1,$2)`, owner, "account-photo-"+owner.String()); err != nil {
				t.Fatal(err)
			}
			defer deleteDB.Exec(context.Background(), `DELETE FROM users WHERE id=$1`, owner)
			store := New(uploadDB)
			profile, err := store.Profile(ctx, owner)
			if err != nil {
				t.Fatal(err)
			}
			var pid int
			if err := uploadDB.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
				t.Fatal(err)
			}
			var body bytes.Buffer
			if err := png.Encode(&body, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
				t.Fatal(err)
			}
			storage := &stagedPhotoStorage{objects: make(map[string][]byte), started: make(chan struct{}), release: make(chan struct{})}
			service := profiles.NewPublishing(store, storage)
			uploadDone := make(chan error, 1)
			go func() {
				_, err := service.UploadPhotoConditional(ctx, owner, bytes.NewReader(body.Bytes()), "image/png", profile.Revision)
				uploadDone <- err
			}()
			select {
			case <-storage.started: // StagePhoto committed before object Put paused.
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}

			var deleteErr error
			if !holdProfile {
				_, deleteErr = deleteDB.Exec(ctx, `DELETE FROM users WHERE id=$1`, owner)
				if deleteErr != nil {
					t.Fatal(deleteErr)
				}
				close(storage.release)
				if err := <-uploadDone; !errors.Is(err, profiles.ErrNotFound) {
					t.Fatalf("activation after account deletion: %v", err)
				}
			} else {
				// Activation now takes the user lock before staging and profile.
				// Deletion can hold user and profile without an object/profile inversion.
				tx, err := deleteDB.Begin(ctx)
				if err != nil {
					t.Fatal(err)
				}
				var locked uuid.UUID
				if err := tx.QueryRow(ctx, `SELECT id FROM users WHERE id=$1 FOR UPDATE`, owner).Scan(&locked); err != nil {
					t.Fatal(err)
				}
				if err := tx.QueryRow(ctx, `SELECT user_id FROM profiles WHERE user_id=$1 FOR UPDATE`, owner).Scan(&locked); err != nil {
					t.Fatal(err)
				}
				close(storage.release)
				waitForBackendLock(t, ctx, deleteDB, pid)
				if _, err := tx.Exec(ctx, `DELETE FROM users WHERE id=$1`, owner); err != nil {
					t.Fatal(err)
				}
				if err := tx.Commit(ctx); err != nil {
					t.Fatal(err)
				}
				if err := <-uploadDone; !errors.Is(err, profiles.ErrNotFound) {
					t.Fatalf("activation after serialized deletion: %v", err)
				}
			}

			var count int
			if err := deleteDB.QueryRow(ctx, `SELECT count(*) FROM profiles WHERE user_id=$1`, owner).Scan(&count); err != nil || count != 0 {
				t.Fatalf("deleted account retained a profile: %d %v", count, err)
			}
			// Account deletion leaves detached registry rows for the durable
			// worker. Make their normal expiration due and use the real cleanup path.
			storage.mu.Lock()
			keys := make([]string, 0, len(storage.objects))
			for key := range storage.objects {
				keys = append(keys, key)
			}
			storage.mu.Unlock()
			for _, key := range keys {
				if _, err := deleteDB.Exec(ctx, `UPDATE profile_photo_objects SET expires_at=now()-interval '1 second' WHERE object_key=$1`, key); err != nil {
					t.Fatal(err)
				}
			}
			if err := service.Cleanup(ctx); err != nil {
				t.Fatal(err)
			}
			storage.mu.Lock()
			remaining := len(storage.objects)
			storage.mu.Unlock()
			if remaining != 0 {
				t.Fatalf("deleted-account photo object retained: %d", remaining)
			}
			for _, key := range keys {
				if err := deleteDB.QueryRow(ctx, `SELECT count(*) FROM profile_photo_objects WHERE object_key=$1`, key).Scan(&count); err != nil || count != 0 {
					t.Fatalf("deleted-account photo registry retained: %d %v", count, err)
				}
			}
		})
	}
}

func waitForBackendLock(t *testing.T, ctx context.Context, monitor *pgxpool.Pool, pid int) {
	t.Helper()
	for {
		var wait *string
		if err := monitor.QueryRow(ctx, `SELECT wait_event_type FROM pg_stat_activity WHERE pid=$1`, pid).Scan(&wait); err != nil {
			t.Fatal(err)
		}
		if wait != nil && *wait == "Lock" {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("backend did not wait on a row lock: ", ctx.Err())
		case <-time.After(10 * time.Millisecond):
		}
	}
}
