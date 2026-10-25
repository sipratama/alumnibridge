package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/sipratama/alumnibridge/apps/api/internal/identity"
)

var ErrInvalidToken = errors.New("invalid access token")
var ErrAccessUnavailable = errors.New("application access check unavailable")

// AccessChecker reads current application access state after a token is
// verified. Unknown subjects may enroll; blocked or non-active subjects fail.
type AccessChecker interface {
	CheckAccess(context.Context, string, string) error
}

type Verifier interface {
	Verify(context.Context, string) (identity.Principal, error)
}

type JWTVerifier struct {
	keys     keyfunc.Keyfunc
	issuer   string
	audience string
	clientID string
	access   AccessChecker
}

func NewJWTVerifier(ctx context.Context, jwksURL, issuer, audience, clientID string, access AccessChecker) (*JWTVerifier, error) {
	keys, err := keyfunc.NewDefaultCtx(ctx, []string{jwksURL})
	if err != nil {
		return nil, err
	}
	return &JWTVerifier{keys: keys, issuer: issuer, audience: audience, clientID: clientID, access: access}, nil
}

type accessClaims struct {
	jwt.RegisteredClaims
	Type            string `json:"typ"`
	AuthorizedParty string `json:"azp"`
	Email           string `json:"email"`
	RealmAccess     struct {
		Roles []string `json:"roles"`
	} `json:"realm_access"`
}

func (v *JWTVerifier) Verify(ctx context.Context, raw string) (identity.Principal, error) {
	claims := &accessClaims{}
	token, err := jwt.ParseWithClaims(raw, claims, v.keys.Keyfunc,
		jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg()}),
		jwt.WithIssuer(v.issuer), jwt.WithAudience(v.audience), jwt.WithExpirationRequired())
	if err != nil || !token.Valid || strings.TrimSpace(claims.Subject) == "" || claims.Type != "Bearer" || claims.AuthorizedParty != v.clientID {
		return identity.Principal{}, ErrInvalidToken
	}
	if v.access == nil {
		return identity.Principal{}, ErrAccessUnavailable
	}
	if err := v.access.CheckAccess(ctx, claims.Issuer, claims.Subject); err != nil {
		if errors.Is(err, identity.ErrAccessDenied) {
			return identity.Principal{}, identity.ErrAccessDenied
		}
		return identity.Principal{}, fmt.Errorf("%w: %v", ErrAccessUnavailable, err)
	}
	roles := make(map[string]bool, len(claims.RealmAccess.Roles))
	for _, role := range claims.RealmAccess.Roles {
		roles[role] = true
	}
	return identity.Principal{Subject: claims.Subject, Email: claims.Email, Roles: roles}, nil
}
