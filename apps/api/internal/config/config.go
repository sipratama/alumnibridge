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
	Port            string
	DatabaseURL     string
	OIDCIssuer      string
	OIDCAudience    string
	OIDCClientID    string
	OIDCJWKSURL     string
	ShutdownTimeout time.Duration
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
	jwksURL := getenv("OIDC_JWKS_URL", issuer+"/protocol/openid-connect/certs")
	return Config{Port: port, DatabaseURL: url, OIDCIssuer: issuer, OIDCAudience: audience, OIDCClientID: clientID, OIDCJWKSURL: jwksURL, ShutdownTimeout: 10 * time.Second}, nil
}

func (c Config) Address() string { return net.JoinHostPort("", c.Port) }

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
