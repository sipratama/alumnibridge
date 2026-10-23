package postgres

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sipratama/alumnibridge/apps/api/internal/profiles"
)

func TestProfileStorage(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	a, b := uuid.New(), uuid.New()
	for _, v := range []struct {
		id            uuid.UUID
		subject, name string
	}{{a, "profile-a-" + a.String(), "Jane Doe"}, {b, "profile-b-" + b.String(), "Jane Doe"}} {
		_, err = db.Exec(ctx, `INSERT INTO users(id,oidc_subject,display_name) VALUES($1,$2,$3)`, v.id, v.subject, v.name)
		if err != nil {
			t.Fatal(err)
		}
	}
	defer db.Exec(ctx, `DELETE FROM users WHERE id IN ($1,$2)`, a, b)
	s := New(db)
	pa, err := s.Profile(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.Profile(ctx, a)
	if err != nil || again.Slug != pa.Slug {
		t.Fatalf("idempotence: %v %v", again, err)
	}
	pb, err := s.Profile(ctx, b)
	if err != nil {
		t.Fatal(err)
	}
	if pa.Slug == pb.Slug {
		t.Fatal("slug collision")
	}
	pb.Slug = pa.Slug
	if _, err = s.UpdateProfile(ctx, b, pb); !errors.Is(err, profiles.ErrConflict) {
		t.Fatalf("collision: %v", err)
	}
	pa.Slug = "jane-new"
	if _, err = s.UpdateProfile(ctx, a, pa); err != nil {
		t.Fatal(err)
	}
	var reserved uuid.UUID
	if err = db.QueryRow(ctx, `SELECT user_id FROM profile_slug_history WHERE slug=$1`, "jane-doe").Scan(&reserved); err != nil || reserved != a {
		t.Fatalf("old slug lost: %v", err)
	}
	e, err := s.SaveExperience(ctx, a, profiles.Experience{Company: "Acme", JobTitle: "Engineer", StartDate: "2020-01-01"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if lenMustExperience(t, s, ctx, a) != 1 {
		t.Fatal("experience missing")
	}
	if err = s.DeleteExperience(ctx, b, e.ID); !errors.Is(err, profiles.ErrNotFound) {
		t.Fatalf("cross-user delete: %v", err)
	}
	e.Company = "Changed by another user"
	if _, err = s.SaveExperience(ctx, b, e, true); !errors.Is(err, profiles.ErrNotFound) {
		t.Fatalf("cross-user update: %v", err)
	}
	if err = s.DeleteExperience(ctx, a, e.ID); err != nil {
		t.Fatal(err)
	}
	ed, err := s.SaveEducation(ctx, a, profiles.Education{Institution: "Another University"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.DeleteEducation(ctx, b, ed.ID); !errors.Is(err, profiles.ErrNotFound) {
		t.Fatal(err)
	}
	ed.Institution = "Changed by another user"
	if _, err = s.SaveEducation(ctx, b, ed, true); !errors.Is(err, profiles.ErrNotFound) {
		t.Fatalf("cross-user education update: %v", err)
	}
	if err = s.DeleteEducation(ctx, a, ed.ID); err != nil {
		t.Fatal(err)
	}
	skill, err := s.AddSkill(ctx, a, "Data Science", "data science")
	if err != nil {
		t.Fatal(err)
	}
	againSkill, err := s.AddSkill(ctx, a, "data science", "data science")
	if err != nil || againSkill.ID != skill.ID {
		t.Fatalf("duplicate skill: %v", err)
	}
	if err = s.DeleteSkill(ctx, b, skill.ID); !errors.Is(err, profiles.ErrNotFound) {
		t.Fatal(err)
	}
	if err = s.DeleteSkill(ctx, a, skill.ID); err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(ctx, `INSERT INTO profile_experiences(user_id,company,title,start_date,end_date) VALUES($1,'Acme','Engineer','2024-01-01','2023-01-01')`, a)
	if err == nil {
		t.Fatal("database accepted experience with reversed dates")
	}
	_, err = db.Exec(ctx, `INSERT INTO profile_educations(user_id,institution_name,start_date,end_date) VALUES($1,'Another University','2024-01-01','2023-01-01')`, a)
	if err == nil {
		t.Fatal("database accepted education with reversed dates")
	}
}
func lenMustExperience(t *testing.T, s *Store, ctx context.Context, id uuid.UUID) int {
	t.Helper()
	v, err := s.Experiences(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	return len(v)
}
