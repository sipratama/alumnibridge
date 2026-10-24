package profiles

import (
	"github.com/google/uuid"
	"testing"
)

func TestDiscoveryPolicy(t *testing.T) {
	for _, v := range []struct {
		visibility, moderation string
		verified, want         bool
	}{{"public", "visible", true, true}, {"private", "visible", true, false}, {"public", "hidden", true, false}, {"public", "visible", false, false}} {
		if DiscoveryEligible(v.visibility, v.moderation, v.verified) != v.want {
			t.Fatal(v)
		}
	}
	id := uuid.New()
	yes, no := true, false
	bad := [][]AcademicConsent{{{MembershipID: id, ShowStudyProgram: true}}, {{MembershipID: id, ShowGraduationYear: true}}, {{MembershipID: uuid.Nil}}, {{MembershipID: id}, {MembershipID: id}}, make([]AcademicConsent, 21)}
	for _, a := range bad {
		if err := (DiscoveryPatch{DirectoryDiscoverable: &yes, Affiliations: &a}).Validate(); err == nil {
			t.Fatal("accepted invalid consent")
		}
	}
	if err := (DiscoveryPatch{}).Validate(); err == nil {
		t.Fatal("empty patch")
	}
	a := []AcademicConsent{{MembershipID: id, ShowInstitution: true}}
	if err := (DiscoveryPatch{DirectoryDiscoverable: &no, Affiliations: &a}).Validate(); err == nil {
		t.Fatal("contradictory opt-out")
	}
	twenty := make([]AcademicConsent, 20)
	for i := range twenty {
		twenty[i] = AcademicConsent{MembershipID: uuid.New(), ShowInstitution: true}
	}
	if err := (DiscoveryPatch{DirectoryDiscoverable: &yes, Affiliations: &twenty}).Validate(); err != nil {
		t.Fatal(err)
	}
	empty := []AcademicConsent{}
	if err := (DiscoveryPatch{Affiliations: &empty}).Validate(); err != nil {
		t.Fatal(err)
	}
}
