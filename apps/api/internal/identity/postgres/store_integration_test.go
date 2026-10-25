package postgres

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sipratama/alumnibridge/apps/api/internal/identity"
)

func TestProvisionAndOnboardPostgres(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to run PostgreSQL integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	db, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	store := New(db, "https://identity-integration.example")
	subject := "integration-" + uuid.NewString()
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := db.Exec(cleanupCtx, `DELETE FROM users WHERE oidc_subject=$1`, subject); err != nil {
			t.Errorf("remove integration user: %v", err)
		}
		db.Close()
	})
	var wg sync.WaitGroup
	ids := make(chan uuid.UUID, 2)
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			user, err := store.Provision(ctx, subject, "integration@example.invalid")
			ids <- user.ID
			errs <- err
		}()
	}
	wg.Wait()
	close(ids)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var id uuid.UUID
	for got := range ids {
		if id == uuid.Nil {
			id = got
		} else if got != id {
			t.Fatalf("duplicate provisioning produced %s and %s", id, got)
		}
	}
	var count int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM users WHERE oidc_subject=$1`, subject).Scan(&count); err != nil || count != 1 {
		t.Fatalf("count=%d err=%v", count, err)
	}
	student := identity.OnboardingInput{DisplayName: "Integration User", Role: "student", Institution: "uhamka", StudyProgram: "mathematics-education"}
	user, err := store.Onboard(ctx, id, student)
	if err != nil || len(user.Memberships) != 1 || user.Memberships[0].Status != "active" {
		t.Fatalf("student=%+v err=%v", user, err)
	}
	slug := "onboard-revision-" + uuid.NewString()
	if _, err := db.Exec(ctx, `INSERT INTO profile_slug_history(slug,user_id) VALUES($1,$2)`, slug, id); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO profiles(user_id,slug) VALUES($1,$2)`, id, slug); err != nil {
		t.Fatal(err)
	}
	var initialRevision int64
	if err := db.QueryRow(ctx, `SELECT content_revision FROM profiles WHERE user_id=$1`, id).Scan(&initialRevision); err != nil {
		t.Fatal(err)
	}
	user, err = store.Onboard(ctx, id, student)
	if err != nil || len(user.Memberships) != 1 {
		t.Fatalf("idempotent student=%+v err=%v", user, err)
	}
	var revision int64
	if err := db.QueryRow(ctx, `SELECT content_revision FROM profiles WHERE user_id=$1`, id).Scan(&revision); err != nil || revision != initialRevision {
		t.Fatalf("idempotent onboarding changed profile revision: %d %v", revision, err)
	}
	renamed := student
	renamed.DisplayName = "Renamed Integration User"
	if _, err := store.Onboard(ctx, id, renamed); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `SELECT content_revision FROM profiles WHERE user_id=$1`, id).Scan(&revision); err != nil || revision != initialRevision+1 {
		t.Fatalf("display-name change did not advance profile revision: %d %v", revision, err)
	}
	year := 2020
	alumni := student
	alumni.Role = "alumni"
	alumni.GraduationYear = &year
	user, err = store.Onboard(ctx, id, alumni)
	if err != nil || len(user.Memberships) != 2 {
		t.Fatalf("alumni=%+v err=%v", user, err)
	}
	var alumniStatus string
	for _, m := range user.Memberships {
		if m.Role == "alumni" {
			alumniStatus = m.Status
		}
	}
	if alumniStatus != "pending" {
		t.Fatalf("alumni status=%s", alumniStatus)
	}
	year = 2021
	alumni.GraduationYear = &year
	if _, err := store.Onboard(ctx, id, alumni); !errors.Is(err, identity.ErrMembershipConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
}
