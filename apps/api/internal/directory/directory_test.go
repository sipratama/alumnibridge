package directory

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNormalization(t *testing.T) {
	s, err := Normalize(Spec{Query: "  ÁLUMNA  Example ", Company: " ACME ", Skills: []string{" Go ", "Data   Science"}, Limit: 20})
	if err != nil || s.Query != "álumna example" || s.Company != "acme" || strings.Join(s.Skills, ",") != "data science,go" {
		t.Fatalf("%+v %v", s, err)
	}
	for _, s := range []Spec{{Limit: 0}, {Limit: 51}, {Limit: 20, Query: "a"}, {Limit: 20, Query: strings.Repeat("é", 101)}, {Limit: 20, Query: "ab\x00"}, {Limit: 20, Query: string([]byte{255})}, {Limit: 20, StudyProgram: "math"}, {Limit: 20, Institution: "Bad"}, {Limit: 20, GraduationYear: 1949}, {Limit: 20, Skills: []string{"go", " Go "}}, {Limit: 20, Skills: []string{"1", "2", "3", "4", "5", "6"}}, {Limit: 20, After: "invalid/slug"}} {
		if _, err := Normalize(s); !errors.Is(err, ErrInvalid) {
			t.Errorf("accepted %+v: %v", s, err)
		}
	}
}
func TestCursor(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	c, err := NewCursor(key)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1700000000, 0)
	c.now = func() time.Time { return now }
	spec, _ := Normalize(Spec{Limit: 2, Query: "al"})
	token, err := c.Seal(spec, "alumna-one")
	if err != nil {
		t.Fatal(err)
	}
	if slug, err := c.Open(token, spec); err != nil || slug != "alumna-one" {
		t.Fatalf("%s %v", slug, err)
	}
	raw, _ := base64.RawURLEncoding.DecodeString(token)
	plain, err := c.aead.Open(nil, raw[:c.aead.NonceSize()], raw[c.aead.NonceSize():], cursorPurpose)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err = json.Unmarshal(plain, &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload) != 4 || payload["slug"] != "alumna-one" || strings.Contains(string(plain), "user_id") {
		t.Fatalf("unexpected payload %s", plain)
	}
	raw[len(raw)-1] ^= 1
	for _, bad := range []string{"", "bad!", token + "\n", strings.Repeat("x", 2049), base64.RawURLEncoding.EncodeToString(raw)} {
		if _, err := c.Open(bad, spec); !errors.Is(err, ErrInvalid) {
			t.Errorf("accepted bad cursor: %v", err)
		}
	}
	for _, other := range []Spec{{Limit: 3, Query: "al"}, {Limit: 2, Query: "bo"}} {
		other, _ = Normalize(other)
		if _, err := c.Open(token, other); err == nil {
			t.Fatal("unbound cursor")
		}
	}
	now = now.Add(15 * time.Minute)
	if _, err := c.Open(token, spec); err == nil {
		t.Fatal("expired cursor accepted")
	}
	wrong, _ := NewCursor(make([]byte, 32))
	if _, err := wrong.Open(token, spec); err == nil {
		t.Fatal("wrong key accepted")
	}
	for _, n := range []int{0, 16, 24, 31, 33} {
		if _, err := NewCursor(make([]byte, n)); err == nil {
			t.Fatal("invalid key accepted")
		}
	}
}

type pageRepo struct{ specs []Spec }

func (r *pageRepo) Search(ctx context.Context, s Spec) ([]Entry, error) {
	r.specs = append(r.specs, s)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s.After != "" {
		return []Entry{{Slug: "charlie"}}, nil
	}
	return []Entry{{Slug: "alice"}, {Slug: "bravo"}, {Slug: "charlie"}}, nil
}
func TestServicePaginationAndCancellation(t *testing.T) {
	c, _ := NewCursor(make([]byte, 32))
	r := &pageRepo{}
	s := New(r, c)
	p, err := s.Search(context.Background(), Spec{Limit: 2}, "")
	if err != nil || len(p.Items) != 2 || p.NextCursor == nil {
		t.Fatalf("%+v %v", p, err)
	}
	p, err = s.Search(context.Background(), Spec{Limit: 2}, *p.NextCursor)
	if err != nil || len(p.Items) != 1 || p.NextCursor != nil || r.specs[1].After != "bravo" {
		t.Fatalf("%+v %v", p, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = s.Search(ctx, Spec{Limit: 2}, ""); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
