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
)

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
	identityhttp.Register(router, identity.NewService(store), verifier)
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
	request(http.MethodGet, "/protected-probe", "", 204)
	onboard := `{"role":"student","display_name":"Access Test","institution":"uhamka","study_program":"mathematics-education"}`
	for _, status := range []string{"suspended", "deleting", "deleted"} {
		if _, err := db.Exec(ctx, `UPDATE users SET access_status=$2 WHERE oidc_subject=$1`, subject, status); err != nil {
			t.Fatal(err)
		}
		request(http.MethodGet, "/api/v1/auth/me", "", 403)
		request(http.MethodPost, "/api/v1/auth/onboarding", onboard, 403)
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
	identityhttp.Register(failedRouter, identity.NewService(store), failedVerifier)
	failedRouter.With(auth.Middleware(failedVerifier)).Get("/protected-probe", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) })
	for _, path := range []string{"/protected-probe", "/api/v1/auth/me"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+raw)
		w := httptest.NewRecorder()
		failedRouter.ServeHTTP(w, req)
		if w.Code != 503 || !strings.Contains(w.Body.String(), "access_check_unavailable") {
			t.Fatalf("database outage on %s: status=%d body=%s", path, w.Code, w.Body.String())
		}
	}
}
