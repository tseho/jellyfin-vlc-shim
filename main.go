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

	vlc "github.com/adrg/libvlc-go/v3"
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
	ServerURL   string `json:"server_url"`
	AccessToken string `json:"access_token"`
	UserID      string `json:"user_id"`
	Username    string `json:"username"`
}

type Configuration struct {
	DeviceID       string `json:"device_id"`
	JellyfinClient string `json:"jellyfin_client"`
	JellyfinDevice string `json:"jellyfin_device"`
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

type PlayCommandData struct {
	ControllingUserId string   `json:"ControllingUserId"`
	ItemIds           []string `json:"ItemIds"`
	PlayCommand       string   `json:"PlayCommand"`
}

type ItemInfo struct {
	Id           string        `json:"Id"`
	Name         string        `json:"Name"`
	Type         string        `json:"Type"`
	MediaSources []MediaSource `json:"MediaSources"`
}

type MediaSource struct {
	Id                 string `json:"Id"`
	Protocol           string `json:"Protocol"`
	SupportsDirectPlay bool   `json:"SupportsDirectPlay"`
}

var (
	configDir  string
	serverURL  string
	username   string
	password   string
	deviceName string
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

var playCmd = &cobra.Command{
	Use:   "play [path]",
	Short: "Play a local video file using VLC",
	Long:  "Play a local video file using libVLC. The file path must be an absolute or relative path to a video file.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return playVideo(args[0])
	},
}

func init() {
	rootCmd.PersistentFlags().StringVar(&configDir, "config", "", "Configuration directory (default: ~/.config/jellyfin-vlc-shim)")

	authCmd.Flags().StringVar(&serverURL, "url", "", "Jellyfin server URL")
	authCmd.Flags().StringVar(&username, "username", "", "Jellyfin username")
	authCmd.Flags().StringVar(&password, "password", "", "Jellyfin password")
	authCmd.Flags().StringVar(&deviceName, "device-name", "", "Device name for Jellyfin (default: hostname)")

	rootCmd.AddCommand(authCmd)
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(playCmd)
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

	// Load or initialize configuration
	config, err := loadConfiguration()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Update device name if provided
	if deviceName != "" {
		config.JellyfinDevice = deviceName
		if err := saveConfiguration(config); err != nil {
			return fmt.Errorf("failed to save configuration with device name: %w", err)
		}
	}

	// Authenticate with Jellyfin
	authResult, err := authenticateWithJellyfin(inputURL, inputUsername, inputPassword, config)
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	// Save credentials
	creds := Credentials{
		ServerURL:   inputURL,
		AccessToken: authResult.AccessToken,
		UserID:      authResult.User.Id,
		Username:    authResult.User.Name,
	}

	if err := saveCredentials(creds); err != nil {
		return fmt.Errorf("failed to save credentials: %w", err)
	}

	log.Printf("Authentication successful")
	log.Printf("Credentials saved to credentials.json")
	return nil
}

func authenticateWithJellyfin(baseURL, username, password string, config *Configuration) (*AuthenticationResult, error) {
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
	req.Header.Set("X-Emby-Authorization", fmt.Sprintf(`MediaBrowser Client="%s", Device="%s", DeviceId="%s", Version="0.0.1"`, config.JellyfinClient, config.JellyfinDevice, config.DeviceID))

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

func loadConfiguration() (*Configuration, error) {
	dir, err := getConfigDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get config directory: %w", err)
	}

	configPath := filepath.Join(dir, "configuration.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Initialize with default values if file doesn't exist
			hostname, err := os.Hostname()
			if err != nil {
				hostname = "Unknown" // Fallback if hostname cannot be determined
			}

			config := &Configuration{
				DeviceID:       generateDeviceID(),
				JellyfinClient: "jellyfin-vlc-shim",
				JellyfinDevice: hostname,
			}
			if saveErr := saveConfiguration(config); saveErr != nil {
				return nil, fmt.Errorf("failed to initialize configuration: %w", saveErr)
			}
			return config, nil
		}
		return nil, fmt.Errorf("failed to read configuration file: %w", err)
	}

	var config Configuration
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse configuration: %w", err)
	}

	return &config, nil
}

func saveConfiguration(config *Configuration) error {
	dir, err := getConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get config directory: %w", err)
	}

	// Create config directory if it doesn't exist
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Marshal configuration to JSON
	jsonData, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal configuration: %w", err)
	}

	// Write to file
	configPath := filepath.Join(dir, "configuration.json")
	if err := os.WriteFile(configPath, jsonData, 0600); err != nil {
		return fmt.Errorf("failed to write configuration file: %w", err)
	}

	return nil
}

func generateDeviceID() string {
	return uuid.New().String()
}

func makeAuthHeader(config *Configuration, token string) string {
	return fmt.Sprintf(
		`MediaBrowser Client="%s", Device="%s", DeviceId="%s", Version="0.0.1", Token="%s"`,
		config.JellyfinClient, config.JellyfinDevice, config.DeviceID, token,
	)
}

func getItemInfo(serverURL, itemID, userID string, config *Configuration, token string) (*ItemInfo, error) {
	endpoint := fmt.Sprintf("%s/Users/%s/Items/%s", serverURL, userID, itemID)

	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", makeAuthHeader(config, token))

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
		return nil, fmt.Errorf("failed to get item info with status %d: %s", resp.StatusCode, string(body))
	}

	var itemInfo ItemInfo
	if err := json.Unmarshal(body, &itemInfo); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &itemInfo, nil
}

func handlePlayCommand(playData PlayCommandData, creds *Credentials, config *Configuration) error {
	if len(playData.ItemIds) == 0 {
		return fmt.Errorf("no items to play")
	}

	// For now, just play the first item
	itemID := playData.ItemIds[0]

	log.Printf("Fetching info for item: %s", itemID)
	itemInfo, err := getItemInfo(creds.ServerURL, itemID, creds.UserID, config, creds.AccessToken)
	if err != nil {
		return fmt.Errorf("failed to get item info: %w", err)
	}

	log.Printf("Playing: %s (Type: %s)", itemInfo.Name, itemInfo.Type)

	// Get the direct stream URL
	streamURL := getDirectStreamURL(creds.ServerURL, itemID, config, creds.AccessToken)
	log.Printf("Stream URL: %s", streamURL)

	// Play the video using VLC
	if err := playVideoURL(streamURL); err != nil {
		return fmt.Errorf("failed to play video: %w", err)
	}

	return nil
}

func getDirectStreamURL(serverURL, itemID string, config *Configuration, token string) string {
	// Construct direct stream URL
	// Format: /Videos/{itemId}/stream?static=true&DeviceId={deviceId}&api_key={token}
	return fmt.Sprintf("%s/Videos/%s/stream?static=true&DeviceId=%s&api_key=%s",
		serverURL, itemID, config.DeviceID, token)
}

func registerCapabilities(serverURL string, config *Configuration, token string) error {
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
	req.Header.Set("Authorization", makeAuthHeader(config, token))

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

func connectWebSocket(creds *Credentials, config *Configuration) error {
	// Convert HTTP(S) URL to WS(S)
	u, err := url.Parse(creds.ServerURL)
	if err != nil {
		return fmt.Errorf("failed to parse server URL: %w", err)
	}

	wsScheme := "ws"
	if u.Scheme == "https" {
		wsScheme = "wss"
	}

	wsURL := fmt.Sprintf("%s://%s/socket?api_key=%s&device_id=%s", wsScheme, u.Host, creds.AccessToken, config.DeviceID)

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

			// Handle different message types
			switch msg.MessageType {
			case "ForceKeepAlive":
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

			case "Play":
				log.Println("Received Play command")

				// Parse the play command data
				dataJSON, err := json.Marshal(msg.Data)
				if err != nil {
					log.Printf("Error marshaling play data: %v", err)
					continue
				}

				var playData PlayCommandData
				if err := json.Unmarshal(dataJSON, &playData); err != nil {
					log.Printf("Error parsing play command: %v", err)
					continue
				}

				// Handle the play command in a separate goroutine to not block message processing
				go func() {
					if err := handlePlayCommand(playData, creds, config); err != nil {
						log.Printf("Error handling play command: %v", err)
					}
				}()
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
	// Load configuration
	config, err := loadConfiguration()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Load credentials
	creds, err := loadCredentials()
	if err != nil {
		return fmt.Errorf("failed to load credentials: %w\nPlease run 'jellyfin-vlc-shim auth' first", err)
	}

	log.Printf("Starting Jellyfin VLC Shim client")
	log.Printf("Server: %s", creds.ServerURL)
	log.Printf("User: %s", creds.Username)
	log.Printf("Device ID: %s", config.DeviceID)

	// Register capabilities
	log.Println("Registering capabilities with Jellyfin server...")
	if err := registerCapabilities(creds.ServerURL, config, creds.AccessToken); err != nil {
		return fmt.Errorf("failed to register capabilities: %w", err)
	}
	log.Println("Capabilities registered successfully!")

	// Connect to WebSocket
	if err := connectWebSocket(creds, config); err != nil {
		return fmt.Errorf("WebSocket error: %w", err)
	}

	return nil
}

func playVideo(path string) error {
	// Initialize libVLC with minimal flags to avoid video output issues
	// "-vv" to enable verbose logging
	if err := vlc.Init("--no-xlib"); err != nil {
		return fmt.Errorf("failed to initialize libVLC: %w", err)
	}
	defer vlc.Release()

	// Create a new player
	player, err := vlc.NewPlayer()
	if err != nil {
		return fmt.Errorf("failed to create player: %w", err)
	}
	defer func() {
		player.Stop()
		player.Release()
	}()

	// Convert to absolute path if relative
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to resolve path: %w", err)
	}

	// Check if file exists
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return fmt.Errorf("file does not exist: %s", absPath)
	}

	log.Printf("Loading media: %s", absPath)

	// Load media from path
	media, err := player.LoadMediaFromPath(absPath)
	if err != nil {
		return fmt.Errorf("failed to load media: %w", err)
	}
	defer media.Release()

	// Get event manager
	manager, err := player.EventManager()
	if err != nil {
		return fmt.Errorf("failed to get event manager: %w", err)
	}

	// Channel to signal when playback ends
	done := make(chan struct{})

	// Register end reached event
	eventCallback := func(event vlc.Event, userData interface{}) {
		log.Println("Playback finished")
		close(done)
	}

	eventID, err := manager.Attach(vlc.MediaPlayerEndReached, eventCallback, nil)
	if err != nil {
		return fmt.Errorf("failed to attach event: %w", err)
	}
	defer manager.Detach(eventID)

	// Start playing
	log.Println("Starting playback...")
	if err := player.Play(); err != nil {
		return fmt.Errorf("failed to start playback: %w", err)
	}

	// Handle Ctrl+C to stop playback gracefully
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)

	log.Println("Playing... Press Ctrl+C to stop")

	// Wait for playback to finish or interrupt
	select {
	case <-done:
		log.Println("Video playback completed")
	case <-interrupt:
		log.Println("Stopping playback...")
		player.Stop()
	}

	return nil
}

func playVideoURL(mediaURL string) error {
	// Initialize libVLC with fullscreen flag
	if err := vlc.Init("--fullscreen"); err != nil {
		return fmt.Errorf("failed to initialize libVLC: %w", err)
	}
	defer vlc.Release()

	// Create a new player
	player, err := vlc.NewPlayer()
	if err != nil {
		return fmt.Errorf("failed to create player: %w", err)
	}
	defer func() {
		player.Stop()
		player.Release()
	}()

	log.Printf("Loading media from URL: %s", mediaURL)

	// Load media from URL
	media, err := player.LoadMediaFromURL(mediaURL)
	if err != nil {
		return fmt.Errorf("failed to load media: %w", err)
	}
	defer media.Release()

	// Get event manager
	manager, err := player.EventManager()
	if err != nil {
		return fmt.Errorf("failed to get event manager: %w", err)
	}

	// Channel to signal when playback ends
	done := make(chan struct{})

	// Register end reached event
	eventCallback := func(event vlc.Event, userData interface{}) {
		log.Println("Playback finished")
		close(done)
	}

	eventID, err := manager.Attach(vlc.MediaPlayerEndReached, eventCallback, nil)
	if err != nil {
		return fmt.Errorf("failed to attach event: %w", err)
	}
	defer manager.Detach(eventID)

	// Start playing
	log.Println("Starting playback...")
	if err := player.Play(); err != nil {
		return fmt.Errorf("failed to start playback: %w", err)
	}

	// Set fullscreen mode
	player.SetFullScreen(true)

	// Handle Ctrl+C to stop playback gracefully
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)

	log.Println("Playing in fullscreen... Press Ctrl+C to stop")

	// Wait for playback to finish or interrupt
	select {
	case <-done:
		log.Println("Video playback completed")
	case <-interrupt:
		log.Println("Stopping playback...")
		player.Stop()
	}

	return nil
}
