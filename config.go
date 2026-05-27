package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	configDir := os.Getenv("XDG_CONFIG_HOME")
	if configDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		configDir = filepath.Join(home, ".config")
	}

	xdgPath := filepath.Join(configDir, "qcal", "config.json")
	cwdPath := "config.json"

	pathsTried := []string{xdgPath}

	data, err := os.ReadFile(xdgPath)
	if err != nil {
		data, err = os.ReadFile(cwdPath)
		pathsTried = append(pathsTried, cwdPath)
	}
	if err != nil {
		return nil, fmt.Errorf("config.json not found\n  (tried: %s)", strings.Join(pathsTried, ", "))
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
