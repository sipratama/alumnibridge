package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sipratama/alumnibridge/apps/api/internal/directory"
	directoryhttp "github.com/sipratama/alumnibridge/apps/api/internal/directory/http"
	"github.com/sipratama/alumnibridge/apps/api/internal/identity"
	identitypg "github.com/sipratama/alumnibridge/apps/api/internal/identity/postgres"
	"github.com/sipratama/alumnibridge/apps/api/internal/profiles"
	profilepg "github.com/sipratama/alumnibridge/apps/api/internal/profiles/postgres"
)

func reviewDatabase(t *testing.T) (*pgxpool.Pool, context.Context) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL required; disposable database only")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	return db, ctx
}
func reviewOwner(t *testing.T, db *pgxpool.Pool, ctx context.Context) uuid.UUID {
	t.Helper()
	owner, member := uuid.New(), uuid.New()
	slug := "audit-" + owner.String()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := db.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO users(id,oidc_subject,display_name) VALUES($1,$2,'Audit fixture')`, owner, slug)
	t.Cleanup(func() {
		if _, err := db.Exec(ctx, `DELETE FROM users WHERE id=$1`, owner); err != nil {
			t.Error(err)
		}
	})
	exec(`INSERT INTO memberships(id,user_id,study_program_id,role,status,graduation_year) SELECT $1,$2,id,'alumni','verified',2020 FROM study_programs LIMIT 1`, member, owner)
	exec(`INSERT INTO profile_slug_history(slug,user_id) VALUES($1,$2)`, slug, owner)
	exec(`INSERT INTO profiles(user_id,slug,visibility,directory_discoverable,directory_consented_at,show_current_company,show_current_job_title) VALUES($1,$2,'public',true,now(),true,true)`, owner, slug)
	return owner
}

func TestReviewLiteralPrefixNormalization(t *testing.T) {
	db, ctx := reviewDatabase(t)
	owner := reviewOwner(t, db, ctx)
	codec, err := directory.NewCursor(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	limits := directoryhttp.DefaultLimits()
	limits.Burst = 1000
	limits.ClientTTL = 20 * time.Minute
	directoryhttp.RegisterRoutesWithLimits(router, directory.New(New(db), codec), limits, slog.Default())
	for _, tc := range []struct{ name, value, query string }{
		{"Indonesian spaces", "  Siti   Nur  ", "SITI NUR"},
		{"ASCII whitespace fast path", " A\tB\nC\vD\fE\rF ", "a b c d e f"},
		{"non-ASCII control whitespace", "A\u0085B", "a b"},
		{"NBSP from onboarding", "Siti\u00a0\u00a0Nur", "siti nur"},
		{"narrow Unicode spaces", "\u202fSiti\u2003Nur\u3000", "Siti Nur"},
		{"accented Latin", "ÉLODIE André", "élodie"},
		{"Turkish simple lowercase", "İpek", "İpek"},
		{"capital sharp S", "STRAẞE", "straße"},
		{"Unicode table difference", "ⒶB Example", "ⒶB"},
		{"new Unicode letter", "\u1c89a Example", "\u1c89a"},
		{"Greek", "ΣΟΣ", "ΣΟΣ"},
		{"literal percent underscore", "100%_ Acme", "100%_"},
		{"literal backslash", `Path\Name`, `Path\`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := db.Exec(ctx, `UPDATE users SET display_name=$2 WHERE id=$1`, owner, tc.value); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(ctx, `UPDATE profiles SET current_company=$2,current_job_title=$2 WHERE user_id=$1`, owner, tc.value); err != nil {
				t.Fatal(err)
			}
			for _, field := range []string{"query", "company", "job_title"} {
				req := httptest.NewRequest("GET", "/directory?"+field+"="+url.QueryEscape(tc.query), nil)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)
				var p directory.Page
				if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil {
					t.Fatal(err)
				}
				found := false
				for _, item := range p.Items {
					if item.Slug == "audit-"+owner.String() {
						found = true
					}
				}
				if w.Code != 200 || !found {
					t.Errorf("%s %q should match %q: HTTP %d %s", field, tc.query, tc.value, w.Code, w.Body.String())
				}
			}
		})
	}
	// No accent stripping, expanding fold (sharp-S/ss), or wildcard language.
	for _, tc := range []struct{ value, query string }{{"Élodie", "Elodie"}, {"Straße", "STRASSE"}, {"100%_ Acme", "100xx"}, {`Path\Name`, `Pathx`}} {
		if _, err := db.Exec(ctx, `UPDATE users SET display_name=$2 WHERE id=$1`, owner, tc.value); err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest("GET", "/directory?query="+url.QueryEscape(tc.query), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != 200 || strings.Contains(w.Body.String(), "audit-"+owner.String()) {
			t.Errorf("nonliteral match %q -> %q: %s", tc.query, tc.value, w.Body.String())
		}
	}
}

func TestReviewProfileEditAndAccountDeletionLockOrder(t *testing.T) {
	db, base := reviewDatabase(t)
	owner := reviewOwner(t, db, base)
	ctx, cancel := context.WithTimeout(base, 10*time.Second)
	defer cancel()
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(base)
	var parentPID int
	if err = tx.QueryRow(ctx, `SELECT pg_backend_pid() FROM users WHERE id=$1 FOR UPDATE`, owner).Scan(&parentPID); err != nil {
		t.Fatal(err)
	}
	edited := make(chan error, 1)
	go func() {
		_, err := profilepg.New(db).UpdateProfile(ctx, owner, func(p profiles.Profile) (profiles.Profile, error) { p.Headline = "Concurrent edit"; return p, nil })
		edited <- err
	}()
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	for {
		var blocked bool
		if err = db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE $1=ANY(pg_blocking_pids(pid)))`, parentPID).Scan(&blocked); err != nil {
			t.Fatal(err)
		}
		if blocked {
			break
		}
		select {
		case err := <-edited:
			t.Fatalf("edit unexpectedly completed: %v", err)
		case <-deadline.C:
			t.Fatal("edit did not reach parent lock")
		case <-tick.C:
		}
	}
	_, deleteErr := tx.Exec(ctx, `DELETE FROM users WHERE id=$1`, owner)
	if deleteErr == nil {
		deleteErr = tx.Commit(ctx)
	} else {
		tx.Rollback(base)
	}
	editErr := <-edited
	if deleteErr != nil || !errors.Is(editErr, profiles.ErrNotFound) {
		t.Fatalf("delete must commit and waiting edit must return not-found, delete=%v edit=%v", deleteErr, editErr)
	}
}

func TestReviewOnboardProfileDiscoveryCompatibility(t *testing.T) {
	db, base := reviewDatabase(t)
	owner := reviewOwner(t, db, base)
	ctx, cancel := context.WithTimeout(base, 10*time.Second)
	defer cancel()
	year := 2020
	yes := true
	input := identity.OnboardingInput{Role: "alumni", DisplayName: "Audit fixture", Institution: "uhamka", StudyProgram: "mathematics-education", GraduationYear: &year}
	for i := 0; i < 12; i++ {
		var wg sync.WaitGroup
		errs := make(chan error, 3)
		wg.Add(3)
		go func() { defer wg.Done(); _, err := identitypg.New(db).Onboard(ctx, owner, input); errs <- err }()
		go func() {
			defer wg.Done()
			_, err := profilepg.New(db).UpdateProfile(ctx, owner, func(p profiles.Profile) (profiles.Profile, error) { p.Headline = "Compatible update"; return p, nil })
			errs <- err
		}()
		go func() {
			defer wg.Done()
			_, err := profilepg.New(db).UpdateDiscovery(ctx, owner, profiles.DiscoveryPatch{DirectoryDiscoverable: &yes})
			errs <- err
		}()
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	state, err := profilepg.New(db).Discovery(ctx, owner)
	if err != nil || !state.EffectiveDiscoverable {
		t.Fatalf("unexpected state: %+v %v", state, err)
	}
}

func TestReviewPairedHiddenProfiles(t *testing.T) {
	db, ctx := reviewDatabase(t)
	a := reviewOwner(t, db, ctx)
	b := reviewOwner(t, db, ctx)
	name := "Paired " + uuid.NewString()[:8]
	for i, id := range []uuid.UUID{a, b} {
		if _, err := db.Exec(ctx, `UPDATE users SET display_name=$2 WHERE id=$1`, id, name); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(ctx, `UPDATE profiles SET show_current_company=false,show_current_job_title=false,current_company=$2,current_job_title=$3 WHERE user_id=$1`, id, []string{"Hidden Alpha", "Hidden Beta"}[i], []string{"Private A", "Private B"}[i]); err != nil {
			t.Fatal(err)
		}
	}
	codec, _ := directory.NewCursor(make([]byte, 32))
	router := chi.NewRouter()
	directoryhttp.RegisterRoutes(router, directory.New(New(db), codec))
	read := func(extra string) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest("GET", "/directory?query="+url.QueryEscape(name)+extra, nil))
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		return w
	}
	before := read("").Body.String()
	var page directory.Page
	if err := json.Unmarshal([]byte(before), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 {
		t.Fatal(page)
	}
	left, right := page.Items[0], page.Items[1]
	left.Slug = ""
	right.Slug = ""
	if !reflect.DeepEqual(left, right) {
		t.Fatal("paired public fields differ", left, right)
	}
	if _, err := db.Exec(ctx, `UPDATE profiles SET current_company='Hidden Changed',current_job_title='Private Changed' WHERE user_id=$1`, b); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `UPDATE memberships SET graduation_year=2049 WHERE user_id=$1`, b); err != nil {
		t.Fatal(err)
	}
	if after := read("").Body.String(); after != before {
		t.Fatalf("private values changed public responses: %s / %s", before, after)
	}
	for _, extra := range []string{"&company=Hidden", "&job_title=Private", "&institution=uhamka", "&graduation_year=2049"} {
		var p directory.Page
		if err := json.Unmarshal(read(extra).Body.Bytes(), &p); err != nil || len(p.Items) != 0 {
			t.Fatal("private filter matched", extra, p, err)
		}
	}
	// Both slash variants remain anonymous and do not redirect to the identity provider.
	for _, path := range []string{"/directory", "/directory/"} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest("GET", path+"?query="+url.QueryEscape(name), nil))
		if w.Code != 200 || w.Header().Get("Location") != "" {
			t.Fatal(path, w.Code, w.Header())
		}
	}
}

func TestReviewOwnerMembershipCatalog(t *testing.T) {
	db, ctx := reviewDatabase(t)
	owner := reviewOwner(t, db, ctx)
	var institution uuid.UUID
	if err := db.QueryRow(ctx, `SELECT institution_id FROM study_programs LIMIT 1`).Scan(&institution); err != nil {
		t.Fatal(err)
	}
	prefix := "catalog-" + owner.String() + "-"
	if _, err := db.Exec(ctx, `INSERT INTO study_programs(institution_id,slug,name) SELECT $1,$2||lpad(n::text,3,'0'),'Catalog fixture' FROM generate_series(1,300)n`, institution, prefix); err != nil {
		t.Fatal(err)
	}
	// Register this cleanup before returning: delete owner memberships before reference rows.
	t.Cleanup(func() {
		db.Exec(ctx, `DELETE FROM users WHERE id=$1`, owner)
		if _, err := db.Exec(ctx, `DELETE FROM study_programs WHERE institution_id=$1 AND starts_with(slug,$2)`, institution, prefix); err != nil {
			t.Error(err)
		}
	})
	if _, err := db.Exec(ctx, `INSERT INTO memberships(user_id,study_program_id,role,status,graduation_year) SELECT $1,id,'alumni','verified',2020 FROM study_programs WHERE institution_id=$2 AND starts_with(slug,$3)`, owner, institution, prefix); err != nil {
		t.Fatal(err)
	}
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	started := time.Now()
	state, err := profilepg.New(db).Discovery(ctx, owner)
	elapsed := time.Since(started)
	runtime.ReadMemStats(&after)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("301-choice catalog: elapsed=%s allocations=%d bytes (includes driver and JSON decode)", elapsed, after.TotalAlloc-before.TotalAlloc)
	if len(state.Memberships) != 301 {
		t.Fatalf("owner choices missing: %d", len(state.Memberships))
	}
	for i, m := range state.Memberships {
		if m.ShowInstitution || m.ShowStudyProgram || m.ShowGraduationYear {
			t.Fatal("unconsented choice published")
		}
		if i > 0 {
			prev := state.Memberships[i-1]
			if prev.Institution > m.Institution || prev.Institution == m.Institution && prev.StudyProgram > m.StudyProgram {
				t.Fatal("nondeterministic choices")
			}
		}
	}
	body, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("301 verified private membership choices: %d JSON bytes", len(body))
	started = time.Now()
	u, err := identity.NewService(identitypg.New(db)).Me(ctx, identity.Principal{Subject: "audit-" + owner.String()})
	if err != nil || len(u.Memberships) != 301 {
		t.Fatal("identity membership loading mismatch", len(u.Memberships), err)
	}
	t.Logf("identity.Me also loads all 301 memberships: %s", time.Since(started))
}

func TestReviewInFlightPoolWaitCancellation(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL required")
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxConns = 1
	db, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	held, err := db.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer held.Release()
	c, _ := directory.NewCursor(make([]byte, 32))
	router := chi.NewRouter()
	directoryhttp.RegisterRoutes(router, directory.New(New(db), c))
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/directory", nil).WithContext(ctx))
	if w.Code != 503 || w.Body.String() != "{\"error\":\"directory_timeout\"}\n" || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("cancellation response: %d %v %s", w.Code, w.Header(), w.Body.String())
	}
}
