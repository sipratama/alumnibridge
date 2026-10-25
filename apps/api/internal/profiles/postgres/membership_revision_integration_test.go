package postgres

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sipratama/alumnibridge/apps/api/internal/identity"
	identitypg "github.com/sipratama/alumnibridge/apps/api/internal/identity/postgres"
	"github.com/sipratama/alumnibridge/apps/api/internal/profiles"
	profileshttp "github.com/sipratama/alumnibridge/apps/api/internal/profiles/http"
)

func TestMembershipRevisionInvalidation(t *testing.T) {
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
	owner, secondProgram := uuid.New(), uuid.New()
	subject := "membership-revision-" + owner.String()
	var firstProgram, institution uuid.UUID
	if err := db.QueryRow(ctx, `SELECT id,institution_id FROM study_programs LIMIT 1`).Scan(&firstProgram, &institution); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO study_programs(id,institution_id,slug,name) VALUES($1,$2,$3,'Revision program')`, secondProgram, institution, "revision-"+owner.String()); err != nil {
		t.Fatal(err)
	}
	defer db.Exec(ctx, `DELETE FROM study_programs WHERE id=$1`, secondProgram)
	if _, err := db.Exec(ctx, `INSERT INTO users(id,oidc_subject,display_name) VALUES($1,$2,'Revision Owner')`, owner, subject); err != nil {
		t.Fatal(err)
	}
	defer db.Exec(ctx, `DELETE FROM users WHERE id=$1`, owner)
	first, second := uuid.New(), uuid.New()
	if _, err := db.Exec(ctx, `INSERT INTO memberships(id,user_id,study_program_id,role,status,graduation_year)
        VALUES($1,$2,$3,'alumni','verified',2020),($4,$2,$5,'alumni','verified',2021)`, first, owner, firstProgram, second, secondProgram); err != nil {
		t.Fatal(err)
	}
	slug := "revision-" + owner.String()
	if _, err := db.Exec(ctx, `INSERT INTO profile_slug_history(slug,user_id) VALUES($1,$2)`, slug, owner); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO profiles(user_id,slug,visibility,directory_discoverable,directory_consented_at)
        VALUES($1,$2,'public',true,now())`, owner, slug); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO profile_directory_affiliations(membership_id,show_institution)
        VALUES($1,true),($2,true)`, first, second); err != nil {
		t.Fatal(err)
	}

	verifier, sign := publicationJWT(t, ctx, db)
	token := sign(subject, time.Now().Add(time.Hour))
	store := New(db)
	router := chi.NewRouter()
	router.Route("/api/v1", func(r chi.Router) {
		profileshttp.RegisterPublishingRoutes(r, profiles.New(store), profiles.NewPublishing(store, nil), identity.NewService(identitypg.New(db, "https://publication.example")), verifier)
		profileshttp.RegisterDiscoveryRoutes(r, profiles.New(store), profiles.NewDiscovery(store), identity.NewService(identitypg.New(db, "https://publication.example")), verifier)
	})
	request := func(method, path, body, etag string, want int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, "/api/v1"+path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		if etag != "" {
			req.Header.Set("If-Match", etag)
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Fatalf("%s %s: got %d want %d: %s", method, path, rec.Code, want, rec.Body.String())
		}
		return rec
	}
	path := "/users/me/profile/discovery"
	original := request(http.MethodGet, path, "", "", 200)
	oldTag := original.Header().Get("ETag")
	if oldTag == "" || !strings.Contains(original.Body.String(), first.String()) || !strings.Contains(original.Body.String(), second.String()) {
		t.Fatalf("expected two eligible memberships and owner ETag: %s %s", oldTag, original.Body.String())
	}
	if _, err := db.Exec(ctx, `UPDATE memberships SET status='pending' WHERE id=$1`, first); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM profile_directory_affiliations WHERE membership_id=$1`, first).Scan(&count); err != nil || count != 0 {
		t.Fatalf("revoked affiliation retained: %d %v", count, err)
	}
	current := request(http.MethodGet, path, "", "", 200)
	newTag := current.Header().Get("ETag")
	if newTag == oldTag || strings.Contains(current.Body.String(), first.String()) || !strings.Contains(current.Body.String(), second.String()) {
		t.Fatalf("membership transition did not change owner state/revision: %s -> %s %s", oldTag, newTag, current.Body.String())
	}
	request(http.MethodPatch, path, `{"directory_discoverable":true}`, oldTag, 412)
	request(http.MethodPatch, "/users/me/profile/privacy", `{"visibility":"private"}`, oldTag, 412)
	request(http.MethodPatch, path, `{"directory_discoverable":true}`, newTag, 200)
	if err := db.QueryRow(ctx, `SELECT count(*) FROM profile_directory_affiliations WHERE membership_id=$1`, first).Scan(&count); err != nil || count != 0 {
		t.Fatalf("stale affiliation restored: %d %v", count, err)
	}

	// A failed transaction must not publish a new validator or revoke consent.
	beforeRollback := request(http.MethodGet, path, "", "", 200).Header().Get("ETag")
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `UPDATE memberships SET status='pending' WHERE id=$1`, second); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if got := request(http.MethodGet, path, "", "", 200).Header().Get("ETag"); got != beforeRollback {
		t.Fatalf("rollback advanced revision: %s -> %s", beforeRollback, got)
	}

	if _, err := db.Exec(ctx, `DELETE FROM memberships WHERE id=$1`, second); err != nil {
		t.Fatal(err)
	}
	loss := request(http.MethodGet, path, "", "", 200)
	if loss.Header().Get("ETag") == beforeRollback || !strings.Contains(loss.Body.String(), `"directory_discoverable":false`) {
		t.Fatalf("last loss failed to revoke: %s", loss.Body.String())
	}
	request(http.MethodPatch, path, `{"directory_discoverable":true}`, beforeRollback, 412)
	if _, err := db.Exec(ctx, `UPDATE memberships SET status='verified' WHERE id=$1`, first); err != nil {
		t.Fatal(err)
	}
	restored := request(http.MethodGet, path, "", "", 200)
	if !strings.Contains(restored.Body.String(), `"directory_discoverable":false`) || restored.Header().Get("ETag") == loss.Header().Get("ETag") {
		t.Fatalf("reverification restored consent or failed to bump revision: %s", restored.Body.String())
	}
	request(http.MethodPatch, path, `{"directory_discoverable":true}`, loss.Header().Get("ETag"), 412)
	request(http.MethodPatch, path, `{"directory_discoverable":true}`, restored.Header().Get("ETag"), 200)
	beforeYearChange := request(http.MethodGet, path, "", "", 200).Header().Get("ETag")

	// A stale REPEATABLE READ transaction on another connection must fail
	// with 40001, allowing a full-transaction retry rather than stale consent.
	a, err := db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Rollback(ctx)
	b, err := db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Rollback(ctx)
	for _, transaction := range []pgx.Tx{a, b} {
		if err := transaction.QueryRow(ctx, `SELECT count(*) FROM memberships WHERE user_id=$1`, owner).Scan(&count); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := a.Exec(ctx, `UPDATE memberships SET graduation_year=2022 WHERE id=$1`, first); err != nil {
		t.Fatal(err)
	}
	if err := a.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	afterYearChange := request(http.MethodGet, path, "", "", 200).Header().Get("ETag")
	if afterYearChange == beforeYearChange {
		t.Fatal("verified affiliation detail change left discovery ETag valid")
	}
	request(http.MethodPatch, path, `{"directory_discoverable":true}`, beforeYearChange, 412)
	_, err = b.Exec(ctx, `UPDATE memberships SET graduation_year=2023 WHERE id=$1`, first)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "40001" {
		t.Fatalf("stale snapshot was accepted: %v", err)
	}
	if err := b.Rollback(ctx); err != nil {
		t.Fatal(err)
	}

	// Independent connections racing over the last two verified memberships
	// cannot both commit while leaving an active directory opt-in.
	if _, err := db.Exec(ctx, `UPDATE memberships SET status='verified' WHERE id=$1`, first); err != nil {
		t.Fatal(err)
	}
	third := uuid.New()
	if _, err := db.Exec(ctx, `INSERT INTO memberships(id,user_id,study_program_id,role,status,graduation_year) VALUES($1,$2,$3,'alumni','verified',2024)`, third, owner, secondProgram); err != nil {
		t.Fatal(err)
	}
	fresh := request(http.MethodGet, path, "", "", 200).Header().Get("ETag")
	if fresh == afterYearChange {
		t.Fatal("verified membership insertion left discovery ETag valid")
	}
	request(http.MethodPatch, path, `{"directory_discoverable":true}`, fresh, 200)
	var wg sync.WaitGroup
	failures := make(chan error, 2)
	for _, membership := range []uuid.UUID{first, third} {
		wg.Add(1)
		go func(id uuid.UUID) {
			defer wg.Done()
			_, e := db.Exec(ctx, `UPDATE memberships SET status='pending' WHERE id=$1`, id)
			failures <- e
		}(membership)
	}
	wg.Wait()
	close(failures)
	for e := range failures {
		if e != nil {
			t.Fatal(e)
		}
	}
	final := request(http.MethodGet, path, "", "", 200)
	if !strings.Contains(final.Body.String(), `"directory_discoverable":false`) {
		t.Fatalf("concurrent last loss preserved consent: %s", final.Body.String())
	}
}
