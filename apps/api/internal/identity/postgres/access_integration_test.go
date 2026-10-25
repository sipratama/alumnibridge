package postgres

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sipratama/alumnibridge/apps/api/internal/auth"
	"github.com/sipratama/alumnibridge/apps/api/internal/identity"
	identityhttp "github.com/sipratama/alumnibridge/apps/api/internal/identity/http"
	"github.com/sipratama/alumnibridge/apps/api/internal/profiles"
	profileshttp "github.com/sipratama/alumnibridge/apps/api/internal/profiles/http"
)

type ownerProfileProbe struct {
	profiles.Store
	profileCalls int
}

type ownerIdentityProbe struct {
	identity.Store
	provisionCalls int
}

func (s *ownerIdentityProbe) Provision(ctx context.Context, subject, email string) (identity.User, error) {
	s.provisionCalls++
	return s.Store.Provision(ctx, subject, email)
}

func (s *ownerProfileProbe) Profile(_ context.Context, _ uuid.UUID) (profiles.Profile, error) {
	s.profileCalls++
	return profiles.Profile{DisplayName: "Access Test", Slug: "access-test", Revision: 1}, nil
}

func (*ownerProfileProbe) Counts(_ context.Context, _ uuid.UUID) ([3]int, error) {
	return [3]int{}, nil
}

func TestVerifiedTokenApplicationAccess(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	issuer := "https://access-test.example/realms/alumnibridge"
	subject := "access-" + uuid.NewString()
	t.Cleanup(func() {
		cleanup, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		if _, err := db.Exec(cleanup, `DELETE FROM identity_subject_blocks WHERE issuer=$1 AND subject=$2`, issuer, subject); err != nil {
			t.Error(err)
		}
		if _, err := db.Exec(cleanup, `DELETE FROM users WHERE oidc_subject=$1`, subject); err != nil {
			t.Error(err)
		}
		db.Close()
	})
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{map[string]string{
			"kty": "RSA", "kid": "access-test", "use": "sig", "alg": "RS256",
			"n": base64.RawURLEncoding.EncodeToString(key.PublicKey.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.PublicKey.E)).Bytes()),
		}}})
	}))
	defer jwks.Close()
	store := New(db, issuer)
	if err := store.CheckAccess(ctx, "https://different-issuer.example", subject); err == nil {
		t.Fatal("issuer/store mismatch must fail closed")
	}
	verifier, err := auth.NewJWTVerifier(ctx, jwks.URL, issuer, "alumnibridge-api", "alumnibridge-web", store)
	if err != nil {
		t.Fatal(err)
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss": issuer, "sub": subject, "aud": "alumnibridge-api", "azp": "alumnibridge-web",
		"typ": "Bearer", "exp": time.Now().Add(time.Hour).Unix(),
	})
	token.Header["kid"] = "access-test"
	raw, err := token.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	profileProbe := &ownerProfileProbe{}
	identityProbe := &ownerIdentityProbe{Store: store}
	router.Route("/api/v1", func(r chi.Router) {
		identityhttp.RegisterRoutes(r, identity.NewService(store), verifier)
		profileshttp.RegisterRoutes(r, profiles.New(profileProbe), identity.NewService(identityProbe), verifier)
	})
	router.With(auth.Middleware(verifier)).Get("/protected-probe", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) })
	request := func(method, path, body string, want int) {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+raw)
		if method == http.MethodPost {
			req.Header.Set("Content-Type", "application/json")
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != want {
			t.Fatalf("%s %s status=%d body=%s want=%d", method, path, w.Code, w.Body.String(), want)
		}
	}
	request(http.MethodGet, "/api/v1/auth/me", "", 200)
	request(http.MethodGet, "/api/v1/users/me/profile", "", 200)
	if identityProbe.provisionCalls != 1 || profileProbe.profileCalls != 1 {
		t.Fatalf("active owner route did not reach handler and profile service: identity=%d profile=%d", identityProbe.provisionCalls, profileProbe.profileCalls)
	}
	unauthorized := httptest.NewRecorder()
	router.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/api/v1/users/me/profile", nil))
	if unauthorized.Code != 401 || !strings.Contains(unauthorized.Body.String(), `"unauthorized"`) || identityProbe.provisionCalls != 1 || profileProbe.profileCalls != 1 {
		t.Fatalf("owner route without token: status=%d body=%s identity=%d profile=%d", unauthorized.Code, unauthorized.Body.String(), identityProbe.provisionCalls, profileProbe.profileCalls)
	}
	request(http.MethodGet, "/protected-probe", "", 204)
	onboard := `{"role":"student","display_name":"Access Test","institution":"uhamka","study_program":"mathematics-education"}`
	for _, status := range []string{"suspended", "deleting", "deleted"} {
		if _, err := db.Exec(ctx, `UPDATE users SET access_status=$2 WHERE oidc_subject=$1`, subject, status); err != nil {
			t.Fatal(err)
		}
		request(http.MethodGet, "/api/v1/auth/me", "", 403)
		request(http.MethodPost, "/api/v1/auth/onboarding", onboard, 403)
		request(http.MethodGet, "/api/v1/users/me/profile", "", 403)
		if identityProbe.provisionCalls != 1 || profileProbe.profileCalls != 1 {
			t.Fatalf("%s reached owner profile handler after denial: identity=%d profile=%d", status, identityProbe.provisionCalls, profileProbe.profileCalls)
		}
		request(http.MethodGet, "/protected-probe", "", 403)
		if _, err := store.Provision(ctx, subject, "attacker@example.invalid"); !errors.Is(err, identity.ErrAccessDenied) {
			t.Fatalf("status %s permitted direct provision: %v", status, err)
		}
	}
	// A future deletion workflow must insert the block before deleting the user.
	if _, err := db.Exec(ctx, `INSERT INTO identity_subject_blocks(issuer,subject) VALUES($1,$2)`, issuer, subject); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `DELETE FROM users WHERE oidc_subject=$1`, subject); err != nil {
		t.Fatal(err)
	}
	request(http.MethodGet, "/api/v1/auth/me", "", 403)
	if _, err := store.Provision(ctx, subject, "reused@example.invalid"); !errors.Is(err, identity.ErrAccessDenied) {
		t.Fatalf("blocked subject was provisioned: %v", err)
	}
	var count int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM users WHERE oidc_subject=$1`, subject).Scan(&count); err != nil || count != 0 {
		t.Fatalf("deleted subject reappeared: count=%d err=%v", count, err)
	}
	// A database failure must fail closed even while this JWT remains valid.
	failedDB, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	failedDB.Close()
	failedVerifier, err := auth.NewJWTVerifier(ctx, jwks.URL, issuer, "alumnibridge-api", "alumnibridge-web", New(failedDB, issuer))
	if err != nil {
		t.Fatal(err)
	}
	failedRouter := chi.NewRouter()
	failedRouter.Route("/api/v1", func(r chi.Router) {
		identityhttp.RegisterRoutes(r, identity.NewService(store), failedVerifier)
		profileshttp.RegisterRoutes(r, profiles.New(profileProbe), identity.NewService(identityProbe), failedVerifier)
	})
	failedRouter.With(auth.Middleware(failedVerifier)).Get("/protected-probe", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) })
	for _, path := range []string{"/protected-probe", "/api/v1/auth/me", "/api/v1/users/me/profile"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+raw)
		w := httptest.NewRecorder()
		failedRouter.ServeHTTP(w, req)
		if w.Code != 503 || !strings.Contains(w.Body.String(), "access_check_unavailable") {
			t.Fatalf("database outage on %s: status=%d body=%s", path, w.Code, w.Body.String())
		}
	}
	if identityProbe.provisionCalls != 1 || profileProbe.profileCalls != 1 {
		t.Fatalf("database outage reached owner profile handler: identity=%d profile=%d", identityProbe.provisionCalls, profileProbe.profileCalls)
	}
}

func TestProvisionWaitsForIssuerSubjectBlock(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	db, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	issuer := "https://access-test.example/realms/alumnibridge"
	subject := "block-race-" + uuid.NewString()
	t.Cleanup(func() {
		cleanup, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		if _, err := db.Exec(cleanup, `DELETE FROM identity_subject_blocks WHERE issuer=$1 AND subject=$2`, issuer, subject); err != nil {
			t.Error(err)
		}
		if _, err := db.Exec(cleanup, `DELETE FROM users WHERE oidc_subject=$1`, subject); err != nil {
			t.Error(err)
		}
		db.Close()
	})
	workerConfig, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	workerConfig.MaxConns = 1
	worker, err := pgxpool.NewWithConfig(ctx, workerConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer worker.Close()
	var workerPID int
	if err := worker.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&workerPID); err != nil {
		t.Fatal(err)
	}
	w, err := db.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Release()
	tx, err := w.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	// A future lifecycle writer must take exactly this lock before inserting a block.
	if _, err := tx.Exec(ctx, subjectAdvisoryLockSQL, issuer, subject); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO identity_subject_blocks(issuer,subject) VALUES($1,$2)`, issuer, subject); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		_, err := New(worker, issuer).Provision(ctx, subject, "race@example.invalid")
		result <- err
	}()
	// Confirm the second connection is waiting on the advisory lock, not merely
	// scheduled to run after the writer commits.
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for waiting := false; !waiting; {
		select {
		case err := <-result:
			t.Fatalf("provision returned before block commit: %v", err)
		case <-deadline.C:
			t.Fatal("provision never waited on writer's advisory lock")
		case <-ticker.C:
			var waitType string
			if err := db.QueryRow(ctx, `SELECT COALESCE(wait_event_type,'') FROM pg_stat_activity WHERE pid=$1`, workerPID).Scan(&waitType); err != nil {
				t.Fatal(err)
			}
			waiting = waitType == "Lock"
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-result; !errors.Is(err, identity.ErrAccessDenied) {
		t.Fatalf("provision after block commit: %v; want access denied", err)
	}
	if _, err := New(worker, issuer).Provision(ctx, subject, "again@example.invalid"); !errors.Is(err, identity.ErrAccessDenied) {
		t.Fatalf("repeated provision after block commit: %v; want access denied", err)
	}
	var active int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM users WHERE oidc_subject=$1 AND access_status='active'`, subject).Scan(&active); err != nil || active != 0 {
		t.Fatalf("active account after block commit: count=%d err=%v", active, err)
	}
}

func TestBlockWriterWaitsForFirstProvision(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	db, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	issuer := "https://access-test.example/realms/alumnibridge"
	subject := "provision-first-" + uuid.NewString()
	t.Cleanup(func() {
		cleanup, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		if _, err := db.Exec(cleanup, `DELETE FROM identity_subject_blocks WHERE issuer=$1 AND subject=$2`, issuer, subject); err != nil {
			t.Error(err)
		}
		if _, err := db.Exec(cleanup, `DELETE FROM users WHERE oidc_subject=$1`, subject); err != nil {
			t.Error(err)
		}
		db.Close()
	})
	config, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	config.MaxConns = 1
	provisionDB, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer provisionDB.Close()
	writerDB, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer writerDB.Close()
	var provisionPID, writerPID int
	if err := provisionDB.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&provisionPID); err != nil {
		t.Fatal(err)
	}
	if err := writerDB.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&writerPID); err != nil {
		t.Fatal(err)
	}
	// Hold the users table so Provision owns the advisory lock but cannot commit yet.
	hold, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer hold.Rollback(ctx)
	if _, err := hold.Exec(ctx, `LOCK TABLE users IN SHARE MODE`); err != nil {
		t.Fatal(err)
	}
	provisionDone := make(chan error, 1)
	go func() {
		_, err := New(provisionDB, issuer).Provision(ctx, subject, "first@example.invalid")
		provisionDone <- err
	}()
	waitForAccessLock(t, ctx, db, provisionPID)
	writerLocked := make(chan struct{})
	releaseWriter := make(chan struct{})
	writerDone := make(chan error, 1)
	go func() {
		tx, err := writerDB.Begin(ctx)
		if err != nil {
			writerDone <- err
			return
		}
		defer tx.Rollback(ctx)
		if _, err = tx.Exec(ctx, subjectAdvisoryLockSQL, issuer, subject); err != nil {
			writerDone <- err
			return
		}
		close(writerLocked)
		select {
		case <-releaseWriter:
		case <-ctx.Done():
			writerDone <- ctx.Err()
			return
		}
		// The synthetic lifecycle writer must block the now-existing row in its
		// own transaction before committing the issuer/subject marker.
		if _, err = tx.Exec(ctx, `UPDATE users SET access_status='deleted' WHERE oidc_subject=$1`, subject); err != nil {
			writerDone <- err
			return
		}
		if _, err = tx.Exec(ctx, `INSERT INTO identity_subject_blocks(issuer,subject) VALUES($1,$2)`, issuer, subject); err != nil {
			writerDone <- err
			return
		}
		writerDone <- tx.Commit(ctx)
	}()
	waitForAccessLock(t, ctx, db, writerPID)
	if err := hold.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-provisionDone; err != nil {
		t.Fatalf("first provision: %v", err)
	}
	select {
	case <-writerLocked:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	var active int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM users WHERE oidc_subject=$1 AND access_status='active'`, subject).Scan(&active); err != nil || active != 1 {
		t.Fatalf("provision did not win first: %d %v", active, err)
	}
	close(releaseWriter)
	if err := <-writerDone; err != nil {
		t.Fatalf("synthetic writer: %v", err)
	}
	if _, err := New(provisionDB, issuer).Provision(ctx, subject, "second@example.invalid"); !errors.Is(err, identity.ErrAccessDenied) {
		t.Fatalf("post-block provision: %v", err)
	}
	if err := db.QueryRow(ctx, `SELECT count(*) FROM users WHERE oidc_subject=$1 AND access_status='active'`, subject).Scan(&active); err != nil || active != 0 {
		t.Fatalf("active user after block commit: %d %v", active, err)
	}
}

func waitForAccessLock(t *testing.T, ctx context.Context, db *pgxpool.Pool, pid int) {
	t.Helper()
	for {
		var waitType string
		if err := db.QueryRow(ctx, `SELECT COALESCE(wait_event_type,'') FROM pg_stat_activity WHERE pid=$1`, pid).Scan(&waitType); err != nil {
			t.Fatal(err)
		}
		if waitType == "Lock" {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("backend did not wait on a lock: ", ctx.Err())
		case <-time.After(10 * time.Millisecond):
		}
	}
}
