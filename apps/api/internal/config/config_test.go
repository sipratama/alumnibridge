package config

import "testing"

func TestLoad(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	if _, err := Load(); err == nil {
		t.Fatal("expected missing database URL error")
	}
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
	t.Setenv("OIDC_ISSUER", "http://localhost:18085/realms/alumnibridge")
	t.Setenv("OIDC_AUDIENCE", "alumnibridge-api")
	t.Setenv("OIDC_CLIENT_ID", "alumnibridge-web")
	t.Setenv("API_PORT", "invalid")
	if _, err := Load(); err == nil {
		t.Fatal("expected invalid port error")
	}
	t.Setenv("API_PORT", "8080")
	cfg, err := Load()
	if err != nil || cfg.Address() != ":8080" {
		t.Fatalf("unexpected config: %+v, %v", cfg, err)
	}
}
