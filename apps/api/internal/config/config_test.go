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
	t.Setenv("OBJECT_STORAGE_ENDPOINT", "localhost:19000")
	t.Setenv("OBJECT_STORAGE_ACCESS_KEY", "test")
	t.Setenv("OBJECT_STORAGE_SECRET_KEY", "test-secret")
	t.Setenv("OBJECT_STORAGE_BUCKET", "test-bucket")
	t.Setenv("DIRECTORY_CURSOR_KEY", "L1vAgBrfwBBurWKc4D9fp3LXZEnC9h5Bq/HL7nVWLS8=")
	cfg, err := Load()
	if err != nil || cfg.Address() != ":8080" {
		t.Fatalf("unexpected config: %+v, %v", cfg, err)
	}
}

func TestCursorKeyConfiguration(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
	t.Setenv("OIDC_ISSUER", "http://localhost:18085/realms/alumnibridge")
	t.Setenv("OIDC_AUDIENCE", "alumnibridge-api")
	t.Setenv("OIDC_CLIENT_ID", "alumnibridge-web")
	t.Setenv("API_PORT", "8080")
	t.Setenv("OBJECT_STORAGE_ENDPOINT", "localhost:19000")
	t.Setenv("OBJECT_STORAGE_ACCESS_KEY", "test")
	t.Setenv("OBJECT_STORAGE_SECRET_KEY", "test")
	t.Setenv("OBJECT_STORAGE_BUCKET", "test")
	for _, key := range []string{"", "not base64", "YWJj", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=\n"} {
		t.Setenv("DIRECTORY_CURSOR_KEY", key)
		if _, err := Load(); err == nil {
			t.Fatal("invalid cursor key accepted")
		}
	}
}
