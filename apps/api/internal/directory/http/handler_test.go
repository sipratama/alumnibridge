package directoryhttp

import (
	"github.com/go-chi/chi/v5"
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
