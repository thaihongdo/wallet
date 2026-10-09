// Package config loads service configuration from environment
// variables. There is no config library: stdlib os only, per the
// project's tech-stack constraint.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds every environment-derived setting the service needs.
type Config struct {
	Port            string
	DatabaseURL     string
	RedisURL        string
	APIKeys         []string
	RateLimitPerMin int
	DBMaxConns      int32
	LogLevel        string
	GinMode         string
	ShutdownTimeout time.Duration
}

const (
	defaultPort            = "8080"
	defaultRateLimitPerMin = 60
	defaultDBMaxConns      = 5
	defaultLogLevel        = "info"
	defaultGinMode         = "release"
	defaultShutdownTimeout = 10 * time.Second
)

// Load reads configuration from the environment, applying defaults for
// optional variables and defaults. It returns an error listing every
// missing required variable (DATABASE_URL, REDIS_URL, API_KEYS) at
// once, rather than failing on the first one found.
func Load() (Config, error) {
	cfg := Config{
		Port:            getEnvOr("PORT", defaultPort),
		DatabaseURL:     os.Getenv("DATABASE_URL"),
		RedisURL:        os.Getenv("REDIS_URL"),
		LogLevel:        getEnvOr("LOG_LEVEL", defaultLogLevel),
		GinMode:         getEnvOr("GIN_MODE", defaultGinMode),
		RateLimitPerMin: defaultRateLimitPerMin,
		DBMaxConns:      defaultDBMaxConns,
		ShutdownTimeout: defaultShutdownTimeout,
	}

	var missing []string
	if cfg.DatabaseURL == "" {
		missing = append(missing, "DATABASE_URL")
	}
	if cfg.RedisURL == "" {
		missing = append(missing, "REDIS_URL")
	}
	if raw := os.Getenv("API_KEYS"); raw == "" {
		missing = append(missing, "API_KEYS")
	} else {
		cfg.APIKeys = splitAndTrim(raw)
	}

	if raw := os.Getenv("RATE_LIMIT_PER_MIN"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			return Config{}, fmt.Errorf("parse RATE_LIMIT_PER_MIN: %w", err)
		}
		cfg.RateLimitPerMin = n
	}
	if raw := os.Getenv("DB_MAX_CONNS"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			return Config{}, fmt.Errorf("parse DB_MAX_CONNS: %w", err)
		}
		cfg.DBMaxConns = int32(n)
	}
	if raw := os.Getenv("SHUTDOWN_TIMEOUT"); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil {
			return Config{}, fmt.Errorf("parse SHUTDOWN_TIMEOUT: %w", err)
		}
		cfg.ShutdownTimeout = d
	}

	if len(missing) > 0 {
		return Config{}, fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}
	return cfg, nil
}

func getEnvOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func splitAndTrim(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
