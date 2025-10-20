package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"jellyfin-vlc-shim/config"
	"jellyfin-vlc-shim/jellyfin"
	"jellyfin-vlc-shim/player"

	"github.com/spf13/cobra"
	ffmpeg "github.com/u2takey/ffmpeg-go"
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
	log.Printf("ConfigDir: %s", dir)
	log.Printf("Server: %s", creds.ServerURL)
	log.Printf("User: %s", creds.Username)
	log.Printf("Device ID: %s", cfg.DeviceID)

	// Create Jellyfin client
	client := jellyfin.NewClient(creds.ServerURL, creds.AccessToken, creds.UserID, cfg.DeviceID, cfg.JellyfinClient, cfg.JellyfinDevice)

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle Ctrl+C and SIGTERM to stop gracefully
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(interrupt)

	go func() {
		<-interrupt
		log.Println("Received shutdown signal, closing...")
		cancel()
	}()

	// Register capabilities
	log.Println("Registering capabilities with Jellyfin server...")
	if err := client.RegisterCapabilities(); err != nil {
		return fmt.Errorf("failed to register capabilities: %w", err)
	}
	log.Println("Capabilities registered successfully!")

	// Connect to WebSocket and handle messages
	return client.ConnectWebSocket(ctx, func(msg jellyfin.WebSocketMessage) error {
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

	log.Printf("Playing: %s", itemInfo.Name)
	itemInfoJSON, _ := json.MarshalIndent(itemInfo, "", "  ")
	log.Printf("Item info: %s", string(itemInfoJSON))

	// Log MediaSourceId if present
	if playData.MediaSourceId != "" {
		log.Printf("MediaSourceId: %s", playData.MediaSourceId)
	}

	// Get the direct stream URL
	videoStreamURL := client.GetVideoDirectStreamURL(itemID)
	log.Printf("Video URL: %s", videoStreamURL)

	// Get subtitle
	subtitle := client.GetSubtitleInfo(playData, itemInfo)
	if subtitle != nil {
		subtitleJSON, _ := json.MarshalIndent(subtitle, "", "  ")
		log.Printf("Subtitle info: %s", string(subtitleJSON))
	}

	// Burn external subtitles if enabled (slow and CPU intensive)
	if cfg.BurnExternalSubtitles && subtitle != nil && subtitle.External {
		return playJellyfinVideoWithExternalSubtitle(videoStreamURL, subtitle, itemID, client, cfg)
	}

	return playJellyfinVideo(videoStreamURL, subtitle, itemID, client, cfg)
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
		if playstateData.SeekPositionTicks > 0 {
			// Convert ticks to milliseconds (1 tick = 100 nanoseconds)
			seekTimeMs := playstateData.SeekPositionTicks / 10000
			if err := p.SeekTo(seekTimeMs); err != nil {
				return fmt.Errorf("failed to seek: %w", err)
			}
			UpdatePlaybackStatus(client, p)
		}
	default:
		log.Printf("Unknown playstate command: %s", command)
	}

	return nil
}

func playJellyfinVideo(mediaURL string, subtitle *jellyfin.SubtitleInfo, itemID string, client *jellyfin.Client, cfg *config.Config) error {
	vlcArgs := []string{}
	if cfg.VLCVerbose {
		vlcArgs = append(vlcArgs, "--verbose=2")
	}

	p, err := player.New(&player.Options{
		VLCArgs:    vlcArgs,
		Fullscreen: cfg.Fullscreen,
	})
	if err != nil {
		return fmt.Errorf("failed to create player: %w", err)
	}

	subtitleTempPath := fmt.Sprintf("/tmp/%s.srt", itemID)

	defer func() {
		playerLock.Lock()
		activePlayer = nil
		activeItemID = ""
		playerLock.Unlock()
		p.Release()

		os.Remove(subtitleTempPath)
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

	if subtitle != nil && subtitle.External {
		err := downloadSubtitle(*subtitle.URL, subtitleTempPath)
		if err != nil {
			return err
		}

		// Add the subtitle file option to VLC
		if err := media.AddOptions(fmt.Sprintf(":sub-file=%s", subtitleTempPath)); err != nil {
			log.Printf("Warning: failed to add subtitle option: %v", err)
		} else {
			log.Printf("Added subtitle file option: %s", subtitleTempPath)
		}
	}

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
	time.Sleep(100 * time.Millisecond)

	if subtitle != nil && !subtitle.External {
		p.EnableSubtitle(subtitle.Index)
	}

	// Report playback start to Jellyfin
	if err := client.ReportPlaybackStart(itemID, 0); err != nil {
		log.Printf("Warning: failed to report playback start: %v", err)
	} else {
		log.Println("Playback start reported to Jellyfin")
	}

	log.Println("Playing in fullscreen... Press Ctrl+C to stop")

	// Wait for playback to finish
	<-done
	log.Println("Video playback completed")

	// Get final position from state and report playback stopped
	state := p.GetState()
	finalPositionMs := state.GetCurrentPositionMs()
	finalPositionTicks := int64(finalPositionMs) * 10000

	if err := client.ReportPlaybackStopped(itemID, finalPositionTicks); err != nil {
		log.Printf("Warning: failed to report playback stopped: %v", err)
	}

	return nil
}

func playJellyfinVideoWithExternalSubtitle(mediaURL string, subtitle *jellyfin.SubtitleInfo, itemID string, client *jellyfin.Client, cfg *config.Config) error {
	subtitleTempPath := fmt.Sprintf("/tmp/%s.srt", itemID)
	err := downloadSubtitle(*subtitle.URL, subtitleTempPath)
	if err != nil {
		return err
	}

	defer func() {
		if err := os.Remove(subtitleTempPath); err != nil {
			log.Printf("Warning: failed to remove subtitle file: %v", err)
		}
	}()

	streamURL, err := startStreamWithBurnedSubtitles(mediaURL, subtitleTempPath, cfg)
	if err != nil {
		return fmt.Errorf("failed to start stream with burned subtitles: %w", err)
	}

	return playJellyfinVideo(streamURL, nil, itemID, client, cfg)
}

func downloadSubtitle(url, path string) error {
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to download subtitle: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download subtitle, status: %d", resp.StatusCode)
	}

	out, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create subtitle file: %w", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, resp.Body); err != nil {
		return fmt.Errorf("failed to write subtitle file: %w", err)
	}

	log.Printf("Downloaded subtitle to: %s", path)
	return nil
}

func startStreamWithBurnedSubtitles(inputURL, subtitleFile string, cfg *config.Config) (string, error) {
	outputURL := "http://0.0.0.0:8090/stream"

	go func() {
		log.Printf("Starting ffmpeg stream: %s", outputURL)
		err := ffmpeg.Input(inputURL, ffmpeg.KwArgs{"re": ""}).
			Filter("subtitles", ffmpeg.Args{subtitleFile}).
			Output(outputURL, ffmpeg.KwArgs{
				"c:v":    cfg.BurnEncoder,
				"preset": cfg.BurnSpeed,
				"crf":    "20",
				"tune":   "zerolatency",
				"g":      "48",
				"c:a":    "copy",
				"f":      "mpegts",
				"listen": "1",
			}).
			OverWriteOutput().
			Run()

		if err != nil {
			log.Printf("Error in ffmpeg stream: %v", err)
		}
	}()

	// Give ffmpeg a moment to start listening
	time.Sleep(100 * time.Millisecond)

	return outputURL, nil
}
