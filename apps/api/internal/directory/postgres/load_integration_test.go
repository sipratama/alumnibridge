package postgres

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sipratama/alumnibridge/apps/api/internal/directory"
	directoryhttp "github.com/sipratama/alumnibridge/apps/api/internal/directory/http"
	"github.com/sipratama/alumnibridge/apps/api/internal/httpserver"
)

// Unlike EXPLAIN, concurrent connections need committed fixtures. This test is
// explicitly opt-in and requires its own empty disposable database.
func TestDirectoryLoad(t *testing.T) {
	if os.Getenv("DIRECTORY_LOAD") != "1" {
		t.Skip("opt-in isolated burst assessment")
	}
	db, ctx := reviewDatabase(t)
	var count int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&count); err != nil || count != 0 {
		t.Fatal("requires empty disposable database", err)
	}
	if err := db.QueryRow(ctx, `SELECT count(*) FROM profile_slug_history WHERE starts_with(slug,'perf-')`).Scan(&count); err != nil || count != 0 {
		t.Fatal("requires a fresh disposable database: previous load fixtures retain permanent slug tombstones", err)
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	seedPerformance(t, ctx, tx, 25000)
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := db.Exec(context.Background(), `DELETE FROM users WHERE oidc_subject LIKE 'perf-%'`); err != nil {
			t.Error(err)
		}
		if _, err := db.Exec(context.Background(), `DELETE FROM skills WHERE id=md5('perf-skill')::uuid`); err != nil {
			t.Error(err)
		}
	})
	limits := directoryhttp.DefaultLimits()
	cfg := db.Config()
	cfg.MaxConns = int32(limits.MaxConcurrent)
	cfg.MinConns = 0
	searchDB, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer searchDB.Close()
	codec, _ := directory.NewCursor(make([]byte, 32))
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := httpserver.New(db, logger)
	directoryhttp.RegisterRoutesWithLimits(router, directory.New(New(searchDB), codec), limits, logger)
	gate := make(chan struct{})
	var wg sync.WaitGroup
	var mu sync.Mutex
	statuses := make(map[int]int)
	peak := time.Duration(0)
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-gate
			req := httptest.NewRequest("GET", "/directory?query=siti", nil)
			// Multiple canonical peers demonstrate the global cap even when source
			// addresses vary; the normal unit suite separately exercises one-IP 429.
			req.RemoteAddr = fmt.Sprintf("192.0.2.%d:2000", i+1)
			w := httptest.NewRecorder()
			start := time.Now()
			router.ServeHTTP(w, req)
			elapsed := time.Since(start)
			mu.Lock()
			statuses[w.Code]++
			peak = max(peak, elapsed)
			mu.Unlock()
			if w.Code != 200 && w.Code != 503 {
				t.Errorf("unexpected burst response %d %s", w.Code, w.Body.String())
			}
		}(i)
	}
	close(gate)
	deadline := time.Now().Add(time.Second)
	for searchDB.Stat().AcquiredConns() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("burst never reached search pool")
		}
		time.Sleep(time.Millisecond)
	}
	start := time.Now()
	ready := httptest.NewRecorder()
	router.ServeHTTP(ready, httptest.NewRequest("GET", "/ready", nil))
	readyElapsed := time.Since(start)
	wg.Wait()
	if ready.Code != 200 || statuses[503] == 0 || searchDB.Stat().AcquiredConns() != 0 {
		t.Fatal("burst did not remain bounded", ready.Code, statuses, searchDB.Stat())
	}
	if peak > 4*time.Second {
		t.Fatal("search exceeded request plus cleanup budget", peak)
	}
	t.Logf("25k profiles / 100 distinct-peer simultaneous searches: statuses=%v max_duration=%s ready=%d ready_duration=%s pool_max=%d", statuses, peak, ready.Code, readyElapsed, searchDB.Stat().MaxConns())
}
