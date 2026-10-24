package profiles

import (
	"context"
	"time"

	"github.com/google/uuid"
)

const MaxDirectoryAffiliations = 20

// PubliclyVisible is shared by direct publication and directory eligibility.
func PubliclyVisible(visibility, moderation string) bool {
	return visibility == "public" && moderation == "visible"
}
func DiscoveryEligible(visibility, moderation string, verified bool) bool {
	return PubliclyVisible(visibility, moderation) && verified
}

type AcademicConsent struct {
	MembershipID       uuid.UUID `json:"membership_id"`
	ShowInstitution    bool      `json:"show_institution"`
	ShowStudyProgram   bool      `json:"show_study_program"`
	ShowGraduationYear bool      `json:"show_graduation_year"`
}
type DiscoveryMembership struct {
	AcademicConsent
	Institution    string `json:"institution"`
	StudyProgram   string `json:"study_program"`
	GraduationYear int    `json:"graduation_year"`
}
type Discovery struct {
	DirectoryDiscoverable bool                  `json:"directory_discoverable"`
	DirectoryConsentedAt  *time.Time            `json:"directory_consented_at"`
	Eligible              bool                  `json:"eligible"`
	EffectiveDiscoverable bool                  `json:"effective_discoverable"`
	Revision              int64                 `json:"-"`
	Memberships           []DiscoveryMembership `json:"memberships"`
}
type DiscoveryPatch struct {
	DirectoryDiscoverable *bool
	Affiliations          *[]AcademicConsent
}

func (p DiscoveryPatch) Validate() error {
	if p.DirectoryDiscoverable == nil && p.Affiliations == nil {
		return ErrInvalid
	}
	if p.Affiliations == nil {
		return nil
	}
	if len(*p.Affiliations) > MaxDirectoryAffiliations {
		return ErrInvalid
	}
	seen := map[uuid.UUID]bool{}
	for _, a := range *p.Affiliations {
		if a.MembershipID == uuid.Nil || seen[a.MembershipID] || (!a.ShowInstitution && (a.ShowStudyProgram || a.ShowGraduationYear)) {
			return ErrInvalid
		}
		if p.DirectoryDiscoverable != nil && !*p.DirectoryDiscoverable && (a.ShowInstitution || a.ShowStudyProgram || a.ShowGraduationYear) {
			return ErrInvalid
		}
		seen[a.MembershipID] = true
	}
	return nil
}

type DiscoveryStore interface {
	Discovery(context.Context, uuid.UUID) (Discovery, error)
	UpdateDiscovery(context.Context, uuid.UUID, DiscoveryPatch) (Discovery, error)
}
type ConditionalDiscoveryStore interface {
 UpdateDiscoveryConditional(context.Context, uuid.UUID, DiscoveryPatch, int64) (Discovery, error)
}
type DiscoveryService struct{ store DiscoveryStore }

func NewDiscovery(store DiscoveryStore) *DiscoveryService { return &DiscoveryService{store: store} }
func (s *DiscoveryService) Discovery(ctx context.Context, owner uuid.UUID) (Discovery, error) {
	return s.store.Discovery(ctx, owner)
}
func (s *DiscoveryService) UpdateConditional(ctx context.Context, owner uuid.UUID, patch DiscoveryPatch, expected int64) (Discovery, error) {
 if err := patch.Validate(); err != nil { return Discovery{}, err }
 if expected < 1 { return Discovery{}, ErrInvalid }
 store, ok := s.store.(ConditionalDiscoveryStore)
 if !ok { return Discovery{}, ErrInvalid }
 return store.UpdateDiscoveryConditional(ctx, owner, patch, expected)
}
func (s *DiscoveryService) Update(ctx context.Context, owner uuid.UUID, patch DiscoveryPatch) (Discovery, error) {
	if err := patch.Validate(); err != nil {
		return Discovery{}, err
	}
	return s.store.UpdateDiscovery(ctx, owner, patch)
}
