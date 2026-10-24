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
	if cfg.DirectoryRequestTimeout.Milliseconds() != 2000 || cfg.DirectoryStatementTimeout.Milliseconds() != 1500 || cfg.DirectoryMaxConcurrent != 8 || cfg.DirectoryRatePerMinute != 60 || cfg.DirectoryBurst != 10 || cfg.DirectoryMaxClients != 10000 || cfg.DirectoryClientTTL.Seconds() != 600 {
		t.Fatal("unexpected directory defaults")
	}
	for _, tc := range []struct{ key, value string }{
		{"DIRECTORY_REQUEST_TIMEOUT_MS", "0"}, {"DIRECTORY_REQUEST_TIMEOUT_MS", "10000"},
		{"DIRECTORY_STATEMENT_TIMEOUT_MS", "2000"}, {"DIRECTORY_STATEMENT_TIMEOUT_MS", "oops"},
		{"DIRECTORY_MAX_CONCURRENT", "0"}, {"DIRECTORY_MAX_CONCURRENT", "65"},
		{"DIRECTORY_RATE_PER_MINUTE", "0"}, {"DIRECTORY_BURST", "1001"},
		{"DIRECTORY_MAX_CLIENTS", "100001"}, {"DIRECTORY_CLIENT_TTL_SECONDS", "1"},
	} {
		t.Run(tc.key+tc.value, func(t *testing.T) {
			t.Setenv(tc.key, tc.value)
			if _, err := Load(); err == nil {
				t.Fatal("invalid search configuration accepted")
			}
		})
	}
	t.Run("TTL below burst refill", func(t *testing.T) {
		t.Setenv("DIRECTORY_RATE_PER_MINUTE", "1")
		t.Setenv("DIRECTORY_CLIENT_TTL_SECONDS", "60")
		if _, err := Load(); err == nil {
			t.Fatal("TTL permits fresh-burst bypass")
		}
	})
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
