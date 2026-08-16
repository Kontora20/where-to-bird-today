// Package config loads the server's configuration from environment variables.
//
// Coming from .NET: this replaces appsettings.json, IConfiguration, and
// IOptions<T>. There is no provider chain and no reflection-based binding —
// just explicit reads with explicit defaults, returning a plain struct.
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
//
// Fields are added as phases need them: EBIRD_API_KEY arrives in Phase 2. It is
// documented in .env.example already, but is deliberately not read here yet —
// unused config is dead code.
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
