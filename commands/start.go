package commands

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"jellyfin-vlc-shim/config"
	"jellyfin-vlc-shim/jellyfin"
	"jellyfin-vlc-shim/player"

	"github.com/spf13/cobra"
)

var (
	// Global player state for handling commands
	activePlayer *player.Player
	activeItemID string
	playerLock   = &sync.Mutex{}
)

func NewStartCmd(configDir *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the Jellyfin VLC Shim client",
		Long:  "Start the client, register with Jellyfin server, and listen for playback commands",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runClient(*configDir)
		},
	}

	return cmd
}

func runClient(configDir string) error {
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
	state := p.GetState()
	finalPositionMs := state.GetCurrentPositionMs()
	finalPositionTicks := int64(finalPositionMs) * 10000

	if err := client.ReportPlaybackStopped(itemID, finalPositionTicks); err != nil {
		log.Printf("Warning: failed to report playback stopped: %v", err)
	}

	return nil
}
