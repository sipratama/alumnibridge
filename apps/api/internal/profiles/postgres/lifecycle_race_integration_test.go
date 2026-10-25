package postgres

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sipratama/alumnibridge/apps/api/internal/directory"
	directorypg "github.com/sipratama/alumnibridge/apps/api/internal/directory/postgres"
	"github.com/sipratama/alumnibridge/apps/api/internal/identity"
	identitypg "github.com/sipratama/alumnibridge/apps/api/internal/identity/postgres"
	"github.com/sipratama/alumnibridge/apps/api/internal/profiles"
)

const lifecycleIssuer = "https://lifecycle-race.example/realms/alumnibridge"
const lifecycleLockSQL = `SELECT pg_advisory_xact_lock(hashtextextended(length($1)::text || ':' || $1 || $2, 0))`

type lifecycleFixture struct {
	db      *pgxpool.Pool
	owner   uuid.UUID
	subject string
	slug    string
	content int64
	privacy int64
}

func newLifecycleFixture(t *testing.T, ctx context.Context, url string) lifecycleFixture {
	t.Helper()
	db, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	f := lifecycleFixture{db: db, owner: uuid.New()}
	f.subject = "lifecycle-" + f.owner.String()
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := db.Exec(cleanup, `DELETE FROM users WHERE id=$1`, f.owner); err != nil {
			t.Error(err)
		}
		if _, err := db.Exec(cleanup, `DELETE FROM identity_subject_blocks WHERE issuer=$1 AND subject=$2`, lifecycleIssuer, f.subject); err != nil {
			t.Error(err)
		}
		db.Close()
	})
	if _, err := db.Exec(ctx, `INSERT INTO users(id,oidc_subject,display_name) VALUES($1,$2,'Lifecycle Test')`, f.owner, f.subject); err != nil {
		t.Fatal(err)
	}
	p, err := New(db).Profile(ctx, f.owner)
	if err != nil {
		t.Fatal(err)
	}
	f.slug = p.Slug
	var membership uuid.UUID
	if err := db.QueryRow(ctx, `INSERT INTO memberships(user_id,study_program_id,role,status,graduation_year)
		SELECT $1,id,'alumni','verified',2020 FROM study_programs WHERE slug='mathematics-education' LIMIT 1 RETURNING id`, f.owner).Scan(&membership); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `UPDATE profiles SET visibility='public',directory_discoverable=true,directory_consented_at=now() WHERE user_id=$1`, f.owner); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO profile_directory_affiliations(membership_id,show_institution) VALUES($1,true)`, membership); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `SELECT content_revision,privacy_revision FROM profiles WHERE user_id=$1`, f.owner).Scan(&f.content, &f.privacy); err != nil {
		t.Fatal(err)
	}
	if _, err := profiles.NewPublishing(New(db), &stagedPhotoStorage{}).Public(ctx, f.slug); err != nil {
		t.Fatal(err)
	}
	if !directoryContains(t, ctx, f) {
		t.Fatal("active consenting fixture absent from anonymous directory")
	}
	if err := identitypg.New(db, lifecycleIssuer).CheckAccess(ctx, lifecycleIssuer, f.subject); err != nil {
		t.Fatal(err)
	}
	return f
}

// This is a synthetic database transition, not an application deletion writer.
func commitSyntheticWithdrawal(ctx context.Context, db *pgxpool.Pool, f lifecycleFixture) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, lifecycleLockSQL, lifecycleIssuer, f.subject); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `SELECT id FROM users WHERE id=$1 FOR UPDATE`, f.owner); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO identity_subject_blocks(issuer,subject) VALUES($1,$2)`, lifecycleIssuer, f.subject); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE users SET access_status='deleting' WHERE id=$1`, f.owner); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE profiles SET visibility='private',directory_discoverable=false,directory_consented_at=NULL WHERE user_id=$1`, f.owner); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func assertLifecycleWithdrawal(t *testing.T, ctx context.Context, f lifecycleFixture, wantContent int64) {
	t.Helper()
	if err := identitypg.New(f.db, lifecycleIssuer).CheckAccess(ctx, lifecycleIssuer, f.subject); !errors.Is(err, identity.ErrAccessDenied) {
		t.Fatalf("subject remained accessible after transition: %v", err)
	}
	var status, visibility string
	var consent bool
	var consentAt *time.Time
	var content, privacy int64
	if err := f.db.QueryRow(ctx, `SELECT u.access_status,p.visibility,p.directory_discoverable,p.directory_consented_at,p.content_revision,p.privacy_revision
		FROM users u JOIN profiles p ON p.user_id=u.id WHERE u.id=$1`, f.owner).Scan(&status, &visibility, &consent, &consentAt, &content, &privacy); err != nil {
		t.Fatal(err)
	}
	if status != "deleting" || visibility != "private" || consent || consentAt != nil || content != wantContent || privacy <= f.privacy {
		t.Fatalf("withdrawal state: status=%s visibility=%s consent=%t at=%v content=%d privacy=%d", status, visibility, consent, consentAt, content, privacy)
	}
	var affiliations int
	if err := f.db.QueryRow(ctx, `SELECT count(*) FROM profile_directory_affiliations a JOIN memberships m ON m.id=a.membership_id WHERE m.user_id=$1`, f.owner).Scan(&affiliations); err != nil || affiliations != 0 {
		t.Fatalf("affiliations survived: %d %v", affiliations, err)
	}
	if _, err := profiles.NewPublishing(New(f.db), &stagedPhotoStorage{}).Public(ctx, f.slug); !errors.Is(err, profiles.ErrNotFound) {
		t.Fatalf("anonymous public profile survived: %v", err)
	}
	if directoryContains(t, ctx, f) {
		t.Fatal("withdrawn owner remained in anonymous directory")
	}
}

func directoryContains(t *testing.T, ctx context.Context, f lifecycleFixture) bool {
	t.Helper()
	items, err := directorypg.New(f.db).Search(ctx, directory.Spec{Query: "Lifecycle Test", Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if item.Slug == f.slug {
			return true
		}
	}
	return false
}

func oneConnection(t *testing.T, ctx context.Context, url string) (*pgxpool.Pool, int) {
	t.Helper()
	config, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	config.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	var pid int
	if err := pool.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	return pool, pid
}

func TestProfileMutationVersusLifecycleWithdrawal(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	for _, ownerFirst := range []bool{false, true} {
		name := "withdrawal_first"
		if ownerFirst {
			name = "owner_first"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			f := newLifecycleFixture(t, ctx, url)
			store := New(f.db)
			change := func(p profiles.Profile) (profiles.Profile, error) { p.Headline = "Owner write"; return p, nil }
			if !ownerFirst {
				if err := commitSyntheticWithdrawal(ctx, f.db, f); err != nil {
					t.Fatal(err)
				}
				// Negative control for the former single-statement profile writer:
				// PostgreSQL alone does not reject a post-transition dependent-row write.
				unguarded, err := f.db.Begin(ctx)
				if err != nil {
					t.Fatal(err)
				}
				tag, err := unguarded.Exec(ctx, `UPDATE profiles SET headline='unguarded' WHERE user_id=$1`, f.owner)
				if err != nil || tag.RowsAffected() != 1 {
					t.Fatalf("unguarded control: affected=%d err=%v", tag.RowsAffected(), err)
				}
				if err := unguarded.Rollback(ctx); err != nil {
					t.Fatal(err)
				}
				if _, err := store.UpdateProfileRevision(ctx, f.owner, f.content, change); !errors.Is(err, profiles.ErrAccountUnavailable) {
					t.Fatalf("post-withdrawal profile PATCH: %v", err)
				}
				publish := true
				guards := map[string]func() error{
					"privacy": func() error {
						_, err := store.UpdatePrivacyConditional(ctx, f.owner, profiles.PrivacyPatch{Visibility: "public"}, f.privacy)
						return err
					},
					"discovery": func() error {
						_, err := store.UpdateDiscoveryConditional(ctx, f.owner, profiles.DiscoveryPatch{DirectoryDiscoverable: &publish}, f.privacy)
						return err
					},
					"experience append": func() error {
						_, err := store.SaveExperience(ctx, f.owner, profiles.Experience{Company: "Test", JobTitle: "Writer", StartDate: "2020-01-01"}, false)
						return err
					},
					"education append": func() error {
						_, err := store.SaveEducation(ctx, f.owner, profiles.Education{Institution: "Test"}, false)
						return err
					},
					"skill add": func() error {
						_, err := store.AddSkill(ctx, f.owner, "Test Skill", "test-skill-"+f.owner.String())
						return err
					},
					"photo stage": func() error {
						return store.StagePhoto(ctx, f.owner, profiles.Photo{Key: uuid.NewString(), ContentType: "image/png", Size: 1})
					},
					"photo remove": func() error { _, err := store.RemovePhotoConditional(ctx, f.owner, f.content); return err },
				}
				for name, write := range guards {
					if err := write(); !errors.Is(err, profiles.ErrAccountUnavailable) {
						t.Fatalf("post-withdrawal %s: %v", name, err)
					}
				}
				assertLifecycleWithdrawal(t, ctx, f, f.content)
				return
			}
			ownerDB, ownerPID := oneConnection(t, ctx, url)
			transitionDB, transitionPID := oneConnection(t, ctx, url)
			hold, err := f.db.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer hold.Rollback(ctx)
			if _, err := hold.Exec(ctx, `SELECT user_id FROM profiles WHERE user_id=$1 FOR UPDATE`, f.owner); err != nil {
				t.Fatal(err)
			}
			ownerDone := make(chan error, 1)
			go func() {
				_, err := New(ownerDB).UpdateProfileRevision(ctx, f.owner, f.content, change)
				ownerDone <- err
			}()
			waitForBackendLock(t, ctx, f.db, ownerPID) // Owner holds user, waits for profile.
			transitionDone := make(chan error, 1)
			go func() { transitionDone <- commitSyntheticWithdrawal(ctx, transitionDB, f) }()
			waitForBackendLock(t, ctx, f.db, transitionPID) // Transition waits for user.
			if err := hold.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			if err := <-ownerDone; err != nil {
				t.Fatalf("owner-first profile PATCH: %v", err)
			}
			if err := <-transitionDone; err != nil {
				t.Fatalf("transition after owner: %v", err)
			}
			assertLifecycleWithdrawal(t, ctx, f, f.content+1)
			var headline string
			if err := f.db.QueryRow(ctx, `SELECT headline FROM profiles WHERE user_id=$1`, f.owner).Scan(&headline); err != nil || headline != "Owner write" {
				t.Fatalf("owner write was lost: %q %v", headline, err)
			}
		})
	}
}

func TestPhotoActivationVersusLifecycleWithdrawal(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	for _, ownerFirst := range []bool{false, true} {
		name := "withdrawal_first"
		if ownerFirst {
			name = "owner_first"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			f := newLifecycleFixture(t, ctx, url)
			store := New(f.db)
			key := "lifecycle-photo-" + uuid.NewString()
			storage := &stagedPhotoStorage{objects: map[string][]byte{key: []byte("synthetic")}}
			if err := store.StagePhoto(ctx, f.owner, profiles.Photo{Key: key, ContentType: "image/png", Size: 9}); err != nil {
				t.Fatal(err)
			}
			service := profiles.NewPublishing(store, storage)
			if !ownerFirst {
				if err := commitSyntheticWithdrawal(ctx, f.db, f); err != nil {
					t.Fatal(err)
				}
				if _, err := store.ActivatePhotoConditional(ctx, f.owner, key, f.content); !errors.Is(err, profiles.ErrAccountUnavailable) {
					t.Fatalf("post-withdrawal photo activation: %v", err)
				}
				if err := store.DiscardStagedPhoto(ctx, f.owner, key); err != nil {
					t.Fatal(err)
				}
				assertLifecycleWithdrawal(t, ctx, f, f.content)
			} else {
				ownerDB, ownerPID := oneConnection(t, ctx, url)
				transitionDB, transitionPID := oneConnection(t, ctx, url)
				hold, err := f.db.Begin(ctx)
				if err != nil {
					t.Fatal(err)
				}
				defer hold.Rollback(ctx)
				if _, err := hold.Exec(ctx, `SELECT object_key FROM profile_photo_objects WHERE object_key=$1 FOR UPDATE`, key); err != nil {
					t.Fatal(err)
				}
				ownerDone := make(chan error, 1)
				go func() {
					_, err := New(ownerDB).ActivatePhotoConditional(ctx, f.owner, key, f.content)
					ownerDone <- err
				}()
				waitForBackendLock(t, ctx, f.db, ownerPID) // Owner holds user, waits for staging.
				transitionDone := make(chan error, 1)
				go func() { transitionDone <- commitSyntheticWithdrawal(ctx, transitionDB, f) }()
				waitForBackendLock(t, ctx, f.db, transitionPID)
				if err := hold.Commit(ctx); err != nil {
					t.Fatal(err)
				}
				if err := <-ownerDone; err != nil {
					t.Fatalf("owner-first photo activation: %v", err)
				}
				if err := <-transitionDone; err != nil {
					t.Fatalf("transition after photo: %v", err)
				}
				assertLifecycleWithdrawal(t, ctx, f, f.content+1)
				var pointer string
				if err := f.db.QueryRow(ctx, `SELECT photo_object_key FROM profiles WHERE user_id=$1`, f.owner).Scan(&pointer); err != nil || pointer != key {
					t.Fatalf("serialized photo pointer: %q %v", pointer, err)
				}
				// Simulate later erasure only to exercise existing detached-object cleanup.
				if _, err := f.db.Exec(ctx, `DELETE FROM users WHERE id=$1`, f.owner); err != nil {
					t.Fatal(err)
				}
				if _, err := f.db.Exec(ctx, `UPDATE profile_photo_objects SET expires_at=now()-interval '1 second' WHERE object_key=$1`, key); err != nil {
					t.Fatal(err)
				}
			}
			if err := service.Cleanup(ctx); err != nil {
				t.Fatal(err)
			}
			storage.mu.Lock()
			_, exists := storage.objects[key]
			storage.mu.Unlock()
			var rows int
			if err := f.db.QueryRow(ctx, `SELECT count(*) FROM profile_photo_objects WHERE object_key=$1`, key).Scan(&rows); err != nil || rows != 0 || exists {
				t.Fatalf("photo cleanup: registry=%d object=%t err=%v", rows, exists, err)
			}
		})
	}
}
