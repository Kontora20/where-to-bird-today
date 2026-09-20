package config

import (
	"log/slog"
	"os"
	"slices"
	"testing"
)

// clearEnv removes the variables Load reads, so a value set on the developer's
// machine cannot change a test's result.
//
// t.Setenv is called first even though the value is thrown away: it is what
// registers the cleanup that restores the original value when the test ends.
// os.Unsetenv then removes the variable entirely, which is different from
// setting it to "" — Load distinguishes "absent" from "present but empty".
func clearEnv(t *testing.T, keys ...string) {
	t.Helper()

	for _, key := range keys {
		t.Setenv(key, "")
		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("unsetting %s: %v", key, err)
		}
	}
}

// TestLoad covers the environment cases Load has to get right.
//
// None of these subtests call t.Parallel: the environment is process-wide
// state, and the testing package refuses to combine t.Setenv with parallel
// tests rather than trusting us to avoid the race.
func TestLoad(t *testing.T) {
	tests := []struct {
		name            string
		env             map[string]string
		wantErr         bool
		wantPort        int
		wantLevel       slog.Level
		wantDatabaseURL string
		wantEbirdAPIKey string
		wantOSRMURL     string
		wantRegions     []string
	}{
		{
			name: "defaults apply when only DATABASE_URL is set",
			env: map[string]string{
				"DATABASE_URL": "postgres://localhost/wheretobird",
			},
			wantPort:        8080,
			wantLevel:       slog.LevelInfo,
			wantDatabaseURL: "postgres://localhost/wheretobird",
			wantRegions:     []string{"LT", "LV", "EE"},
		},
		{
			name: "every variable is read",
			env: map[string]string{
				"PORT":         "9000",
				"LOG_LEVEL":    "debug",
				"DATABASE_URL": "postgres://localhost/other",
			},
			wantPort:        9000,
			wantLevel:       slog.LevelDebug,
			wantDatabaseURL: "postgres://localhost/other",
			wantRegions:     []string{"LT", "LV", "EE"},
		},
		{
			name:    "missing DATABASE_URL is an error",
			env:     map[string]string{},
			wantErr: true,
		},
		{
			name: "blank DATABASE_URL is an error",
			env: map[string]string{
				"DATABASE_URL": "   ",
			},
			wantErr: true,
		},
		{
			name: "unparsable PORT is an error",
			env: map[string]string{
				"PORT":         "eight thousand",
				"DATABASE_URL": "postgres://localhost/wheretobird",
			},
			wantErr: true,
		},
		{
			name: "EBIRD_API_KEY is read but stays optional",
			env: map[string]string{
				"DATABASE_URL":  "postgres://localhost/wheretobird",
				"EBIRD_API_KEY": "test-key",
			},
			wantPort:        8080,
			wantLevel:       slog.LevelInfo,
			wantDatabaseURL: "postgres://localhost/wheretobird",
			wantEbirdAPIKey: "test-key",
			wantRegions:     []string{"LT", "LV", "EE"},
		},
		{
			name: "OSRM_URL is read but stays optional",
			env: map[string]string{
				"DATABASE_URL": "postgres://localhost/wheretobird",
				"OSRM_URL":     " http://127.0.0.1:5000 ",
			},
			wantPort:        8080,
			wantLevel:       slog.LevelInfo,
			wantDatabaseURL: "postgres://localhost/wheretobird",
			wantOSRMURL:     "http://127.0.0.1:5000",
			wantRegions:     []string{"LT", "LV", "EE"},
		},
		{
			name: "OSRM_URL that is not an http URL is an error",
			env: map[string]string{
				"DATABASE_URL": "postgres://localhost/wheretobird",
				"OSRM_URL":     "127.0.0.1:5000",
			},
			wantErr: true,
		},
		{
			name: "EBIRD_REGIONS overrides the Baltics default",
			env: map[string]string{
				"DATABASE_URL":  "postgres://localhost/wheretobird",
				"EBIRD_REGIONS": " US-CA , US-NY ,, ",
			},
			wantPort:        8080,
			wantLevel:       slog.LevelInfo,
			wantDatabaseURL: "postgres://localhost/wheretobird",
			wantRegions:     []string{"US-CA", "US-NY"},
		},
		{
			name: "blank EBIRD_REGIONS is an error",
			env: map[string]string{
				"DATABASE_URL":  "postgres://localhost/wheretobird",
				"EBIRD_REGIONS": "  ,  ",
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clearEnv(t, "PORT", "LOG_LEVEL", "DATABASE_URL", "EBIRD_API_KEY", "EBIRD_REGIONS", "OSRM_URL")
			for key, value := range tc.env {
				t.Setenv(key, value)
			}

			cfg, err := Load()

			if tc.wantErr {
				if err == nil {
					t.Fatalf("Load() succeeded with %+v, want an error", cfg)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load() failed: %v", err)
			}

			if cfg.Port != tc.wantPort {
				t.Errorf("Load().Port = %d, want %d", cfg.Port, tc.wantPort)
			}
			if cfg.LogLevel != tc.wantLevel {
				t.Errorf("Load().LogLevel = %v, want %v", cfg.LogLevel, tc.wantLevel)
			}
			if cfg.DatabaseURL != tc.wantDatabaseURL {
				t.Errorf("Load().DatabaseURL = %q, want %q", cfg.DatabaseURL, tc.wantDatabaseURL)
			}
			if cfg.EbirdAPIKey != tc.wantEbirdAPIKey {
				t.Errorf("Load().EbirdAPIKey = %q, want %q", cfg.EbirdAPIKey, tc.wantEbirdAPIKey)
			}
			if cfg.OSRMURL != tc.wantOSRMURL {
				t.Errorf("Load().OSRMURL = %q, want %q", cfg.OSRMURL, tc.wantOSRMURL)
			}
			if !slices.Equal(cfg.Regions, tc.wantRegions) {
				t.Errorf("Load().Regions = %v, want %v", cfg.Regions, tc.wantRegions)
			}
		})
	}
}
