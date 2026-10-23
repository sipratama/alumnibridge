package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestJWTVerifier(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{map[string]string{
			"kty": "RSA", "kid": "test", "use": "sig", "alg": "RS256",
			"n": base64.RawURLEncoding.EncodeToString(key.PublicKey.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.PublicKey.E)).Bytes()),
		}}})
	}))
	defer jwks.Close()
	verifier, err := NewJWTVerifier(context.Background(), jwks.URL, "https://issuer.example/realm", "alumnibridge-api", "alumnibridge-web")
	if err != nil {
		t.Fatal(err)
	}
	base := func() jwt.MapClaims {
		return jwt.MapClaims{
			"iss": "https://issuer.example/realm", "aud": "alumnibridge-api", "sub": "subject-1",
			"exp": time.Now().Add(time.Hour).Unix(), "typ": "Bearer", "azp": "alumnibridge-web",
			"realm_access": map[string]any{"roles": []string{"alumnibridge-admin"}},
		}
	}
	sign := func(claims jwt.MapClaims, signingKey *rsa.PrivateKey) string {
		t.Helper()
		token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
		token.Header["kid"] = "test"
		raw, err := token.SignedString(signingKey)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	valid := sign(base(), key)
	actor, err := verifier.Verify(context.Background(), valid)
	if err != nil || actor.Subject != "subject-1" || !actor.Roles["alumnibridge-admin"] {
		t.Fatalf("valid token: actor=%+v err=%v", actor, err)
	}

	cases := map[string]func(jwt.MapClaims){
		"expired":         func(c jwt.MapClaims) { c["exp"] = time.Now().Add(-time.Hour).Unix() },
		"missing expiry":  func(c jwt.MapClaims) { delete(c, "exp") },
		"wrong issuer":    func(c jwt.MapClaims) { c["iss"] = "https://other.example" },
		"wrong audience":  func(c jwt.MapClaims) { c["aud"] = "other-api" },
		"wrong type":      func(c jwt.MapClaims) { c["typ"] = "ID" },
		"wrong client":    func(c jwt.MapClaims) { c["azp"] = "other-client" },
		"missing subject": func(c jwt.MapClaims) { delete(c, "sub") },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			claims := base()
			mutate(claims)
			if _, err := verifier.Verify(context.Background(), sign(claims, key)); err == nil {
				t.Fatal("expected rejection")
			}
		})
	}
	t.Run("invalid signature", func(t *testing.T) {
		otherKey, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := verifier.Verify(context.Background(), sign(base(), otherKey)); err == nil {
			t.Fatal("expected rejection")
		}
	})
	t.Run("malformed", func(t *testing.T) {
		if _, err := verifier.Verify(context.Background(), "not.a.jwt"); err == nil {
			t.Fatal("expected rejection")
		}
	})
}
