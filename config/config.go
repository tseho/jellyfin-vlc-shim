package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/uuid"
)

// Config holds the Jellyfin client configuration
type Config struct {
	DeviceID       string `json:"device_id"`
	JellyfinClient string `json:"jellyfin_client"`
	JellyfinDevice string `json:"jellyfin_device"`
	Fullscreen     bool   `json:"fullscreen"`
}

// Credentials holds the authentication information
type Credentials struct {
	ServerURL   string `json:"server_url"`
	AccessToken string `json:"access_token"`
	UserID      string `json:"user_id"`
	Username    string `json:"username"`
}

// Load loads the configuration from the config directory
func Load(configDir string) (*Config, error) {
	configPath := filepath.Join(configDir, "configuration.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Initialize with default values if file doesn't exist
			hostname, err := os.Hostname()
			if err != nil {
				hostname = "Unknown" // Fallback if hostname cannot be determined
			}

			cfg := &Config{
				DeviceID:       uuid.New().String(),
				JellyfinClient: "jellyfin-vlc-shim",
				JellyfinDevice: hostname,
				Fullscreen:     true,
			}
			if saveErr := Save(configDir, cfg); saveErr != nil {
				return nil, fmt.Errorf("failed to initialize configuration: %w", saveErr)
			}
			return cfg, nil
		}
		return nil, fmt.Errorf("failed to read configuration file: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse configuration: %w", err)
	}

	return &cfg, nil
}

// Save saves the configuration to the config directory
func Save(configDir string, cfg *Config) error {
	// Create config directory if it doesn't exist
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Marshal configuration to JSON
	jsonData, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal configuration: %w", err)
	}

	// Write to file
	configPath := filepath.Join(configDir, "configuration.json")
	if err := os.WriteFile(configPath, jsonData, 0600); err != nil {
		return fmt.Errorf("failed to write configuration file: %w", err)
	}

	return nil
}

// LoadCredentials loads the credentials from the config directory
func LoadCredentials(configDir string) (*Credentials, error) {
	credPath := filepath.Join(configDir, "credentials.json")
	data, err := os.ReadFile(credPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read credentials file: %w", err)
	}

	var creds Credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("failed to parse credentials: %w", err)
	}

	return &creds, nil
}

// SaveCredentials saves the credentials to the config directory
func SaveCredentials(configDir string, creds *Credentials) error {
	// Create config directory if it doesn't exist
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Marshal credentials to JSON
	jsonData, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal credentials: %w", err)
	}

	// Write to file
	credPath := filepath.Join(configDir, "credentials.json")
	if err := os.WriteFile(credPath, jsonData, 0600); err != nil {
		return fmt.Errorf("failed to write credentials file: %w", err)
	}

	return nil
}

// GetConfigDir returns the configuration directory path
func GetConfigDir(customDir string) (string, error) {
	if customDir != "" {
		return customDir, nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}

	return filepath.Join(homeDir, ".config", "jellyfin-vlc-shim"), nil
}
