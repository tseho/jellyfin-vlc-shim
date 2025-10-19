package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
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

type Credentials struct {
	ServerURL   string `json:"serverUrl"`
	AccessToken string `json:"accessToken"`
	UserID      string `json:"userId"`
	Username    string `json:"username"`
	DeviceID    string `json:"deviceId"`
}

type Capabilities struct {
	PlayableMediaTypes   string `json:"PlayableMediaTypes"`
	SupportsMediaControl bool   `json:"SupportsMediaControl"`
	SupportedCommands    string `json:"SupportedCommands"`
}

type WebSocketMessage struct {
	MessageType string      `json:"MessageType"`
	Data        interface{} `json:"Data,omitempty"`
}

var (
	configDir string
	serverURL string
	username  string
	password  string
)

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

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the Jellyfin VLC Shim client",
	Long:  "Start the client, register with Jellyfin server, and listen for playback commands",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runClient()
	},
}

func init() {
	rootCmd.PersistentFlags().StringVar(&configDir, "config", "", "Configuration directory (default: ~/.config/jellyfin-vlc-shim)")

	authCmd.Flags().StringVar(&serverURL, "url", "", "Jellyfin server URL")
	authCmd.Flags().StringVar(&username, "username", "", "Jellyfin username")
	authCmd.Flags().StringVar(&password, "password", "", "Jellyfin password")

	rootCmd.AddCommand(authCmd)
	rootCmd.AddCommand(runCmd)
}

func getConfigDir() (string, error) {
	if configDir != "" {
		return configDir, nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}

	return filepath.Join(homeDir, ".config", "jellyfin-vlc-shim"), nil
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func authenticate() error {
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

	// Should we load the existing one if exists?
	deviceID := generateDeviceID()

	// Authenticate with Jellyfin
	authResult, err := authenticateWithJellyfin(inputURL, inputUsername, inputPassword, deviceID)
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	// Save credentials
	creds := Credentials{
		ServerURL:   inputURL,
		AccessToken: authResult.AccessToken,
		UserID:      authResult.User.Id,
		Username:    authResult.User.Name,
		DeviceID:    deviceID,
	}

	if err := saveCredentials(creds); err != nil {
		return fmt.Errorf("failed to save credentials: %w", err)
	}

	log.Printf("Authentication successful")
	log.Printf("Credentials saved to credentials.json")
	return nil
}

func authenticateWithJellyfin(baseURL, username, password string, deviceID string) (*AuthenticationResult, error) {
	authReq := AuthRequest{
		Username: username,
		Pw:       password,
	}

	jsonData, err := json.Marshal(authReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	endpoint := fmt.Sprintf("%s/Users/AuthenticateByName", baseURL)
	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Emby-Authorization", fmt.Sprintf(`MediaBrowser Client="jellyfin-vlc-shim", Device="TV", DeviceId="%s", Version="0.0.1"`, deviceID))

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("authentication failed with status %d: %s", resp.StatusCode, string(body))
	}

	var authResult AuthenticationResult
	if err := json.Unmarshal(body, &authResult); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &authResult, nil
}

func saveCredentials(creds Credentials) error {
	dir, err := getConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get config directory: %w", err)
	}

	// Create config directory if it doesn't exist
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Marshal credentials to JSON
	jsonData, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal credentials: %w", err)
	}

	// Write to file
	credPath := filepath.Join(dir, "credentials.json")
	if err := os.WriteFile(credPath, jsonData, 0600); err != nil {
		return fmt.Errorf("failed to write credentials file: %w", err)
	}

	return nil
}

func loadCredentials() (*Credentials, error) {
	dir, err := getConfigDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get config directory: %w", err)
	}

	credPath := filepath.Join(dir, "credentials.json")
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

func generateDeviceID() string {
	return uuid.New().String()
}

func makeAuthHeader(deviceID, token string) string {
	return fmt.Sprintf(
		`MediaBrowser Client="jellyfin-vlc-shim", Device="TV", DeviceId="%s", Version="0.0.1", Token="%s"`,
		deviceID, token,
	)
}

func registerCapabilities(serverURL, deviceID, token string) error {
	caps := Capabilities{
		PlayableMediaTypes:   "Video",
		SupportsMediaControl: true,
		SupportedCommands: strings.Join([]string{
			"Play",
			"Playstate",
			"PlayNext",
			"PlayMediaSource",
			"SetVolume",
			"SetAudioStreamIndex",
			"SetSubtitleStreamIndex",
			"Stop",
			"Seek",
		}, ","),
	}

	jsonData, err := json.Marshal(caps)
	if err != nil {
		return fmt.Errorf("failed to marshal capabilities: %w", err)
	}

	endpoint := fmt.Sprintf("%s/Sessions/Capabilities/Full", serverURL)
	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", makeAuthHeader(deviceID, token))

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to register capabilities with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

func connectWebSocket(serverURL, deviceID, token string) error {
	// Convert HTTP(S) URL to WS(S)
	u, err := url.Parse(serverURL)
	if err != nil {
		return fmt.Errorf("failed to parse server URL: %w", err)
	}

	wsScheme := "ws"
	if u.Scheme == "https" {
		wsScheme = "wss"
	}

	wsURL := fmt.Sprintf("%s://%s/socket?api_key=%s&device_id=%s", wsScheme, u.Host, token, deviceID)

	log.Printf("Connecting to WebSocket: %s", wsURL)

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to WebSocket: %w", err)
	}
	defer conn.Close()

	log.Println("WebSocket connected successfully!")
	log.Println("Listening for messages from Jellyfin server...")
	log.Println("Press Ctrl+C to stop")

	// Handle graceful shutdown
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)

	done := make(chan struct{})

	// Read messages
	go func() {
		defer close(done)
		for {
			var msg WebSocketMessage
			err := conn.ReadJSON(&msg)
			if err != nil {
				log.Printf("Error reading message: %v", err)
				return
			}

			// Pretty print the message
			jsonData, err := json.MarshalIndent(msg, "", "  ")
			if err != nil {
				log.Printf("Error marshaling message: %v", err)
				continue
			}

			log.Printf("Received message:\n%s\n", string(jsonData))

			// Handle keep-alive
			if msg.MessageType == "ForceKeepAlive" {
				log.Println("Sending KeepAlive response...")
				keepAlive := map[string]string{
					"MessageType": "KeepAlive",
				}
				if err := conn.WriteJSON(keepAlive); err != nil {
					log.Printf("Error sending keep-alive: %v", err)
					return
				}

				// Start periodic keep-alive
				if dataMap, ok := msg.Data.(map[string]interface{}); ok {
					if timeoutSec, ok := dataMap["Timeout"].(float64); ok {
						go func() {
							ticker := time.NewTicker(time.Duration(timeoutSec/2) * time.Second)
							defer ticker.Stop()

							for {
								select {
								case <-ticker.C:
									keepAlive := map[string]string{
										"MessageType": "KeepAlive",
									}
									if err := conn.WriteJSON(keepAlive); err != nil {
										log.Printf("Error sending periodic keep-alive: %v", err)
										return
									}
									log.Println("Sent periodic KeepAlive")
								case <-done:
									return
								}
							}
						}()
					}
				}
			}
		}
	}()

	// Wait for interrupt signal
	select {
	case <-interrupt:
		log.Printf("Shutting down...")
		err := conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		if err != nil {
			log.Printf("Error sending close message: %v", err)
		}
		select {
		case <-done:
		case <-time.After(time.Second):
		}
	case <-done:
	}

	return nil
}

func runClient() error {
	// Load credentials
	creds, err := loadCredentials()
	if err != nil {
		return fmt.Errorf("failed to load credentials: %w\nPlease run 'jellyfin-vlc-shim auth' first", err)
	}

	log.Printf("Starting Jellyfin VLC Shim client")
	log.Printf("Server: %s", creds.ServerURL)
	log.Printf("User: %s", creds.Username)
	log.Printf("Device ID: %s", creds.DeviceID)

	// Register capabilities
	log.Println("Registering capabilities with Jellyfin server...")
	if err := registerCapabilities(creds.ServerURL, creds.DeviceID, creds.AccessToken); err != nil {
		return fmt.Errorf("failed to register capabilities: %w", err)
	}
	log.Println("Capabilities registered successfully!")

	// Connect to WebSocket
	if err := connectWebSocket(creds.ServerURL, creds.DeviceID, creds.AccessToken); err != nil {
		return fmt.Errorf("WebSocket error: %w", err)
	}

	return nil
}
