package postgres_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sipratama/alumnibridge/apps/api/internal/auth"
	"github.com/sipratama/alumnibridge/apps/api/internal/directory"
	directoryhttp "github.com/sipratama/alumnibridge/apps/api/internal/directory/http"
	directorypg "github.com/sipratama/alumnibridge/apps/api/internal/directory/postgres"
	"github.com/sipratama/alumnibridge/apps/api/internal/identity"
	identitypg "github.com/sipratama/alumnibridge/apps/api/internal/identity/postgres"
	"github.com/sipratama/alumnibridge/apps/api/internal/profiles"
	profileshttp "github.com/sipratama/alumnibridge/apps/api/internal/profiles/http"
	profilespg "github.com/sipratama/alumnibridge/apps/api/internal/profiles/postgres"
)

type fixture struct {
	id, membership uuid.UUID
	token, slug    string
}

func TestDirectoryConsentPrivacyAndSignedJWT(t *testing.T) {
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL required; disposable database only")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	db, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := db.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{map[string]string{"kty": "RSA", "kid": "directory", "alg": "RS256", "use": "sig", "n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()), "e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes())}}})
	}))
	defer jwks.Close()
	verifier, err := auth.NewJWTVerifier(ctx, jwks.URL, "https://directory.example", "alumnibridge-api", "alumnibridge-web", identitypg.New(db, "https://directory.example"))
	if err != nil {
		t.Fatal(err)
	}
	sign := func(subject, audience string, expiry time.Time) string {
		t.Helper()
		tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{"sub": subject, "iss": "https://directory.example", "aud": audience, "azp": "alumnibridge-web", "typ": "Bearer", "exp": expiry.Unix()})
		tok.Header["kid"] = "directory"
		signed, err := tok.SignedString(key)
		if err != nil {
			t.Fatal(err)
		}
		return signed
	}
	ids := identity.NewService(identitypg.New(db, "https://directory.example"))
	ps := profilespg.New(db)
	profile := profiles.New(ps)
	discovery := profiles.NewDiscovery(ps)
	codec, _ := directory.NewCursor(make([]byte, 32))
	search := directory.New(directorypg.New(db), codec)
	router := chi.NewRouter()
	router.Route("/api/v1", func(r chi.Router) {
		limits := directoryhttp.DefaultLimits()
		limits.Burst = 1000
		limits.ClientTTL = 20 * time.Minute
		directoryhttp.RegisterRoutesWithLimits(r, search, limits, slog.Default())
		profileshttp.RegisterDiscoveryRoutes(r, profile, discovery, ids, verifier)
		profileshttp.RegisterPublishingRoutes(r, profile, profiles.NewPublishing(ps, nil), ids, verifier)
	})
	call := func(method, path, body, token string, want int, ifMatch ...string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, "/api/v1"+path, strings.NewReader(body))
		req = req.WithContext(ctx)
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		if method == "PATCH" && want != 401 && (path == "/users/me/profile/privacy" || path == "/users/me/profile/discovery") {
			if len(ifMatch) != 0 {
				if ifMatch[0] != "" {
					req.Header.Set("If-Match", ifMatch[0])
				}
			} else {
				ownerGet := httptest.NewRequest("GET", "/api/v1"+path, nil).WithContext(ctx)
				ownerGet.Header.Set("Authorization", "Bearer "+token)
				ownerResponse := httptest.NewRecorder()
				router.ServeHTTP(ownerResponse, ownerGet)
				if ownerResponse.Code != 200 || ownerResponse.Header().Get("ETag") == "" {
					t.Fatalf("owner GET %s did not supply an ETag: %d %s", path, ownerResponse.Code, ownerResponse.Body.String())
				}
				req.Header.Set("If-Match", ownerResponse.Header().Get("ETag"))
			}
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != want {
			t.Fatalf("%s %s got %d want %d: %s", method, path, w.Code, want, w.Body.String())
		}
		if (strings.Contains(path, "/discovery") || strings.HasPrefix(path, "/directory") || strings.HasPrefix(path, "/profiles/")) && (w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Content-Type-Options") != "nosniff") {
			t.Fatalf("unsafe headers: %v", w.Header())
		}
		return w
	}
	ownerPath := "/users/me/profile/discovery"
	for _, token := range []string{"", "invalid", sign("expired", "alumnibridge-api", time.Now().Add(-time.Hour)), sign("wrong-audience", "other", time.Now().Add(time.Hour))} {
		call("GET", ownerPath, "", token, 401)
		call("PATCH", ownerPath, `{"directory_discoverable":true}`, token, 401)
	}
	run := uuid.NewString()[:8]
	namePrefix := "M4B " + run
	var program uuid.UUID
	if err = db.QueryRow(ctx, `SELECT id FROM study_programs WHERE slug='mathematics-education'`).Scan(&program); err != nil {
		t.Fatal(err)
	}
	institution2, program2, program3 := uuid.New(), uuid.New(), uuid.New()
	exec(`INSERT INTO institutions(id,slug,name) VALUES($1,$2,'Other University')`, institution2, "m4b-"+run)
	exec(`INSERT INTO study_programs(id,institution_id,slug,name) VALUES($1,$2,'other','Other program'),($3,$2,'pending','Pending program')`, program2, institution2, program3)
	defer func() {
		db.Exec(context.Background(), `DELETE FROM study_programs WHERE id IN ($1,$2)`, program2, program3)
		db.Exec(context.Background(), `DELETE FROM institutions WHERE id=$1`, institution2)
	}()
	var fixtures []fixture
	defer func() {
		for _, f := range fixtures {
			if _, err := db.Exec(context.Background(), `DELETE FROM users WHERE id=$1`, f.id); err != nil {
				t.Error(err)
			}
		}
	}()
	makeFixture := func(label, role, status string) fixture {
		t.Helper()
		subject := uuid.NewString()
		u, err := ids.Me(ctx, identity.Principal{Subject: subject, Email: "private-" + subject + "@example.invalid"})
		if err != nil {
			t.Fatal(err)
		}
		f := fixture{id: u.ID, token: sign(subject, "alumnibridge-api", time.Now().Add(time.Hour)), slug: "m4b-" + run + "-" + label}
		fixtures = append(fixtures, f)
		if _, err = profile.Profile(ctx, u.ID); err != nil {
			t.Fatal(err)
		}
		name := namePrefix + " " + label
		if _, err = profile.UpdateProfile(ctx, u.ID, profiles.ProfilePatch{Slug: &f.slug, DisplayName: &name}); err != nil {
			t.Fatal(err)
		}
		year := any(2020)
		if role == "student" {
			year = nil
		}
		if err = db.QueryRow(ctx, `INSERT INTO memberships(user_id,study_program_id,role,status,graduation_year) VALUES($1,$2,$3,$4,$5) RETURNING id`, u.ID, program, role, status, year).Scan(&f.membership); err != nil {
			t.Fatal(err)
		}
		return f
	}
	student := makeFixture("student", "student", "active")
	pending := makeFixture("pending", "alumni", "pending")
	rejected := makeFixture("rejected", "alumni", "rejected")
	noConsent := makeFixture("no-consent", "alumni", "verified")
	private := makeFixture("private", "alumni", "verified")
	hidden := makeFixture("hidden", "alumni", "verified")
	a := makeFixture("alpha", "alumni", "verified")
	b := makeFixture("bravo", "alumni", "verified")
	c := makeFixture("charlie", "alumni", "verified")
	for _, f := range []fixture{student, pending, rejected, noConsent, hidden, a, b, c} {
		call("PATCH", "/users/me/profile/privacy", `{"visibility":"public"}`, f.token, 200)
	}
	exec(`UPDATE profiles SET moderation_status='hidden' WHERE user_id=$1`, hidden.id)
	for _, f := range []fixture{student, pending, rejected, private, hidden} {
		call("PATCH", ownerPath, `{"directory_discoverable":true}`, f.token, 409)
	}
	var defaults profiles.Discovery
	json.Unmarshal(call("GET", ownerPath, "", a.token, 200).Body.Bytes(), &defaults)
	if defaults.DirectoryDiscoverable || defaults.EffectiveDiscoverable || defaults.DirectoryConsentedAt != nil || !defaults.Eligible || len(defaults.Memberships) != 1 || defaults.Memberships[0].ShowInstitution {
		t.Fatalf("defaults: %+v", defaults)
	}
	originalETag := call("GET", ownerPath, "", a.token, 200).Header().Get("ETag")
	if originalETag == "" {
		t.Fatal("missing original discovery ETag")
	}
	call("PATCH", ownerPath, `{"directory_discoverable":false}`, a.token, 428, "")
	call("PATCH", ownerPath, `{"directory_discoverable":false}`, a.token, 200, originalETag)
	call("PATCH", ownerPath, `{"directory_discoverable":false}`, a.token, 412, originalETag)
	searchPage := func(extra, token string) directory.Page {
		t.Helper()
		w := call("GET", "/directory?query="+url.QueryEscape(namePrefix)+extra, "", token, 200)
		var p directory.Page
		if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil {
			t.Fatal(err)
		}
		for _, secret := range []string{"user_id", "membership_id", "oidc_subject", "email", "moderation_status", "directory_consented_at", "photo_object_key", a.id.String(), a.membership.String()} {
			if strings.Contains(w.Body.String(), secret) {
				t.Fatalf("public leak: %s", w.Body.String())
			}
		}
		return p
	}
	if p := searchPage("", ""); len(p.Items) != 0 || p.NextCursor != nil {
		t.Fatal(p)
	}
	// Read predicates remain mandatory even if privileged SQL sets stale consent on an unverified owner.
	for _, f := range []fixture{student, pending, rejected} {
		exec(`UPDATE profiles SET directory_discoverable=true,directory_consented_at=now() WHERE user_id=$1`, f.id)
	}
	if p := searchPage("", ""); len(p.Items) != 0 {
		t.Fatal("unverified persisted consent bypassed SQL policy", p)
	}
	for _, f := range []fixture{student, pending, rejected} {
		exec(`UPDATE profiles SET directory_discoverable=false WHERE user_id=$1`, f.id)
	}

	// Database constraints reject incomplete timestamps, dangling references and dependent flags.
	if _, err = db.Exec(ctx, `UPDATE profiles SET directory_discoverable=true WHERE user_id=$1`, a.id); err == nil {
		t.Fatal("timestamp constraint bypassed")
	}
	if _, err = db.Exec(ctx, `INSERT INTO profile_directory_affiliations(membership_id,show_study_program) VALUES($1,true)`, a.membership); err == nil {
		t.Fatal("academic dependency constraint bypassed")
	}
	if _, err = db.Exec(ctx, `INSERT INTO profile_directory_affiliations(membership_id) VALUES($1)`, uuid.New()); err == nil {
		t.Fatal("membership FK bypassed")
	}
	for _, f := range []fixture{a, b, c} {
		call("PATCH", ownerPath, `{"directory_discoverable":true}`, f.token, 200)
	}
	p := searchPage("&limit=2", "")
	if len(p.Items) != 2 || p.Items[0].Slug != a.slug || p.Items[1].Slug != b.slug || p.NextCursor == nil {
		t.Fatalf("page1 %+v", p)
	}
	if signed := searchPage("&limit=2", a.token); len(signed.Items) != len(p.Items) || signed.Items[0].Slug != p.Items[0].Slug {
		t.Fatal("authenticated projection differs")
	}
	next := searchPage("&limit=2&cursor="+url.QueryEscape(*p.NextCursor), "")
	if len(next.Items) != 1 || next.Items[0].Slug != c.slug || next.NextCursor != nil {
		t.Fatalf("page2 %+v", next)
	}
	call("GET", "/directory?query="+url.QueryEscape(namePrefix)+"&limit=3&cursor="+url.QueryEscape(*p.NextCursor), "", "", 400)
	call("GET", "/directory?query=Other&limit=2&cursor="+url.QueryEscape(*p.NextCursor), "", "", 400)
	for _, query := range []string{"limit=51", "foo=x", "limit=1&limit=2", "query=", "study_program=other", "skills=go&skills=GO", "cursor=not-a-cursor"} {
		call("GET", "/directory?"+query, "", "", 400)
	}
	// An anchor is only a lexical boundary; hidden/deleted anchors need no lookup.
	exec(`UPDATE profiles SET moderation_status='hidden' WHERE user_id=$1`, b.id)
	for _, item := range searchPage("", "").Items {
		if item.Slug == b.slug {
			t.Fatal("hidden opted-in profile leaked")
		}
	}

	next = searchPage("&limit=2&cursor="+url.QueryEscape(*p.NextCursor), "")
	if len(next.Items) != 1 || next.Items[0].Slug != c.slug {
		t.Fatal(next)
	}
	exec(`DELETE FROM users WHERE id=$1`, b.id)
	if next = searchPage("&limit=2&cursor="+url.QueryEscape(*p.NextCursor), ""); len(next.Items) != 1 {
		t.Fatal(next)
	}
	// Authenticated references are owner scoped and replacement failure is atomic.
	consent := func(id uuid.UUID, inst, prog, year bool) string {
		return fmt.Sprintf(`{"membership_id":%q,"show_institution":%t,"show_study_program":%t,"show_graduation_year":%t}`, id.String(), inst, prog, year)
	}
	replace := func(entries ...string) string { return `{"affiliations":[` + strings.Join(entries, ",") + `]}` }
	call("PATCH", ownerPath, replace(consent(a.membership, true, true, true)), a.token, 200)
	before := call("GET", ownerPath, "", a.token, 200).Body.String()
	call("PATCH", ownerPath, replace(consent(c.membership, true, true, true)), a.token, 404)
	call("PATCH", ownerPath, replace(consent(uuid.New(), true, true, true)), a.token, 404)
	if after := call("GET", ownerPath, "", a.token, 200).Body.String(); before != after {
		t.Fatal("failed replacement changed consent")
	}
	// A database failure after the profile write rolls the entire consent transaction back.
	exec(`CREATE FUNCTION m4b_reject_affiliation() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic consent failure'; END $$;
 CREATE TRIGGER m4b_reject_affiliation BEFORE INSERT ON profile_directory_affiliations FOR EACH ROW EXECUTE FUNCTION m4b_reject_affiliation()`)
	defer db.Exec(context.Background(), `DROP TRIGGER IF EXISTS m4b_reject_affiliation ON profile_directory_affiliations; DROP FUNCTION IF EXISTS m4b_reject_affiliation()`)
	failed := call("PATCH", ownerPath, replace(consent(a.membership, true, false, false)), a.token, 500)
	if strings.Contains(failed.Body.String(), "synthetic") || strings.Contains(failed.Body.String(), "SQL") {
		t.Fatal("database detail leaked")
	}
	exec(`DROP TRIGGER m4b_reject_affiliation ON profile_directory_affiliations; DROP FUNCTION m4b_reject_affiliation()`)
	if after := call("GET", ownerPath, "", a.token, 200).Body.String(); before != after {
		t.Fatal("SQL failure did not roll back consent")
	}

	call("PATCH", ownerPath, replace(consent(a.membership, false, true, false)), a.token, 400)
	call("PATCH", ownerPath, replace(consent(a.membership, true, false, false), consent(a.membership, true, false, false)), a.token, 400)
	many := make([]string, 21)
	for i := range many {
		many[i] = consent(uuid.New(), true, false, false)
	}
	call("PATCH", ownerPath, replace(many...), a.token, 400)
	var second, pendingID uuid.UUID
	if err = db.QueryRow(ctx, `INSERT INTO memberships(user_id,study_program_id,role,status,graduation_year) VALUES($1,$2,'alumni','verified',2021) RETURNING id`, a.id, program2).Scan(&second); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(ctx, `INSERT INTO memberships(user_id,study_program_id,role,status,graduation_year) VALUES($1,$2,'alumni','pending',2022) RETURNING id`, a.id, program3).Scan(&pendingID); err != nil {
		t.Fatal(err)
	}
	call("PATCH", ownerPath, replace(consent(pendingID, true, true, true)), a.token, 409)
	// A stale privileged consent record must still not make a pending affiliation searchable/public.
	exec(`INSERT INTO profile_directory_affiliations(membership_id,show_institution,show_study_program,show_graduation_year) VALUES($1,true,true,true)`, pendingID)
	if p := searchPage("&institution=m4b-"+run+"&study_program=pending&graduation_year=2022", ""); len(p.Items) != 0 {
		t.Fatal("pending affiliation borrowed verification", p)
	}

	call("PATCH", ownerPath, replace(consent(a.membership, true, true, true), consent(second, true, true, true)), a.token, 200)
	if p := searchPage("&institution=uhamka&study_program=mathematics-education&graduation_year=2020", ""); len(p.Items) != 1 || p.Items[0].Slug != a.slug || !p.Items[0].VerifiedAlumni {
		t.Fatal(p)
	}
	if p := searchPage("&institution=uhamka&graduation_year=2021", ""); len(p.Items) != 0 {
		t.Fatal("mixed memberships matched", p)
	}
	if p := searchPage("&graduation_year=2022", ""); len(p.Items) != 0 {
		t.Fatal("pending membership matched", p)
	}
	// Partial academic flags control both projection and matching.
	call("PATCH", ownerPath, replace(consent(a.membership, true, false, false)), a.token, 200)
	onlyInstitution := searchPage("&institution=uhamka", "")
	if len(onlyInstitution.Items) != 1 || len(onlyInstitution.Items[0].Affiliations) != 1 || onlyInstitution.Items[0].Affiliations[0].StudyProgram != nil || onlyInstitution.Items[0].Affiliations[0].GraduationYear != nil {
		t.Fatal("unconsented academic projection", onlyInstitution)
	}
	if p := searchPage("&institution=uhamka&study_program=mathematics-education", ""); len(p.Items) != 0 {
		t.Fatal("hidden study program matched", p)
	}
	if p := searchPage("&graduation_year=2020", ""); len(p.Items) != 0 {
		t.Fatal("hidden graduation year matched", p)
	}
	// Literal wildcards are escaped and professional matching requires publication.
	exec(`UPDATE profiles SET current_company='100%_ Acme',current_job_title='Math teacher' WHERE user_id=$1`, a.id)
	for _, filter := range []string{"&company=100%25", "&job_title=Math"} {
		if p := searchPage(filter, ""); len(p.Items) != 0 {
			t.Fatal("hidden profession matched", p)
		}
	}
	call("PATCH", "/users/me/profile/privacy", `{"visibility":"public","show_current_company":true,"show_current_job_title":true}`, a.token, 200)
	if p := searchPage("&company=100%25_&job_title=Math", ""); len(p.Items) != 1 {
		t.Fatal(p)
	}
	if p := searchPage("&company=100x", ""); len(p.Items) != 0 {
		t.Fatal("wildcard injection", p)
	}
	if _, err = profile.AddSkill(ctx, a.id, "M4B Go "+run); err != nil {
		t.Fatal(err)
	}
	skillFilter := "&skills=" + url.QueryEscape("M4B Go "+run)
	if p := searchPage(skillFilter, ""); len(p.Items) != 0 {
		t.Fatal("hidden skill matched")
	}
	call("PATCH", "/users/me/profile/privacy", `{"visibility":"public","show_skills":true}`, a.token, 200)
	if p := searchPage(skillFilter, ""); len(p.Items) != 1 || p.Items[0].Skills == nil {
		t.Fatal(p)
	}
	// Paired hidden-value states yield the exact same public response and no matches.
	call("PATCH", "/users/me/profile/privacy", `{"visibility":"public","show_current_company":false,"show_current_job_title":false,"show_skills":false}`, a.token, 200)
	call("PATCH", ownerPath, `{"affiliations":[]}`, a.token, 200)
	pair1 := call("GET", "/directory?query="+url.QueryEscape(namePrefix), "", "", 200).Body.String()
	exec(`UPDATE profiles SET current_company='Different Secret',current_job_title='Different Private Title' WHERE user_id=$1`, a.id)
	exec(`UPDATE memberships SET graduation_year=2033 WHERE id=$1`, a.membership)
	// Detail edits revoke consent conservatively; explicitly opt back in, without academic disclosure.
	call("PATCH", ownerPath, `{"directory_discoverable":true}`, a.token, 200)
	pair2 := call("GET", "/directory?query="+url.QueryEscape(namePrefix), "", "", 200).Body.String()
	if pair1 != pair2 {
		t.Fatalf("hidden values changed observable projection:\n%s\n%s", pair1, pair2)
	}
	for _, filter := range []string{"&company=Different", "&job_title=Different", "&graduation_year=2033", "&institution=uhamka"} {
		if p := searchPage(filter, ""); len(p.Items) != 0 {
			t.Fatal("hidden field matched", filter, p)
		}
	}
	// Revocation with another verified membership retains discovery but clears that affiliation.
	call("PATCH", ownerPath, replace(consent(a.membership, true, true, true), consent(second, true, true, true)), a.token, 200)
	exec(`UPDATE memberships SET status='pending' WHERE id=$1`, a.membership)
	exec(`UPDATE memberships SET status='verified' WHERE id=$1`, a.membership)
	var current profiles.Discovery
	json.Unmarshal(call("GET", ownerPath, "", a.token, 200).Body.Bytes(), &current)
	if !current.DirectoryDiscoverable {
		t.Fatal("other verified affiliation should retain generic discovery")
	}
	for _, m := range current.Memberships {
		if m.MembershipID == a.membership && m.ShowInstitution {
			t.Fatal("academic consent resurrected")
		}
	}
	exec(`UPDATE memberships SET status='rejected' WHERE id=$1`, a.membership)
	exec(`DELETE FROM memberships WHERE id=$1`, second)
	exec(`UPDATE memberships SET status='verified' WHERE id=$1`, a.membership)
	json.Unmarshal(call("GET", ownerPath, "", a.token, 200).Body.Bytes(), &current)
	if current.DirectoryDiscoverable || current.DirectoryConsentedAt != nil {
		t.Fatal("last-membership consent resurrected")
	}
	// PUBLIC -> PRIVATE clears all consent in the existing privacy UPDATE transaction.
	call("PATCH", ownerPath, `{"directory_discoverable":true,"affiliations":[`+consent(a.membership, true, true, true)+`]}`, a.token, 200)
	call("PATCH", "/users/me/profile/privacy", `{"visibility":"private"}`, a.token, 200)
	call("PATCH", "/users/me/profile/privacy", `{"visibility":"public"}`, a.token, 200)
	json.Unmarshal(call("GET", ownerPath, "", a.token, 200).Body.Bytes(), &current)
	if current.DirectoryDiscoverable || current.DirectoryConsentedAt != nil || current.Memberships[0].ShowInstitution {
		t.Fatal("privacy withdrawal lost", current)
	}
	// Competing privacy/discovery requests serialize; neither order leaves a private opt-in.
	yes := true
	for i := 0; i < 12; i++ {
		if _, err = ps.UpdatePrivacy(ctx, a.id, profiles.PrivacyPatch{Visibility: "public"}); err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		errs := make(chan error, 2)
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, err := ps.UpdateDiscovery(ctx, a.id, profiles.DiscoveryPatch{DirectoryDiscoverable: &yes})
			errs <- err
		}()
		go func() {
			defer wg.Done()
			_, err := ps.UpdatePrivacy(ctx, a.id, profiles.PrivacyPatch{Visibility: "private"})
			errs <- err
		}()
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil && !errors.Is(err, profiles.ErrConflict) {
				t.Fatal(err)
			}
		}
		state, err := ps.Discovery(ctx, a.id)
		if err != nil || state.DirectoryDiscoverable || state.DirectoryConsentedAt != nil {
			t.Fatalf("race: %+v %v", state, err)
		}
	}
	// Verification revocation racing an opt-in always leaves withdrawn persisted consent.
	if _, err = ps.UpdatePrivacy(ctx, a.id, profiles.PrivacyPatch{Visibility: "public"}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 8; i++ {
		exec(`UPDATE memberships SET status='verified' WHERE id=$1`, a.membership)
		var wg sync.WaitGroup
		errs := make(chan error, 2)
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, err := ps.UpdateDiscovery(ctx, a.id, profiles.DiscoveryPatch{DirectoryDiscoverable: &yes})
			errs <- err
		}()
		go func() {
			defer wg.Done()
			_, err := db.Exec(ctx, `UPDATE memberships SET status='pending' WHERE id=$1`, a.membership)
			errs <- err
		}()
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil && !errors.Is(err, profiles.ErrConflict) {
				t.Fatal(err)
			}
		}
		state, err := ps.Discovery(ctx, a.id)
		if err != nil || state.DirectoryDiscoverable {
			t.Fatalf("revocation race: %+v %v", state, err)
		}
	}

	// Concurrent loss of different verified memberships cannot leave resurrectable discovery consent.
	exec(`UPDATE memberships SET status='verified' WHERE id=$1`, a.membership)
	var finalMembership uuid.UUID
	if err = db.QueryRow(ctx, `INSERT INTO memberships(user_id,study_program_id,role,status,graduation_year) VALUES($1,$2,'alumni','verified',2021) RETURNING id`, a.id, program2).Scan(&finalMembership); err != nil {
		t.Fatal(err)
	}
	call("PATCH", ownerPath, `{"directory_discoverable":true}`, a.token, 200)
	var revoked sync.WaitGroup
	revokeErrors := make(chan error, 2)
	for _, membership := range []uuid.UUID{a.membership, finalMembership} {
		revoked.Add(1)
		go func(id uuid.UUID) {
			defer revoked.Done()
			_, err := db.Exec(ctx, `UPDATE memberships SET status='pending' WHERE id=$1`, id)
			revokeErrors <- err
		}(membership)
	}
	revoked.Wait()
	close(revokeErrors)
	for err := range revokeErrors {
		if err != nil {
			t.Fatal(err)
		}
	}
	state, err := ps.Discovery(ctx, a.id)
	if err != nil || state.DirectoryDiscoverable {
		t.Fatalf("concurrent last membership loss: %+v %v", state, err)
	}
	exec(`DELETE FROM memberships WHERE id=$1`, finalMembership)
	// Canonical results only; old slugs redirect only while current visibility allows it.
	exec(`UPDATE memberships SET status='verified' WHERE id=$1`, a.membership)
	call("PATCH", ownerPath, `{"directory_discoverable":true}`, a.token, 200)
	old := a.slug
	newSlug := "m4b-" + run + "-renamed"
	if _, err = profile.UpdateProfile(ctx, a.id, profiles.ProfilePatch{Slug: &newSlug}); err != nil {
		t.Fatal(err)
	}
	call("GET", "/profiles/"+old, "", "", 307)
	for _, item := range searchPage("", "").Items {
		if item.Slug == old {
			t.Fatal("historical duplicate")
		}
	}
	exec(`DELETE FROM users WHERE id=$1`, a.id)
	call("GET", "/profiles/"+old, "", "", 404)
	if _, err = db.Exec(ctx, `UPDATE profile_slug_history SET user_id=$2 WHERE slug=$1`, old, c.id); err == nil {
		t.Fatal("tombstone reassigned")
	}
	// Cancellation reaches pgx; no result or private diagnostic is serialized.
	canceled, stop := context.WithCancel(ctx)
	stop()
	if _, err := directorypg.New(db).Search(canceled, directory.Spec{Limit: 20}); err == nil {
		t.Fatal("cancellation ignored")
	}
}

// A stronger-isolation lifecycle writer must abort rather than retain stale consent.
func TestMembershipRevocationWithOldSnapshot(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL required")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	owner, extraProgram, first, second := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := db.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	var program, institution uuid.UUID
	if err = db.QueryRow(ctx, `SELECT id,institution_id FROM study_programs LIMIT 1`).Scan(&program, &institution); err != nil {
		t.Fatal(err)
	}
	exec(`INSERT INTO study_programs(id,institution_id,slug,name) VALUES($1,$2,$3,'Snapshot fixture')`, extraProgram, institution, "snapshot-"+owner.String())
	defer func() {
		db.Exec(ctx, `DELETE FROM users WHERE id=$1`, owner)
		db.Exec(ctx, `DELETE FROM study_programs WHERE id=$1`, extraProgram)
	}()
	exec(`INSERT INTO users(id,oidc_subject) VALUES($1,$2)`, owner, "snapshot-"+owner.String())
	exec(`INSERT INTO memberships(id,user_id,study_program_id,role,status,graduation_year) VALUES($1,$2,$3,'alumni','verified',2020),($4,$2,$5,'alumni','verified',2021)`, first, owner, program, second, extraProgram)
	slug := "snapshot-" + owner.String()
	exec(`INSERT INTO profile_slug_history(slug,user_id) VALUES($1,$2)`, slug, owner)
	exec(`INSERT INTO profiles(user_id,slug,visibility,directory_discoverable,directory_consented_at) VALUES($1,$2,'public',true,now())`, owner, slug)
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
	// Establish the same pre-revocation snapshot in both transactions.
	for _, tx := range []pgx.Tx{a, b} {
		var n int
		if err = tx.QueryRow(ctx, `SELECT count(*) FROM memberships WHERE user_id=$1`, owner).Scan(&n); err != nil || n != 2 {
			t.Fatalf("snapshot %d %v", n, err)
		}
	}
	if _, err = a.Exec(ctx, `UPDATE memberships SET status='pending' WHERE id=$1`, first); err != nil {
		t.Fatal(err)
	}
	if err = a.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	_, err = b.Exec(ctx, `UPDATE memberships SET status='pending' WHERE id=$1`, second)
	var pgError *pgconn.PgError
	if !errors.As(err, &pgError) || pgError.Code != "40001" {
		t.Fatalf("old snapshot should fail closed with serialization error, got %v", err)
	}
	if err = b.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	// A fresh whole-transaction retry sees the first revocation and clears consent.
	exec(`UPDATE memberships SET status='pending' WHERE id=$1`, second)
	exec(`UPDATE memberships SET status='verified' WHERE id=$1`, first)
	state, err := profilespg.New(db).Discovery(ctx, owner)
	if err != nil || state.DirectoryDiscoverable || state.DirectoryConsentedAt != nil {
		t.Fatalf("consent resurrected: %+v %v", state, err)
	}
}
