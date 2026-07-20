package config

import (
	"os"
	"testing"
)

// requiredEnv are environment variables Load() needs; each test case below
// unsets one of them to check it's actually required.
var requiredEnv = map[string]string{
	"AUTH_TOKEN":              "some-token",
	"GOOGLE_CREDENTIALS_FILE": "/path/to/credentials.json",
	"CALENDAR_ID":             "reference@group.calendar.google.com",
	"TZ":                      "Europe/Madrid",
}

func setEnv(t *testing.T, overrides map[string]string) {
	t.Helper()
	for k, v := range requiredEnv {
		if override, ok := overrides[k]; ok {
			if override == "" {
				t.Setenv(k, "")
				continue
			}
			t.Setenv(k, override)
			continue
		}
		t.Setenv(k, v)
	}
	t.Setenv("PORT", "")
}

func TestLoadRequiresEachEnvVar(t *testing.T) {
	for name := range requiredEnv {
		t.Run(name, func(t *testing.T) {
			setEnv(t, map[string]string{name: ""})
			if _, err := Load(); err == nil {
				t.Fatalf("Load() expected an error when %s is missing", name)
			}
		})
	}
}

func TestLoadRejectsInvalidTZ(t *testing.T) {
	setEnv(t, map[string]string{"TZ": "Not/AZone"})
	if _, err := Load(); err == nil {
		t.Fatal("Load() expected an error for an invalid TZ")
	}
}

func TestLoadSucceeds(t *testing.T) {
	setEnv(t, nil)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Port != "8080" {
		t.Fatalf("Port = %q, want default 8080", cfg.Port)
	}
	if cfg.Location.String() != "Europe/Madrid" {
		t.Fatalf("Location = %q, want Europe/Madrid", cfg.Location.String())
	}
}

func TestLoadUsesExplicitPort(t *testing.T) {
	setEnv(t, nil)
	os.Unsetenv("PORT")
	t.Setenv("PORT", "9090")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Port != "9090" {
		t.Fatalf("Port = %q, want 9090", cfg.Port)
	}
}
