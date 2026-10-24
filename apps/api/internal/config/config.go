package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	urlpkg "net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	DirectoryCursorKey        []byte
	DirectoryRequestTimeout   time.Duration
	DirectoryStatementTimeout time.Duration
	DirectoryMaxConcurrent    int
	DirectoryRatePerMinute    int
	DirectoryBurst            int
	DirectoryMaxClients       int
	DirectoryClientTTL        time.Duration
	StorageEndpoint           string
	StorageAccessKey          string
	StorageSecretKey          string
	StorageBucket             string
	StorageSecure             bool
	Port                      string
	DatabaseURL               string
	OIDCIssuer                string
	OIDCAudience              string
	OIDCClientID              string
	OIDCJWKSURL               string
	ShutdownTimeout           time.Duration
}

func Load() (Config, error) {
	port := getenv("API_PORT", "8080")
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return Config{}, fmt.Errorf("invalid API_PORT: %q", port)
	}
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}
	issuer := strings.TrimRight(os.Getenv("OIDC_ISSUER"), "/")
	audience := os.Getenv("OIDC_AUDIENCE")
	clientID := os.Getenv("OIDC_CLIENT_ID")
	parsed, parseErr := urlpkg.Parse(issuer)
	if parseErr != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || audience == "" || clientID == "" {
		return Config{}, errors.New("OIDC_ISSUER, OIDC_AUDIENCE, and OIDC_CLIENT_ID are required")
	}
	storageEndpoint := os.Getenv("OBJECT_STORAGE_ENDPOINT")
	storageAccess := os.Getenv("OBJECT_STORAGE_ACCESS_KEY")
	storageSecret := os.Getenv("OBJECT_STORAGE_SECRET_KEY")
	storageBucket := os.Getenv("OBJECT_STORAGE_BUCKET")
	storageSecure, err := strconv.ParseBool(getenv("OBJECT_STORAGE_SECURE", "true"))
	if err != nil || storageEndpoint == "" || storageAccess == "" || storageSecret == "" || storageBucket == "" {
		return Config{}, errors.New("object storage configuration is required")
	}
	cursorKey, keyErr := base64.StdEncoding.Strict().DecodeString(os.Getenv("DIRECTORY_CURSOR_KEY"))
	if keyErr != nil || len(cursorKey) != 32 || base64.StdEncoding.EncodeToString(cursorKey) != os.Getenv("DIRECTORY_CURSOR_KEY") {
		return Config{}, errors.New("DIRECTORY_CURSOR_KEY must be canonical base64 encoding of 32 random bytes")
	}
	limits := map[string]struct{ fallback, min, max int }{
		"DIRECTORY_REQUEST_TIMEOUT_MS":   {2000, 100, 9000},
		"DIRECTORY_STATEMENT_TIMEOUT_MS": {1500, 1, 8999},
		"DIRECTORY_MAX_CONCURRENT":       {8, 1, 64},
		"DIRECTORY_RATE_PER_MINUTE":      {60, 1, 60000},
		"DIRECTORY_BURST":                {10, 1, 1000},
		"DIRECTORY_MAX_CLIENTS":          {10000, 1, 100000},
		"DIRECTORY_CLIENT_TTL_SECONDS":   {600, 60, 86400},
	}
	values := make(map[string]int, len(limits))
	for key, limit := range limits {
		n, err := strconv.Atoi(getenv(key, strconv.Itoa(limit.fallback)))
		if err != nil || n < limit.min || n > limit.max {
			return Config{}, fmt.Errorf("invalid %s", key)
		}
		values[key] = n
	}
	if values["DIRECTORY_STATEMENT_TIMEOUT_MS"] >= values["DIRECTORY_REQUEST_TIMEOUT_MS"] {
		return Config{}, errors.New("directory statement timeout must be shorter than request timeout")
	}
	if int64(values["DIRECTORY_CLIENT_TTL_SECONDS"])*int64(values["DIRECTORY_RATE_PER_MINUTE"]) < int64(values["DIRECTORY_BURST"])*60 {
		return Config{}, errors.New("directory client TTL must cover full burst refill")
	}
	jwksURL := getenv("OIDC_JWKS_URL", issuer+"/protocol/openid-connect/certs")
	return Config{
		DirectoryRequestTimeout:   time.Duration(values["DIRECTORY_REQUEST_TIMEOUT_MS"]) * time.Millisecond,
		DirectoryStatementTimeout: time.Duration(values["DIRECTORY_STATEMENT_TIMEOUT_MS"]) * time.Millisecond,
		DirectoryMaxConcurrent:    values["DIRECTORY_MAX_CONCURRENT"],
		DirectoryRatePerMinute:    values["DIRECTORY_RATE_PER_MINUTE"],
		DirectoryBurst:            values["DIRECTORY_BURST"],
		DirectoryMaxClients:       values["DIRECTORY_MAX_CLIENTS"],
		DirectoryClientTTL:        time.Duration(values["DIRECTORY_CLIENT_TTL_SECONDS"]) * time.Second,
		DirectoryCursorKey:        cursorKey, StorageEndpoint: storageEndpoint, StorageAccessKey: storageAccess, StorageSecretKey: storageSecret, StorageBucket: storageBucket, StorageSecure: storageSecure, Port: port, DatabaseURL: url, OIDCIssuer: issuer, OIDCAudience: audience, OIDCClientID: clientID, OIDCJWKSURL: jwksURL, ShutdownTimeout: 10 * time.Second}, nil
}

func (c Config) Address() string { return net.JoinHostPort("", c.Port) }

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
