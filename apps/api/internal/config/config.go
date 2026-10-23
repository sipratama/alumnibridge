package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Port            string
	DatabaseURL     string
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
	return Config{Port: port, DatabaseURL: url, ShutdownTimeout: 10 * time.Second}, nil
}

func (c Config) Address() string { return net.JoinHostPort("", c.Port) }

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
