package profileshttp

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStrictDiscoveryJSON(t *testing.T) {
	for _, body := range []string{`null`, `[]`, `{}`, `{"directory_discoverable":null}`, `{"directory_discoverable":true,"owner_id":"x"}`, `{"directory_discoverable":true,"directory_discoverable":false}`, `{"affiliations":null}`, `{"affiliations":[null]}`, `{"affiliations":[{"membership_id":"00000000-0000-0000-0000-000000000001","show_institution":true}]}`, `{"directory_discoverable":false} {}`, `{"directory_discoverable":true,"verified_alumni":true}`, `{"affiliations":[{"membership_id":"00000000-0000-0000-0000-000000000001","show_institution":null,"show_study_program":false,"show_graduation_year":false}]}`} {
		r := httptest.NewRequest("PATCH", "/", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if _, err := decodeDiscovery(r); err == nil {
			t.Errorf("accepted %s", body)
		}
	}
	for _, body := range []string{`{"directory_discoverable":false}`, `{"affiliations":[]}`} {
		r := httptest.NewRequest("PATCH", "/", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if _, err := decodeDiscovery(r); err != nil {
			t.Errorf("%s: %v", body, err)
		}
	}
	r := httptest.NewRequest("PATCH", "/", strings.NewReader(strings.Repeat(" ", 16385)))
	r.Header.Set("Content-Type", "application/json")
	if _, err := decodeDiscovery(r); err != errBodyLarge {
		t.Fatal(err)
	}
	r = httptest.NewRequest("PATCH", "/", strings.NewReader(`{}`))
	r.Header.Set("Content-Type", "text/plain")
	if _, err := decodeDiscovery(r); err != errMediaType {
		t.Fatal(err)
	}
}
