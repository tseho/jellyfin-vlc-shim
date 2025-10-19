package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

type AuthRequest struct {
	Username string `json:"Username"`
	Pw       string `json:"Pw"`
}

type AuthenticationResult struct {
	User        User   `json:"User"`
	AccessToken string `json:"AccessToken"`
	ServerId    string `json:"ServerId"`
}

type User struct {
	Name string `json:"Name"`
	Id   string `json:"Id"`
}

var rootCmd = &cobra.Command{
	Use:   "jellyfin-vlc-shim",
	Short: "Jellyfin VLC Shim - Control VLC from Jellyfin",
	Long:  "A shim that allows Jellyfin to control VLC media player for video playback",
}

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Authenticate with Jellyfin server",
	Long:  "Authenticate with a Jellyfin server using username and password to obtain an access token",
	RunE: func(cmd *cobra.Command, args []string) error {
		return authenticate()
	},
}

func init() {
	rootCmd.AddCommand(authCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func authenticate() error {
	reader := bufio.NewReader(os.Stdin)

	// Prompt for Jellyfin URL
	fmt.Print("Jellyfin URL: ")
	url, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read URL: %w", err)
	}
	url = strings.TrimSpace(url)
	// Remove trailing slash if present
	url = strings.TrimSuffix(url, "/")

	// Prompt for username
	fmt.Print("Username: ")
	username, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read username: %w", err)
	}
	username = strings.TrimSpace(username)

	// Prompt for password (hidden)
	fmt.Print("Password: ")
	passwordBytes, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		return fmt.Errorf("failed to read password: %w", err)
	}
	fmt.Println() // New line after password input
	password := string(passwordBytes)

	// Authenticate with Jellyfin
	token, err := authenticateWithJellyfin(url, username, password)
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	fmt.Printf("\nAuthentication successful!\nAccess Token: %s\n", token)
	return nil
}

func authenticateWithJellyfin(baseURL, username, password string) (string, error) {
	authReq := AuthRequest{
		Username: username,
		Pw:       password,
	}

	jsonData, err := json.Marshal(authReq)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	endpoint := fmt.Sprintf("%s/Users/AuthenticateByName", baseURL)
	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	// @todo: fix DeviceId
	req.Header.Set("X-Emby-Authorization", `MediaBrowser Client="jellyfin-vlc-shim", Device="TV", DeviceId="jellyfin-vlc-shim", Version="0.0.1"`)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("authentication failed with status %d: %s", resp.StatusCode, string(body))
	}

	var authResult AuthenticationResult
	if err := json.Unmarshal(body, &authResult); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	return authResult.AccessToken, nil
}
