package directoryhttp

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sipratama/alumnibridge/apps/api/internal/directory"
)

func TestFinalReviewAdmissionReleaseAfterEveryFailure(t *testing.T) {
	limits := DefaultLimits()
	limits.MaxConcurrent = 1
	limits.Burst = 100
	var failure error
	var wait bool
	entered := make(chan struct{}, 1)
	repo := searchFunc(func(ctx context.Context, _ directory.Spec) ([]directory.Entry, error) {
		if wait {
			entered <- struct{}{}
			<-ctx.Done()
			return nil, ctx.Err()
		}
		return nil, failure
	})
	router := controlledRouter(repo, limits, slog.New(slog.NewTextHandler(io.Discard, nil)))
	for _, tc := range []struct {
		err    error
		status int
	}{
		{errors.New("private SQL detail"), 500},
		{directory.ErrDatabaseTimeout, 503},
		{directory.ErrInvalid, 400},
		{context.DeadlineExceeded, 503},
		{context.Canceled, 408},
	} {
		failure = tc.err
		if w := request(router, "192.0.2.1:2000"); w.Code != tc.status {
			t.Fatal("unexpected failure response", w.Code)
		}
		failure = nil
		if w := request(router, "192.0.2.1:2000"); w.Code != 200 {
			t.Fatal("permit leaked after failure", tc.err, w.Code)
		}
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/directory?cursor=invalid", nil))
	if w.Code != 400 {
		t.Fatal("invalid cursor was not rejected", w.Code)
	}
	if w = request(router, "192.0.2.1:2000"); w.Code != 200 {
		t.Fatal("permit leaked after cursor rejection", w.Code)
	}

	// Cancel after admission and after the repository has actually started.
	wait = true
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan int, 1)
	go func() {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest("GET", "/directory", nil).WithContext(ctx))
		done <- w.Code
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("request not admitted")
	}
	cancel()
	select {
	case status := <-done:
		if status != 408 {
			t.Fatal("incorrect canceled response", status)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled handler did not return")
	}
	wait = false
	if w = request(router, "192.0.2.1:2000"); w.Code != 200 {
		t.Fatal("permit leaked after active cancellation", w.Code)
	}
}
