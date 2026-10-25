package postgres

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sipratama/alumnibridge/apps/api/internal/auth"
	"github.com/sipratama/alumnibridge/apps/api/internal/identity"
	identitypg "github.com/sipratama/alumnibridge/apps/api/internal/identity/postgres"
	"github.com/sipratama/alumnibridge/apps/api/internal/profiles"
	profileshttp "github.com/sipratama/alumnibridge/apps/api/internal/profiles/http"
)

func TestReviewAuthenticatedAPIAndConcurrency(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	db, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"keys": []any{map[string]string{"kty": "RSA", "kid": "review", "alg": "RS256", "use": "sig", "n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()), "e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes())}}})
	}))
	defer jwks.Close()
	v, err := auth.NewJWTVerifier(ctx, jwks.URL, "https://review.example", "alumnibridge-api", "alumnibridge-web", identitypg.New(db, "https://review.example"))
	if err != nil {
		t.Fatal(err)
	}
	token := func(subject string, exp time.Time) string {
		tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{"sub": subject, "iss": "https://review.example", "aud": "alumnibridge-api", "azp": "alumnibridge-web", "typ": "Bearer", "exp": exp.Unix()})
		tok.Header["kid"] = "review"
		s, err := tok.SignedString(key)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	subjectA, subjectB := uuid.NewString(), uuid.NewString()
	ta, tb := token(subjectA, time.Now().Add(time.Hour)), token(subjectB, time.Now().Add(time.Hour))
	ids := identity.NewService(identitypg.New(db, "https://review.example"))
	store := New(db)
	svc := profiles.New(store)
	r := chi.NewRouter()
	r.Route("/api/v1", func(r chi.Router) { profileshttp.RegisterRoutes(r, svc, ids, v) })
	request := func(method, path, body, tok string, want int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, "/api/v1"+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}
		if method == "PATCH" && path == "/users/me/profile" {
			actor := subjectA
			if tok == tb {
				actor = subjectB
			}
			var ownerID uuid.UUID
			if err := db.QueryRow(ctx, `SELECT id FROM users WHERE oidc_subject=$1`, actor).Scan(&ownerID); err == nil {
				var revision int64
				if err := db.QueryRow(ctx, `SELECT content_revision FROM profiles WHERE user_id=$1`, ownerID).Scan(&revision); err == nil {
					req.Header.Set("If-Match", fmt.Sprintf(`"ab-profile-v%d"`, revision))
				}
			}
		}
		if strings.HasPrefix(path, "/users/me/experiences/") || strings.HasPrefix(path, "/users/me/educations/") {
			kind := "experience"
			table := "profile_experiences"
			if strings.Contains(path, "/educations/") {
				kind = "education"
				table = "profile_educations"
			}
			item, err := uuid.Parse(path[strings.LastIndex(path, "/")+1:])
			if err == nil {
				var rev int64
				err = db.QueryRow(ctx, `SELECT revision FROM `+table+` WHERE id=$1`, item).Scan(&rev)
				if err != nil {
					rev = 1
				}
				req.Header.Set("If-Match", fmt.Sprintf(`"ab-%s-%s-v%d"`, kind, item.String(), rev))
			}
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != want {
			t.Fatalf("%s %s: got %d want %d: %s", method, path, w.Code, want, w.Body.String())
		}
		return w
	}
	request("GET", "/users/me/profile", "", "", 401)
	request("GET", "/users/me/profile", "", "invalid", 401)
	request("GET", "/users/me/profile", "", token(subjectA, time.Now().Add(-time.Hour)), 401)
	firstOwnerRead := request("GET", "/users/me/profile", "", ta, 200)
	request("GET", "/users/me/profile", "", tb, 200)
	firstOwnerETag := firstOwnerRead.Header().Get("ETag")
	if firstOwnerETag == "" {
		t.Fatal("missing original owner ETag")
	}
	ua, _ := ids.Me(ctx, identity.Principal{Subject: subjectA})
	ub, _ := ids.Me(ctx, identity.Principal{Subject: subjectB})
	defer db.Exec(ctx, `DELETE FROM users WHERE id IN ($1,$2)`, ua.ID, ub.ID)
	request("PATCH", "/users/me/profile", `{"display_name":"Alice","headline":"Engineer"}`, ta, 200)
	request("PATCH", "/users/me/profile", `{"display_name":"Bob"}`, tb, 200)
	staleProfile := httptest.NewRequest("PATCH", "/api/v1/users/me/profile", strings.NewReader(`{"headline":"stale override"}`))
	staleProfile.Header.Set("Content-Type", "application/json")
	staleProfile.Header.Set("Authorization", "Bearer "+ta)
	staleProfile.Header.Set("If-Match", firstOwnerETag)
	staleResponse := httptest.NewRecorder()
	r.ServeHTTP(staleResponse, staleProfile)
	if staleResponse.Code != 412 || !strings.Contains(staleResponse.Body.String(), "stale_revision") {
		t.Fatalf("stale signed-JWT owner PATCH must return 412, got %d %s", staleResponse.Code, staleResponse.Body.String())
	}
	request("PATCH", "/users/me/profile", `{"user_id":"`+ub.ID.String()+`","headline":"attack"}`, ta, 400)
	request("PATCH", "/users/me/profile", `{"headline":null}`, ta, 400)
	request("PATCH", "/users/me/profile", `null`, ta, 400)
	request("PATCH", "/users/me/profile", `{"headline":""}`, ta, 200)
	var ex profiles.Experience
	json.Unmarshal(request("POST", "/users/me/experiences", `{"company":"Acme","job_title":"Engineer","start_date":"2020-01-01"}`, ta, 201).Body.Bytes(), &ex)
	request("PATCH", "/users/me/experiences/"+ex.ID.String(), `{"company":"Bad","job_title":"Bad","start_date":"2020-01-01"}`, tb, 404)
	request("DELETE", "/users/me/experiences/"+ex.ID.String(), "", tb, 404)
	request("GET", "/users/me/experiences", "", tb, 200)
	request("PATCH", "/users/me/experiences/"+ex.ID.String(), `{"company":"Good","job_title":"Engineer","start_date":"2020-01-01"}`, ta, 200)
	request("DELETE", "/users/me/experiences/"+ex.ID.String(), "", ta, 204)
	var ed profiles.Education
	json.Unmarshal(request("POST", "/users/me/educations", `{"institution":"Other University"}`, ta, 201).Body.Bytes(), &ed)
	request("PATCH", "/users/me/educations/"+ed.ID.String(), `{"institution":"Bad"}`, tb, 404)
	request("DELETE", "/users/me/educations/"+ed.ID.String(), "", tb, 404)
	request("DELETE", "/users/me/educations/"+ed.ID.String(), "", ta, 204)
	var sk profiles.Skill
	json.Unmarshal(request("POST", "/users/me/skills", `{"name":"Review Skill"}`, ta, 201).Body.Bytes(), &sk)
	request("DELETE", "/users/me/skills/"+sk.ID.String(), "", tb, 404)
	request("DELETE", "/users/me/skills/"+sk.ID.String(), "", ta, 204)
	request("GET", "/skills?query=review&limit=51", "", "", 400)
	// Two users concurrently reserve the same slug: exactly one winner.
	slug := "review-" + uuid.NewString()
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, id := range []uuid.UUID{ua.ID, ub.ID} {
		wg.Add(1)
		go func(id uuid.UUID) {
			defer wg.Done()
			_, err := svc.UpdateProfile(ctx, id, profiles.ProfilePatch{Slug: &slug})
			results <- err
		}(id)
	}
	wg.Wait()
	close(results)
	wins := 0
	for err := range results {
		if err == nil {
			wins++
		} else if err != profiles.ErrConflict {
			t.Fatal(err)
		}
	}
	if wins != 1 {
		t.Fatalf("wins=%d", wins)
	}
	// Different fields patched concurrently must both survive.
	headline, location := "Concurrent headline", "Jakarta"
	for _, patch := range []profiles.ProfilePatch{{Headline: &headline}, {Location: &location}} {
		wg.Add(1)
		go func(patch profiles.ProfilePatch) {
			defer wg.Done()
			if _, err := svc.UpdateProfile(ctx, ua.ID, patch); err != nil {
				t.Error(err)
			}
		}(patch)
	}
	wg.Wait()
	p, err := svc.Profile(ctx, ua.ID)
	if err != nil || p.Headline != headline || p.Location != location {
		t.Fatalf("lost update: %+v %v", p, err)
	}
	// SQL cannot assign another owner's current slug or mutate a reservation.
	other, _ := store.Profile(ctx, ub.ID)
	if _, err = db.Exec(ctx, `UPDATE profiles SET slug=$2 WHERE user_id=$1`, ua.ID, other.Slug); err == nil {
		t.Fatal("cross-owner slug allowed")
	}
	if _, err = db.Exec(ctx, `UPDATE profile_slug_history SET user_id=$2 WHERE slug=$1`, p.Slug, ub.ID); err == nil {
		t.Fatal("reservation reassigned")
	}
	// Rapid successive changes reserve every issued slug.
	for i := 0; i < 5; i++ {
		next := "rapid-" + uuid.NewString()
		if _, err := svc.UpdateProfile(ctx, ua.ID, profiles.ProfilePatch{Slug: &next}); err != nil {
			t.Fatal(err)
		}
		var reserved uuid.UUID
		if err := db.QueryRow(ctx, `SELECT user_id FROM profile_slug_history WHERE slug=$1`, next).Scan(&reserved); err != nil || reserved != ua.ID {
			t.Fatal("rapid change lost reservation", err)
		}
	}
	p, _ = svc.Profile(ctx, ua.ID)
	// Multiple changes preserve old slugs, even after deleting the owner.
	old := p.Slug
	next := "next-" + uuid.NewString()
	if _, err = svc.UpdateProfile(ctx, ua.ID, profiles.ProfilePatch{Slug: &next}); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `DELETE FROM users WHERE id=$1`, ua.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.UpdateProfile(ctx, ub.ID, profiles.ProfilePatch{Slug: &old}); err != profiles.ErrConflict {
		t.Fatalf("deleted owner slug reused: %v", err)
	}
	var owner *uuid.UUID
	if err = db.QueryRow(ctx, `SELECT user_id FROM profile_slug_history WHERE slug=$1`, old).Scan(&owner); err != nil || owner != nil {
		t.Fatalf("missing tombstone %v", err)
	}
}
