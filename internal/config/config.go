package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type Config struct {
	Mode         string          `json:"mode"`
	Port         int             `json:"port"`
	Domain       string          `json:"domain"`
	CookieDomain string          `json:"cookie_domain"`
	DataDir      string          `json:"data_dir"`
	Hostname     string          `json:"hostname"`
	JWTSecret    string          `json:"-"`
	AllowSignups   *bool           `json:"allow_signups"`
	APIKeys        []string        `json:"api_keys"`
	APIKeyFiles    []string        `json:"api_key_files"`
	AuthorizedKeys []string        `json:"authorized_keys"`
	Services     []ServiceConfig `json:"services"`
	Hosts        []HostConfig    `json:"hosts"`
	Jellyfin     *JellyfinConfig `json:"jellyfin"`
	Minecraft    *MinecraftConfig `json:"minecraft"`
	TemperatureCommand string     `json:"temperature_command"`
	// VMControllerSocket is the Unix socket path for foyer-vm-controller.
	// Empty string means the VM feature is disabled.
	VMControllerSocket string `json:"vm_controller_socket"`
}

type MinecraftConfig struct {
	Address string `json:"address"` // host:port, e.g. "localhost:25565"
}

type ServiceConfig struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type HostConfig struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	APIKey string `json:"api_key"`
}

type JellyfinConfig struct {
	URL        string `json:"url"`
	APIKey     string `json:"-"`
	APIKeyFile string `json:"api_key_file"`
}

func Load(path string, jwtSecretFile string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	// Defaults
	if cfg.Mode == "" {
		cfg.Mode = "full"
	}
	if cfg.Port == 0 {
		cfg.Port = 8420
	}
	if cfg.DataDir == "" {
		cfg.DataDir = "/var/lib/foyer"
	}

	// Validate mode
	if cfg.Mode != "full" && cfg.Mode != "api-only" {
		return nil, fmt.Errorf("invalid mode %q: must be \"full\" or \"api-only\"", cfg.Mode)
	}

	// Load JWT secret from file
	if jwtSecretFile != "" {
		secret, err := readSecretFile(jwtSecretFile)
		if err != nil {
			return nil, fmt.Errorf("read JWT secret: %w", err)
		}
		cfg.JWTSecret = secret
	}

	// Validate JWT secret for full mode
	if cfg.Mode == "full" && len(cfg.JWTSecret) < 32 {
		return nil, fmt.Errorf("JWT secret must be at least 32 bytes for full mode (got %d)", len(cfg.JWTSecret))
	}

	// Load API keys from files
	for _, keyFile := range cfg.APIKeyFiles {
		key, err := readSecretFile(keyFile)
		if err != nil {
			return nil, fmt.Errorf("read API key file %s: %w", keyFile, err)
		}
		cfg.APIKeys = append(cfg.APIKeys, key)
	}

	// Disable Jellyfin if no API key configured
	if cfg.Jellyfin != nil && cfg.Jellyfin.APIKeyFile == "" {
		cfg.Jellyfin = nil
	}

	// Load Jellyfin API key from file
	if cfg.Jellyfin != nil && cfg.Jellyfin.APIKeyFile != "" {
		key, err := readSecretFile(cfg.Jellyfin.APIKeyFile)
		if err != nil {
			return nil, fmt.Errorf("read Jellyfin API key: %w", err)
		}
		cfg.Jellyfin.APIKey = key
	}

	return &cfg, nil
}

func (c *Config) SignupsAllowed() bool {
	if c.AllowSignups == nil {
		return true // on by default
	}
	return *c.AllowSignups
}

func readSecretFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}
