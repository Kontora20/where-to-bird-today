// Package config loads the server's configuration from environment variables.
//
// Every value is read explicitly, with an explicit default, into a plain
// struct: there is no provider chain and no reflection-based binding.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
)

// Config is the fully resolved configuration for one run of the server.
type Config struct {
	// Port is the TCP port the HTTP server listens on.
	Port int

	// LogLevel is the minimum severity slog will emit.
	LogLevel slog.Level

	// DatabaseURL is the PostgreSQL connection string. It has no default: the
	// server has nothing useful to do without a database, and inventing a
	// localhost fallback would only turn a clear startup failure into a
	// confusing connection error.
	DatabaseURL string

	// EbirdAPIKey authenticates requests to the eBird API. No default, same
	// reasoning as DatabaseURL.
	EbirdAPIKey string

	// Regions lists the eBird region codes hotspot and observation ingestion
	// covers. Defaults to the Baltics.
	Regions []string
}

// Load reads configuration from the environment and applies defaults.
//
// It returns an error rather than calling os.Exit or panicking: deciding how
// to die is main's job, not a library's. That separation is what makes this
// function testable.
func Load() (Config, error) {
	cfg := Config{
		Port:     8080,
		LogLevel: slog.LevelInfo,
		Regions:  []string{"LT", "LV", "EE"},
	}

	if v, ok := os.LookupEnv("PORT"); ok {
		port, err := parsePort(v)
		if err != nil {
			return Config{}, err
		}
		cfg.Port = port
	}

	if v, ok := os.LookupEnv("LOG_LEVEL"); ok {
		level, err := parseLevel(v)
		if err != nil {
			return Config{}, err
		}
		cfg.LogLevel = level
	}

	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required (see .env.example)")
	}
	cfg.DatabaseURL = databaseURL

	// Not required here: most commands (server, migrate, the AviList loader)
	// never touch eBird. Whatever needs it checks for itself.
	cfg.EbirdAPIKey = strings.TrimSpace(os.Getenv("EBIRD_API_KEY"))

	if v, ok := os.LookupEnv("EBIRD_REGIONS"); ok {
		regions, err := parseRegions(v)
		if err != nil {
			return Config{}, err
		}
		cfg.Regions = regions
	}

	return cfg, nil
}

func parsePort(s string) (int, error) {
	port, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0, fmt.Errorf("PORT %q is not a number: %w", s, err)
	}
	if port < 1 || port > 65535 {
		return 0, fmt.Errorf("PORT %d is outside the valid range 1-65535", port)
	}
	return port, nil
}

func parseRegions(s string) ([]string, error) {
	parts := strings.Split(s, ",")
	regions := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			regions = append(regions, part)
		}
	}
	if len(regions) == 0 {
		return nil, fmt.Errorf("EBIRD_REGIONS %q has no region codes", s)
	}
	return regions, nil
}

func parseLevel(s string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("LOG_LEVEL %q is not one of: debug, info, warn, error", s)
	}
}
