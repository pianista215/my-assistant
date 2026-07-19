// Package config loads runtime configuration from the environment.
package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	// AuthToken is the static bearer token the ESP32 must send to be
	// served display data.
	AuthToken string
	Port      string
}

// Load reads configuration from the environment, loading a local .env
// file first if present (development only; in production the VPS sets
// real environment variables and no .env file exists).
func Load() (*Config, error) {
	_ = godotenv.Load()

	token := os.Getenv("AUTH_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("config: AUTH_TOKEN environment variable is required")
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	return &Config{AuthToken: token, Port: port}, nil
}
