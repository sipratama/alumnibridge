package auth

import (
	"context"
	"errors"
	"strings"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/sipratama/alumnibridge/apps/api/internal/identity"
)

var ErrInvalidToken = errors.New("invalid access token")

type Verifier interface {
	Verify(context.Context, string) (identity.Principal, error)
}

type JWTVerifier struct {
	keys     keyfunc.Keyfunc
	issuer   string
	audience string
	clientID string
}

func NewJWTVerifier(ctx context.Context, jwksURL, issuer, audience, clientID string) (*JWTVerifier, error) {
	keys, err := keyfunc.NewDefaultCtx(ctx, []string{jwksURL})
	if err != nil {
		return nil, err
	}
	return &JWTVerifier{keys: keys, issuer: issuer, audience: audience, clientID: clientID}, nil
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

func (v *JWTVerifier) Verify(_ context.Context, raw string) (identity.Principal, error) {
	claims := &accessClaims{}
	token, err := jwt.ParseWithClaims(raw, claims, v.keys.Keyfunc,
		jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg()}),
		jwt.WithIssuer(v.issuer), jwt.WithAudience(v.audience), jwt.WithExpirationRequired())
	if err != nil || !token.Valid || strings.TrimSpace(claims.Subject) == "" || claims.Type != "Bearer" || claims.AuthorizedParty != v.clientID {
		return identity.Principal{}, ErrInvalidToken
	}
	roles := make(map[string]bool, len(claims.RealmAccess.Roles))
	for _, role := range claims.RealmAccess.Roles {
		roles[role] = true
	}
	return identity.Principal{Subject: claims.Subject, Email: claims.Email, Roles: roles}, nil
}
