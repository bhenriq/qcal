package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type SourceConfig struct {
	Name  string `json:"name"`
	Type  string `json:"type"`            // "google" or "caldav"
	Color string `json:"color,omitempty"` // optional color override
	// Google fields
	ClientID     string `json:"client_id,omitempty"`
	ClientSecret string `json:"client_secret,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	TokenURI     string `json:"token_uri,omitempty"`
	// CalDAV fields
	URL      string `json:"url,omitempty"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

type Config struct {
	Sources []SourceConfig `json:"sources"`
}

func loadConfig() (*Config, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	configPath := filepath.Join(filepath.Dir(exe), "config.json")

	// Fall back to current directory if not found next to binary
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		configPath = "config.json"
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	if len(cfg.Sources) == 0 {
		return nil, fmt.Errorf("no sources defined in config")
	}

	return &cfg, nil
}
