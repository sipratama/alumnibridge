package config

import (
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
	StorageEndpoint  string
	StorageAccessKey string
	StorageSecretKey string
	StorageBucket    string
	StorageSecure    bool
	Port             string
	DatabaseURL      string
	OIDCIssuer       string
	OIDCAudience     string
	OIDCClientID     string
	OIDCJWKSURL      string
	ShutdownTimeout  time.Duration
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
	jwksURL := getenv("OIDC_JWKS_URL", issuer+"/protocol/openid-connect/certs")
	return Config{StorageEndpoint: storageEndpoint, StorageAccessKey: storageAccess, StorageSecretKey: storageSecret, StorageBucket: storageBucket, StorageSecure: storageSecure, Port: port, DatabaseURL: url, OIDCIssuer: issuer, OIDCAudience: audience, OIDCClientID: clientID, OIDCJWKSURL: jwksURL, ShutdownTimeout: 10 * time.Second}, nil
}

func (c Config) Address() string { return net.JoinHostPort("", c.Port) }

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
