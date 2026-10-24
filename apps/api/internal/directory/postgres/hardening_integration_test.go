package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sipratama/alumnibridge/apps/api/internal/directory"
)

func TestSearchDatabaseTimeoutAndCancellationCleanup(t *testing.T) {
	db, ctx := reviewDatabase(t)
	cfg, err := pgxpool.ParseConfig(os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxConns = 1
	app := "directory-timeout-" + uuid.NewString()
	cfg.ConnConfig.RuntimeParams["application_name"] = app
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	lock, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Rollback(ctx)
	if _, err = lock.Exec(ctx, `LOCK TABLE profiles IN ACCESS EXCLUSIVE MODE`); err != nil {
		t.Fatal(err)
	}
	spec := directory.Spec{Limit: 20}
	start := time.Now()
	_, err = NewWithTimeout(pool, 50*time.Millisecond).Search(ctx, spec)
	if !errors.Is(err, directory.ErrDatabaseTimeout) || time.Since(start) > time.Second {
		t.Fatal("real statement timeout not mapped", err, time.Since(start))
	}
	assertClean := func() {
		t.Helper()
		conn, err := pool.Acquire(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Release()
		var timeout string
		if err = conn.QueryRow(ctx, `SHOW statement_timeout`).Scan(&timeout); err != nil || timeout != "0" {
			t.Fatal("pooled timeout leaked", timeout, err)
		}
		var active int
		if err = db.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity WHERE application_name=$1 AND state='active'`, app).Scan(&active); err != nil || active != 0 {
			t.Fatal("abandoned active search", active, err)
		}
	}
	assertClean()
	canceled, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := NewWithTimeout(pool, time.Second).Search(canceled, spec); done <- err }()
	deadline := time.Now().Add(time.Second)
	for {
		var waiting bool
		if err = db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE application_name=$1 AND wait_event_type='Lock' AND state='active')`, app).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("search never reached PostgreSQL")
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	select {
	case err = <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal("lost cancellation", err)
		}
	case <-time.After(time.Second):
		t.Fatal("database search did not stop")
	}
	assertClean()
	if err = lock.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = New(pool).Search(ctx, spec); err != nil {
		t.Fatal("pool unusable after timeout/cancel", err)
	}
	assertClean()
}

func TestLiveCursorPrivacyAndLexicalBoundaries(t *testing.T) {
	db, ctx := reviewDatabase(t)
	for _, scenario := range []string{"static", "exact_page", "max_page", "empty", "deleted_anchor", "private_anchor", "hidden_anchor", "renamed_anchor", "insertions", "withdrawal", "revocation"} {
		t.Run(scenario, func(t *testing.T) {
			prefix := "page-" + uuid.NewString()
			var owners []uuid.UUID
			exec := func(sql string, args ...any) {
				t.Helper()
				if _, err := db.Exec(ctx, sql, args...); err != nil {
					t.Fatal(err)
				}
			}
			add := func(n int) uuid.UUID {
				owner := reviewOwner(t, db, ctx)
				slug := fmt.Sprintf("%s-%03d", prefix, n)
				exec(`UPDATE users SET display_name=$2 WHERE id=$1`, owner, prefix)
				exec(`INSERT INTO profile_slug_history(slug,user_id) VALUES($1,$2)`, slug, owner)
				exec(`UPDATE profiles SET slug=$2 WHERE user_id=$1`, owner, slug)
				return owner
			}
			count := 5
			if scenario == "empty" {
				count = 0
			}
			if scenario == "exact_page" {
				count = 2
			}
			if scenario == "max_page" {
				count = 51
			}
			for i := 1; i <= count; i++ {
				owners = append(owners, add(i*10))
			}
			codec, _ := directory.NewCursor(make([]byte, 32))
			svc := directory.New(New(db), codec)
			spec := directory.Spec{Query: prefix, Limit: 2}
			if scenario == "max_page" {
				spec.Limit = 50
			}
			first, err := svc.Search(ctx, spec, "")
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "max_page" {
				if len(first.Items) != 50 || first.NextCursor == nil {
					t.Fatal("maximum page not bounded", first)
				}
				last, err := svc.Search(ctx, spec, *first.NextCursor)
				if err != nil || len(last.Items) != 1 || last.Items[0].Slug != prefix+"-510" || last.NextCursor != nil {
					t.Fatal("maximum page lookahead", last, err)
				}
				return
			}
			if count <= 2 {
				if len(first.Items) != count || first.NextCursor != nil {
					t.Fatal("page boundary", first)
				}
				return
			}
			if first.NextCursor == nil || len(first.Items) != 2 {
				t.Fatal("missing lookahead cursor", first)
			}
			expected := []string{prefix + "-030", prefix + "-040", prefix + "-050"}
			switch scenario {
			case "deleted_anchor":
				exec(`DELETE FROM users WHERE id=$1`, owners[1])
			case "private_anchor":
				exec(`UPDATE profiles SET visibility='private' WHERE user_id=$1`, owners[1])
			case "hidden_anchor":
				exec(`UPDATE profiles SET moderation_status='hidden' WHERE user_id=$1`, owners[1])
			case "renamed_anchor":
				exec(`INSERT INTO profile_slug_history(slug,user_id) VALUES($1,$2)`, prefix+"-060", owners[1])
				exec(`UPDATE profiles SET slug=$2 WHERE user_id=$1`, owners[1], prefix+"-060")
				expected = append(expected, prefix+"-060") // Live renames may repeat a person under a new public slug.
			case "insertions":
				add(15)
				add(35)
				expected = []string{prefix + "-030", prefix + "-035", prefix + "-040", prefix + "-050"}
			case "withdrawal":
				exec(`UPDATE profiles SET directory_discoverable=false WHERE user_id=$1`, owners[2])
				expected = expected[1:]
			case "revocation":
				exec(`UPDATE memberships SET status='pending' WHERE user_id=$1`, owners[2])
				expected = expected[1:]
			}
			cursor := first.NextCursor
			var got []string
			for pages := 0; cursor != nil; pages++ {
				if pages > 5 {
					t.Fatal("pagination loop")
				}
				p, err := svc.Search(ctx, spec, *cursor)
				if err != nil {
					t.Fatal(err)
				}
				for _, e := range p.Items {
					got = append(got, e.Slug)
				}
				cursor = p.NextCursor
			}
			if !reflect.DeepEqual(got, expected) {
				t.Fatalf("live pagination %v != %v", got, expected)
			}
		})
	}
}
