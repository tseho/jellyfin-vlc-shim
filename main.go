package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"

	"jellyfin-vlc-shim/config"
	"jellyfin-vlc-shim/jellyfin"
	"jellyfin-vlc-shim/player"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var (
	configDir  string
	serverURL  string
	username   string
	password   string
	deviceName string
	// Global player state for handling commands
	activePlayer *player.Player
	activeItemID string
	playerLock   = &sync.Mutex{}
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

	log.Printf("Authentication successful")
	log.Printf("Credentials saved to credentials.json")
	return nil
}

func runClient() error {
	// Get config directory
	dir, err := config.GetConfigDir(configDir)
	if err != nil {
		return fmt.Errorf("failed to get config directory: %w", err)
	}

	// Load configuration
	cfg, err := config.Load(dir)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Load credentials
	creds, err := config.LoadCredentials(dir)
	if err != nil {
		return fmt.Errorf("failed to load credentials: %w\nPlease run 'jellyfin-vlc-shim auth' first", err)
	}

	log.Printf("Starting Jellyfin VLC Shim client")
	log.Printf("Server: %s", creds.ServerURL)
	log.Printf("User: %s", creds.Username)
	log.Printf("Device ID: %s", cfg.DeviceID)

	// Create Jellyfin client
	client := jellyfin.NewClient(creds.ServerURL, creds.AccessToken, creds.UserID, cfg.DeviceID, cfg.JellyfinClient, cfg.JellyfinDevice)

	// Register capabilities
	log.Println("Registering capabilities with Jellyfin server...")
	if err := client.RegisterCapabilities(); err != nil {
		return fmt.Errorf("failed to register capabilities: %w", err)
	}
	log.Println("Capabilities registered successfully!")

	// Connect to WebSocket and handle messages
	return client.ConnectWebSocket(func(msg jellyfin.WebSocketMessage) error {
		switch msg.MessageType {
		case "Play":
			log.Println("Received Play command")

			// Parse the play command data
			dataJSON, err := json.Marshal(msg.Data)
			if err != nil {
				return fmt.Errorf("error marshaling play data: %w", err)
			}

			var playData jellyfin.PlayCommandData
			if err := json.Unmarshal(dataJSON, &playData); err != nil {
				return fmt.Errorf("error parsing play command: %w", err)
			}

			// Handle the play command in a separate goroutine to not block message processing
			go func() {
				if err := handlePlayCommand(playData, client, cfg); err != nil {
					log.Printf("Error handling play command: %v", err)
				}
			}()

		case "Playstate":
			log.Println("Received Playstate command")

			// Parse the playstate command data
			dataJSON, err := json.Marshal(msg.Data)
			if err != nil {
				return fmt.Errorf("error marshaling playstate data: %w", err)
			}

			var playstateData jellyfin.PlaystateCommandData
			if err := json.Unmarshal(dataJSON, &playstateData); err != nil {
				return fmt.Errorf("error parsing playstate command: %w", err)
			}

			// Handle the playstate command
			if err := handlePlaystateCommand(playstateData, client); err != nil {
				log.Printf("Error handling playstate command: %v", err)
			}
		}

		return nil
	})
}

func handlePlayCommand(playData jellyfin.PlayCommandData, client *jellyfin.Client, cfg *config.Config) error {
	if len(playData.ItemIds) == 0 {
		return fmt.Errorf("no items to play")
	}

	// For now, just play the first item
	itemID := playData.ItemIds[0]

	log.Printf("Fetching info for item: %s", itemID)
	itemInfo, err := client.GetItemInfo(itemID)
	if err != nil {
		return fmt.Errorf("failed to get item info: %w", err)
	}

	log.Printf("Playing: %s (Type: %s)", itemInfo.Name, itemInfo.Type)

	// Get the direct stream URL
	streamURL := client.GetDirectStreamURL(itemID)
	log.Printf("Stream URL: %s", streamURL)

	// Play the video using VLC with progress reporting
	if err := playJellyfinVideo(streamURL, itemID, client, cfg); err != nil {
		return fmt.Errorf("failed to play video: %w", err)
	}

	return nil
}

func UpdatePlaybackStatus(client *jellyfin.Client, player *player.Player) {
	itemID := activeItemID
	state := player.GetState()
	positionMs := state.GetCurrentPositionMs()
	positionTicks := int64(positionMs) * 10000

	if err := client.ReportPlaybackProgress(itemID, positionTicks, state.IsPaused); err != nil {
		log.Printf("Warning: failed to report playback progress: %v", err)
	} else {
		log.Printf("Reported playback progress (paused: %v, position: %d ms)", state.IsPaused, positionMs)
	}
}

func handlePlaystateCommand(playstateData jellyfin.PlaystateCommandData, client *jellyfin.Client) error {
	playerLock.Lock()
	p := activePlayer
	playerLock.Unlock()

	if p == nil {
		return fmt.Errorf("no active player")
	}

	command := playstateData.Command
	log.Printf("Received Playstate command: %s", command)

	switch command {
	case "Pause":
		if err := p.Pause(); err != nil {
			return fmt.Errorf("failed to pause: %w", err)
		}
		UpdatePlaybackStatus(client, p)
	case "Unpause":
		if err := p.Unpause(); err != nil {
			return fmt.Errorf("failed to unpause: %w", err)
		}
		UpdatePlaybackStatus(client, p)
	case "PlayPause":
		if err := p.TogglePause(); err != nil {
			return fmt.Errorf("failed to toggle pause: %w", err)
		}
		UpdatePlaybackStatus(client, p)
	case "Stop":
		p.Stop()
	case "NextTrack":
		log.Println("NextTrack not yet implemented")
	case "PreviousTrack":
		log.Println("PreviousTrack not yet implemented")
	case "Seek":
		log.Println("Seek not yet implemented")
		// if playstateData.SeekPositionTicks > 0 {
		// 	// Convert ticks to milliseconds (1 tick = 100 nanoseconds)
		// 	seekTimeMs := playstateData.SeekPositionTicks / 10000
		// 	if err := p.Seek(seekTimeMs); err != nil {
		// 		return fmt.Errorf("failed to seek: %w", err)
		// 	}
		// } else {
		// 	shouldReport = false
		// }
	default:
		log.Printf("Unknown playstate command: %s", command)
	}

	return nil
}

func playVideo(path string) error {
	// Create player with minimal VLC flags for local playback
	p, err := player.New(&player.Options{
		VLCArgs: []string{"--no-xlib"},
	})
	if err != nil {
		return fmt.Errorf("failed to create player: %w", err)
	}
	defer p.Release()

	// Load media from path
	media, err := p.LoadMediaFromPath(path)
	if err != nil {
		return err
	}
	defer media.Release()

	// Setup end reached event
	done, err := p.ListenEndReachedEvent()
	if err != nil {
		return err
	}

	// Start playing
	if err := p.Play(); err != nil {
		return err
	}

	// Wait for playback to finish or interrupt
	p.WaitForEndOrInterrupt(done)

	return nil
}

func playJellyfinVideo(mediaURL, itemID string, client *jellyfin.Client, cfg *config.Config) error {
	// Create player with config fullscreen setting
	p, err := player.New(&player.Options{
		VLCArgs:    []string{"--fullscreen"},
		Fullscreen: cfg.Fullscreen,
	})
	if err != nil {
		return fmt.Errorf("failed to create player: %w", err)
	}
	defer func() {
		playerLock.Lock()
		activePlayer = nil
		activeItemID = ""
		playerLock.Unlock()
		p.Release()
	}()

	// Set the global active player and playback context
	playerLock.Lock()
	activePlayer = p
	activeItemID = itemID
	playerLock.Unlock()

	// Load media from URL
	media, err := p.LoadMediaFromURL(mediaURL)
	if err != nil {
		return fmt.Errorf("failed to load media: %w", err)
	}
	defer media.Release()

	// Setup end reached event
	done, err := p.ListenEndReachedEvent()
	if err != nil {
		return err
	}

	// Start playing
	if err := p.Play(); err != nil {
		return fmt.Errorf("failed to start playback: %w", err)
	}

	// Wait a bit for the player to actually start playing
	time.Sleep(500 * time.Millisecond)

	// Report playback start to Jellyfin
	if err := client.ReportPlaybackStart(itemID, 0); err != nil {
		log.Printf("Warning: failed to report playback start: %v", err)
	} else {
		log.Println("Playback start reported to Jellyfin")
	}

	log.Println("Playing in fullscreen... Press Ctrl+C to stop")

	// Wait for playback to finish or interrupt
	p.WaitForEndOrInterrupt(done)

	// Get final position from state and report playback stopped
	// state := p.GetState()
	// finalPositionMs := state.GetCurrentPositionMs()
	// finalPositionTicks := int64(finalPositionMs) * 10000

	// if err := client.ReportPlaybackStopped(itemID, finalPositionTicks); err != nil {
	// 	log.Printf("Warning: failed to report playback stopped: %v", err)
	// }

	return nil
}
