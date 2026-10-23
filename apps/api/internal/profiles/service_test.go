package profiles

import (
	"strings"
	"testing"
)

func TestSlugRules(t *testing.T) {
	for input, want := range map[string]string{" Jane  DOE! ": "jane-doe", "A/B": "a-b", "  API ": "api", "Émily Smith": "mily-smith"} {
		if got := NormalizeSlug(input); got != want {
			t.Errorf("%q: %q != %q", input, got, want)
		}
	}
	for _, slug := range []string{"api", "admin", "a", "bad_thing", "has--gap"} {
		if ValidSlug(slug) {
			t.Errorf("accepted %q", slug)
		}
	}
	if !ValidSlug("jane-doe") {
		t.Fatal("valid slug rejected")
	}
}
func TestCompleteness(t *testing.T) {
	if got := Completeness(Profile{DisplayName: "Jane", Headline: "Engineer"}, [3]int{1, 0, 1}); got != 44 {
		t.Fatalf("got %d", got)
	}
}
func TestExperienceDates(t *testing.T) {
	end := "2020-01-01"
	valid := Experience{Company: " Acme ", JobTitle: " Developer ", StartDate: "2021-01-01"}
	if _, err := NormalizeExperience(valid); err != nil {
		t.Fatal(err)
	}
	valid.EndDate = &end
	if _, err := NormalizeExperience(valid); err != ErrInvalid {
		t.Fatal("end before start accepted")
	}
	valid.EndDate = nil
	valid.IsCurrent = true
	if _, err := NormalizeExperience(valid); err != nil {
		t.Fatal(err)
	}
	valid.EndDate = &end
	if _, err := NormalizeExperience(valid); err != ErrInvalid {
		t.Fatal("current with end accepted")
	}
	valid.StartDate = "2021-02-31"
	valid.EndDate = nil
	if _, err := NormalizeExperience(valid); err != ErrInvalid {
		t.Fatal("invalid date accepted")
	}
}
func TestEducationDates(t *testing.T) {
	start, end := "2022-01-01", "2021-01-01"
	e := Education{Institution: "Some University", StartDate: &start, EndDate: &end}
	if _, err := NormalizeEducation(e); err != ErrInvalid {
		t.Fatal("backwards dates accepted")
	}
	e.EndDate = nil
	if _, err := NormalizeEducation(e); err != nil {
		t.Fatal(err)
	}
}
func TestSkillNormalization(t *testing.T) {
	name, key, err := NormalizeSkill("  Data   Science ")
	if err != nil || name != "Data Science" || key != "data science" {
		t.Fatalf("%q %q %v", name, key, err)
	}
}

func TestReviewTextAndSlugLimits(t *testing.T) {
	if ValidSlug(strings.Repeat("a", 65)) || !ValidSlug(strings.Repeat("a", 64)) {
		t.Fatal("slug boundary")
	}
	if ValidSlug(NormalizeSlug("你好")) {
		t.Fatal("empty normalized Unicode accepted")
	}
	if NormalizeSlug("Jane DOE") != "jane-doe" {
		t.Fatal("case normalization")
	}
	if _, _, err := NormalizeSkill(strings.Repeat("界", 80)); err != nil {
		t.Fatal("rune limit rejected Unicode", err)
	}
	if _, _, err := NormalizeSkill("Go\x00lang"); err != ErrInvalid {
		t.Fatal("control accepted")
	}
	p := Profile{DisplayName: "Jane", Slug: "jane-doe"}
	bad := "bad\x00value"
	if _, err := ApplyProfilePatch(p, ProfilePatch{Bio: &bad}); err != ErrInvalid {
		t.Fatal("NUL accepted")
	}
}
