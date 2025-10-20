package commands

import (
	"bufio"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"syscall"

	"jellyfin-vlc-shim/config"
	"jellyfin-vlc-shim/jellyfin"
	"jellyfin-vlc-shim/logger"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var (
	serverURL  string
	username   string
	password   string
	deviceName string
)

func NewAuthCmd(configDir *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Authenticate on a Jellyfin server",
		Long:  "Authenticate on a Jellyfin server using username and password to obtain an access token",
		RunE: func(cmd *cobra.Command, args []string) error {
			return authenticate(*configDir)
		},
	}

	cmd.Flags().StringVar(&serverURL, "url", "", "Jellyfin server URL")
	cmd.Flags().StringVar(&username, "username", "", "Jellyfin username")
	cmd.Flags().StringVar(&password, "password", "", "Jellyfin password")
	cmd.Flags().StringVar(&deviceName, "device-name", "", "Device name for Jellyfin (default: hostname)")

	return cmd
}

func authenticate(configDir string) error {
	var err error
	var inputURL, inputUsername, inputPassword string

	// Use flags if provided, otherwise prompt interactively
	if serverURL != "" {
		inputURL = serverURL
	} else {
		reader := bufio.NewReader(os.Stdin)
		fmt.Print("Jellyfin URL: ")
		inputURL, err = reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read URL: %w", err)
		}
		inputURL = strings.TrimSpace(inputURL)
	}
	// Remove trailing slash if present
	inputURL = strings.TrimSuffix(inputURL, "/")

	if username != "" {
		inputUsername = username
	} else {
		reader := bufio.NewReader(os.Stdin)
		fmt.Print("Username: ")
		inputUsername, err = reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read username: %w", err)
		}
		inputUsername = strings.TrimSpace(inputUsername)
	}

	if password != "" {
		inputPassword = password
	} else {
		fmt.Print("Password: ")
		passwordBytes, err := term.ReadPassword(int(syscall.Stdin))
		if err != nil {
			return fmt.Errorf("failed to read password: %w", err)
		}
		fmt.Println() // New line after password input
		inputPassword = string(passwordBytes)
	}

	// Get config directory
	dir, err := config.GetConfigDir(configDir)
	if err != nil {
		return fmt.Errorf("failed to get config directory: %w", err)
	}

	// Load or initialize configuration
	cfg, err := config.Load(dir)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Initialize logger with configured log level
	logger.Initialize(cfg.LogLevel)

	// Update device name if provided
	if deviceName != "" {
		cfg.JellyfinDevice = deviceName
		if err := config.Save(dir, cfg); err != nil {
			return fmt.Errorf("failed to save configuration with device name: %w", err)
		}
	}

	// Authenticate with Jellyfin
	authResult, err := jellyfin.Authenticate(inputURL, inputUsername, inputPassword, cfg.DeviceID, cfg.JellyfinClient, cfg.JellyfinDevice)
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	// Save credentials
	creds := &config.Credentials{
		ServerURL:   inputURL,
		AccessToken: authResult.AccessToken,
		UserID:      authResult.User.Id,
		Username:    authResult.User.Name,
	}

	if err := config.SaveCredentials(dir, creds); err != nil {
		return fmt.Errorf("failed to save credentials: %w", err)
	}

	slog.Info("Authentication successful")
	slog.Info("Credentials saved to credentials.json")
	return nil
}
