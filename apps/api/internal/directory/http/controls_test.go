package directoryhttp

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/sipratama/alumnibridge/apps/api/internal/directory"
)

func TestLimiterRefillExpirationAndBound(t *testing.T) {
	l := DefaultLimits()
	l.Burst = 2
	l.MaxClients = 2
	r := newLimiter(l)
	now := time.Unix(1000, 0)
	r.now = func() time.Time { return now }
	a, b, c := netip.MustParseAddr("192.0.2.1"), netip.MustParseAddr("192.0.2.2"), netip.MustParseAddr("192.0.2.3")
	for i := 0; i < 2; i++ {
		if ok, _, _ := r.allow(a); !ok {
			t.Fatal("burst rejected")
		}
	}
	if ok, full, retry := r.allow(a); ok || full || retry != 1 {
		t.Fatal("expected one-second refill", ok, full, retry)
	}
	now = now.Add(100 * time.Millisecond)
	if ok, _, retry := r.allow(a); ok || retry != 1 {
		t.Fatal("fractional refill rounded incorrectly")
	}
	now = now.Add(900 * time.Millisecond)
	if ok, _, _ := r.allow(a); !ok {
		t.Fatal("refill failed")
	}
	r.allow(b)
	if ok, full, _ := r.allow(c); ok || !full || len(r.clients) != 2 {
		t.Fatal("client bound bypassed")
	}
	now = now.Add(l.ClientTTL + time.Minute)
	if ok, _, _ := r.allow(c); !ok || len(r.clients) != 1 {
		t.Fatal("expired client entries retained")
	}
}

func TestLimiterConcurrentBurst(t *testing.T) {
	r := newLimiter(DefaultLimits())
	r.now = func() time.Time { return time.Unix(1000, 0) }
	var allowed atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if ok, _, _ := r.allow(netip.MustParseAddr("::1")); ok {
				allowed.Add(1)
			}
		}()
	}
	wg.Wait()
	if allowed.Load() != 10 {
		t.Fatal("burst not atomic", allowed.Load())
	}
}

func TestSlowRefillRetryAfter(t *testing.T) {
	l := DefaultLimits()
	l.RatePerMinute = 1
	l.Burst = 1
	r := newLimiter(l)
	now := time.Unix(1000, 0)
	r.now = func() time.Time { return now }
	ip := netip.MustParseAddr("::1")
	r.allow(ip)
	if ok, _, retry := r.allow(ip); ok || retry != 60 {
		t.Fatal("incorrect slow refill", retry)
	}
	now = now.Add(59 * time.Second)
	if ok, _, retry := r.allow(ip); ok || retry != 1 {
		t.Fatal("incorrect remaining wait", retry)
	}
	now = now.Add(time.Second)
	if ok, _, _ := r.allow(ip); !ok {
		t.Fatal("token not restored after Retry-After")
	}
}

type searchFunc func(context.Context, directory.Spec) ([]directory.Entry, error)

func (f searchFunc) Search(ctx context.Context, s directory.Spec) ([]directory.Entry, error) {
	return f(ctx, s)
}

func controlledRouter(repo directory.Repository, l Limits, logger *slog.Logger) chi.Router {
	c, _ := directory.NewCursor(make([]byte, 32))
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	RegisterRoutesWithLimits(r, directory.New(repo, c), l, logger)
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
	return r
}
func request(r http.Handler, remote string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", "/directory", nil)
	req.RemoteAddr = remote
	// A rotating forwarded identity and an authenticated-looking header grant no bypass.
	req.Header.Set("X-Forwarded-For", time.Now().String())
	req.Header.Set("Forwarded", "for=198.51.100.2")
	req.Header.Set("Authorization", "Bearer ignored-by-public-search")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestSourceIdentityAndProxySpoofing(t *testing.T) {
	l := DefaultLimits()
	l.Burst = 1
	l.MaxClients = 2
	r := controlledRouter(searchFunc(func(context.Context, directory.Spec) ([]directory.Entry, error) { return nil, nil }), l, slog.New(slog.NewTextHandler(io.Discard, nil)))
	for _, tc := range []struct {
		remote string
		status int
	}{
		{"192.0.2.1:1000", 200}, {"[::ffff:192.0.2.1]:2000", 429}, {"192.0.2.1:3000", 429},
		{"[2001:db8::1]:1000", 200}, {"[2001:0db8:0:0::1]:1001", 429}, {"[2001:db8::2]:1000", 503}, {"spoofed", 400},
	} {
		w := request(r, tc.remote)
		if w.Code != tc.status {
			t.Fatalf("%s: %d %s", tc.remote, w.Code, w.Body.String())
		}
		if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Fatal("unsafe headers")
		}
		if tc.status == 429 && w.Header().Get("Retry-After") != "1" {
			t.Fatal("missing refill hint")
		}
	}
}

func TestAdmissionCancellationAndOtherRoutes(t *testing.T) {
	l := DefaultLimits()
	l.MaxConcurrent = 2
	l.RequestTimeout = 100 * time.Millisecond
	entered := make(chan struct{}, 4)
	repo := searchFunc(func(ctx context.Context, _ directory.Spec) ([]directory.Entry, error) {
		entered <- struct{}{}
		<-ctx.Done()
		return nil, ctx.Err()
	})
	var logs bytes.Buffer
	r := controlledRouter(repo, l, slog.New(slog.NewJSONHandler(&logs, nil)))
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := request(r, "192.0.2.1:1000")
			if w.Code != 503 || !strings.Contains(w.Body.String(), "directory_timeout") {
				t.Error("missing request timeout", w.Code, w.Body.String())
			}
		}()
	}
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("request not admitted")
		}
	}
	start := time.Now()
	w := request(r, "192.0.2.1:1000")
	if w.Code != 503 || !strings.Contains(w.Body.String(), "directory_overloaded") || time.Since(start) > 50*time.Millisecond {
		t.Fatal("admission queued request", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/health", nil))
	if w.Code != 200 {
		t.Fatal("unrelated route blocked")
	}
	wg.Wait()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/directory", nil).WithContext(ctx))
	if w.Code != 408 {
		t.Fatal("client cancellation not distinct", w.Code)
	}
	// Both prior slots were released: this request reaches the repository.
	w = request(r, "192.0.2.1:1000")
	if !strings.Contains(w.Body.String(), "directory_timeout") {
		t.Fatal("admission leaked", w.Body.String())
	}
	for _, signal := range []string{"directory_timeout", "directory_overloaded", "directory_canceled", "request_id", "in_flight"} {
		if !strings.Contains(logs.String(), signal) {
			t.Fatal("missing operational signal", signal)
		}
	}
}

func TestSearchLogsExcludeInputs(t *testing.T) {
	var logs bytes.Buffer
	r := controlledRouter(searchFunc(func(context.Context, directory.Spec) ([]directory.Entry, error) {
		return nil, directory.ErrDatabaseTimeout
	}), DefaultLimits(), slog.New(slog.NewJSONHandler(&logs, nil)))
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/directory?query=PrivateName&cursor=secret-cursor", nil)
	req.Header.Set("X-Request-ID", "test-correlation")
	r.ServeHTTP(w, req)
	if strings.Contains(logs.String(), "PrivateName") || strings.Contains(logs.String(), "secret-cursor") || !strings.Contains(logs.String(), "test-correlation") {
		t.Fatal("unsafe or uncorrelated logging", logs.String())
	}
	w = request(r, "192.0.2.1:1234")
	if w.Code != 503 || !strings.Contains(w.Body.String(), "directory_database_timeout") {
		t.Fatal("database timeout not distinct", w.Body.String())
	}
}
