package directoryhttp

import (
	"context"
	"errors"
	"github.com/go-chi/chi/v5"
	"github.com/sipratama/alumnibridge/apps/api/internal/directory"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStrictParameters(t *testing.T) {
	for _, raw := range []string{"query=", "query=%20", "foo=x", "limit=20&limit=20", "query=ab&query=cd", "study_program=math", "graduation_year=0", "graduation_year=no", "limit=51", "limit=0", "skills=go&skills=Go", "query=%00x", "skills=Go%0A", "query=%FF", "bad=%ZZ", "cursor=", "query=" + strings.Repeat("a", 4097)} {
		if _, _, err := parse(raw); err == nil {
			t.Errorf("accepted %s", raw)
		}
	}
	s, _, err := parse("query=Alice&skills=Go&skills=Teaching&institution=uhamka&study_program=math&graduation_year=2020&limit=50")
	if err != nil || s.Query != "alice" || len(s.Skills) != 2 || s.Limit != 50 {
		t.Fatalf("%+v %v", s, err)
	}
}

func TestMethodErrorsAreNotCached(t *testing.T) {
	r := chi.NewRouter()
	RegisterRoutes(r, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", "/directory", nil))
	if w.Code != 405 || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("%d %v", w.Code, w.Header())
	}
}

type failedSearch struct{ err error }

func (s failedSearch) Search(context.Context, directory.Spec) ([]directory.Entry, error) {
	return nil, s.err
}
func TestSafeFailureResponses(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
		body   string
	}{{context.Canceled, 408, "directory_canceled"}, {context.DeadlineExceeded, 503, "directory_timeout"}, {errors.New("private SQL detail"), 500, "internal_error"}} {
		c, _ := directory.NewCursor(make([]byte, 32))
		r := chi.NewRouter()
		RegisterRoutes(r, directory.New(failedSearch{tc.err}, c))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", "/directory", nil))
		if w.Code != tc.status || w.Body.String() != "{\"error\":\""+tc.body+"\"}\n" || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Fatalf("unsafe failure: %d %v %s", w.Code, w.Header(), w.Body.String())
		}
	}
}
