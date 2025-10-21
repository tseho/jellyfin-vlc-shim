package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"jellyfin-vlc-shim/config"
	"jellyfin-vlc-shim/jellyfin"
	"jellyfin-vlc-shim/logger"
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

	// Initialize logger with configured log level
	logger.Initialize(cfg.LogLevel)

	// Load credentials
	creds, err := config.LoadCredentials(dir)
	if err != nil {
		return fmt.Errorf("failed to load credentials: %w\nPlease run 'jellyfin-vlc-shim auth' first", err)
	}

	slog.Info("Starting Jellyfin VLC Shim client")
	slog.Info("Configuration loaded", "configDir", dir, "server", creds.ServerURL, "user", creds.Username, "deviceID", cfg.DeviceID)

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
		slog.Info("Received shutdown signal, closing...")
		cancel()
	}()

	// Register capabilities
	slog.Debug("Registering capabilities with Jellyfin server...")
	if err := client.RegisterCapabilities(); err != nil {
		return fmt.Errorf("failed to register capabilities: %w", err)
	}
	slog.Debug("Capabilities registered successfully!")

	// Connect to WebSocket and handle messages
	return client.ConnectWebSocket(ctx, func(msg jellyfin.WebSocketMessage) error {
		switch msg.MessageType {
		case "Play":
			slog.Debug("Received Play command")

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
					slog.Error("Error handling play command", "error", err)
				}
			}()

		case "Playstate":
			slog.Debug("Received Playstate command")

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
				slog.Error("Error handling playstate command", "error", err)
			}

		case "GeneralCommand":
			slog.Debug("Received GeneralCommand")

			// Parse the general command data
			dataJSON, err := json.Marshal(msg.Data)
			if err != nil {
				return fmt.Errorf("error marshaling general command data: %w", err)
			}

			var generalData jellyfin.GeneralCommandData
			if err := json.Unmarshal(dataJSON, &generalData); err != nil {
				return fmt.Errorf("error parsing general command: %w", err)
			}

			// Handle the general command
			if err := handleGeneralCommand(generalData, client); err != nil {
				slog.Error("Error handling general command", "error", err)
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

	slog.Debug("Fetching info for item", "itemID", itemID)
	itemInfo, err := client.GetItemInfo(itemID)
	if err != nil {
		return fmt.Errorf("failed to get item info: %w", err)
	}

	slog.Info("Playing", "name", itemInfo.Name)

	slog.Debug("Play data", "context", playData)
	slog.Debug("Item info", "context", itemInfo)

	// Get the direct stream URL
	videoStreamURL := client.GetVideoDirectStreamURL(itemID)
	slog.Debug("Video URL", "url", videoStreamURL)

	// Get subtitle
	subtitle := client.GetSubtitleInfo(playData, itemInfo)
	if subtitle != nil {
		slog.Debug("Subtitle info", "context", subtitle)
	}

	if !cfg.BurnExternalSubtitles {
		slog.Debug("Burning external subtitles is disabled")
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
		slog.Warn("Failed to report playback progress", "error", err)
	} else {
		slog.Debug("Reported playback progress", "paused", state.IsPaused, "positionMs", positionMs)
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
	slog.Debug("Received Playstate command", "command", command)

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
		slog.Info("NextTrack not yet implemented")
	case "PreviousTrack":
		slog.Info("PreviousTrack not yet implemented")
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
		slog.Warn("Unknown playstate command", "command", command)
	}

	return nil
}

func handleGeneralCommand(generalData jellyfin.GeneralCommandData, client *jellyfin.Client) error {
	playerLock.Lock()
	p := activePlayer
	playerLock.Unlock()

	if p == nil {
		return fmt.Errorf("no active player")
	}

	command := generalData.Name
	slog.Debug("Received General command", "command", command)

	switch command {
	case "SetAudioStreamIndex":
		if indexValue, ok := generalData.Arguments["Index"]; ok {
			// The index could be a string or a number
			var audioIndex int
			switch v := indexValue.(type) {
			case string:
				// Parse string to int
				if _, err := fmt.Sscanf(v, "%d", &audioIndex); err != nil {
					return fmt.Errorf("failed to parse audio index: %w", err)
				}
			case float64:
				audioIndex = int(v)
			case int:
				audioIndex = v
			default:
				return fmt.Errorf("unexpected type for audio index: %T", indexValue)
			}

			slog.Info("Setting audio stream index", "index", audioIndex)
			if err := p.EnableAudio(audioIndex); err != nil {
				return fmt.Errorf("failed to enable audio: %w", err)
			}
		} else {
			return fmt.Errorf("missing Index argument for SetAudioStreamIndex")
		}
	case "SetSubtitleStreamIndex":
		if indexValue, ok := generalData.Arguments["Index"]; ok {
			// The index could be a string or a number
			var subtitleIndex int
			switch v := indexValue.(type) {
			case string:
				// Parse string to int
				if _, err := fmt.Sscanf(v, "%d", &subtitleIndex); err != nil {
					return fmt.Errorf("failed to parse subtitle index: %w", err)
				}
			case float64:
				subtitleIndex = int(v)
			case int:
				subtitleIndex = v
			default:
				return fmt.Errorf("unexpected type for subtitle index: %T", indexValue)
			}

			slog.Info("Setting subtitle stream index", "index", subtitleIndex)
			if err := p.EnableSubtitle(subtitleIndex); err != nil {
				return fmt.Errorf("failed to enable subtitle: %w", err)
			}
		} else {
			return fmt.Errorf("missing Index argument for SetSubtitleStreamIndex")
		}
	default:
		slog.Warn("Unknown general command", "command", command)
	}

	return nil
}

func playJellyfinVideo(mediaURL string, subtitle *jellyfin.SubtitleInfo, itemID string, client *jellyfin.Client, cfg *config.Config) error {
	vlcArgs := []string{}
	if cfg.VLCDebug {
		vlcArgs = append(vlcArgs, "--verbose=2")
	} else {
		vlcArgs = append(vlcArgs, "--quiet", "--file-logging", "--logfile=/dev/null")
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
			slog.Warn("Failed to add subtitle option", "error", err)
		} else {
			slog.Debug("Added subtitle using vlc sub-file", "path", subtitleTempPath)
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
		if err := p.EnableSubtitle(subtitle.Index); err != nil {
			slog.Warn("Failed to enable subtitle", "error", err)
		}
	}

	// Report playback start to Jellyfin
	if err := client.ReportPlaybackStart(itemID, 0); err != nil {
		slog.Warn("Failed to report playback start", "error", err)
	}

	// Wait for playback to finish
	<-done
	slog.Info("Video playback completed")

	// Get final position from state and report playback stopped
	state := p.GetState()
	finalPositionMs := state.GetCurrentPositionMs()
	finalPositionTicks := int64(finalPositionMs) * 10000

	if err := client.ReportPlaybackStopped(itemID, finalPositionTicks); err != nil {
		slog.Warn("Failed to report playback stopped", "error", err)
	}

	return nil
}

func playJellyfinVideoWithExternalSubtitle(mediaURL string, subtitle *jellyfin.SubtitleInfo, itemID string, client *jellyfin.Client, cfg *config.Config) error {
	slog.Info("Burning external subtitles into video stream...")

	subtitleTempPath := fmt.Sprintf("/tmp/%s.srt", itemID)
	err := downloadSubtitle(*subtitle.URL, subtitleTempPath)
	if err != nil {
		return err
	}

	defer func() {
		if err := os.Remove(subtitleTempPath); err != nil {
			slog.Warn("Failed to remove subtitle file", "error", err)
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

	slog.Debug("Downloaded subtitle", "path", path)
	return nil
}

func startStreamWithBurnedSubtitles(inputURL, subtitleFile string, cfg *config.Config) (string, error) {
	outputURL := "http://0.0.0.0:8090/stream"

	go func() {
		slog.Debug("Starting ffmpeg stream", "url", outputURL)
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
			slog.Error("Error in ffmpeg stream", "error", err)
		}
	}()

	// Give ffmpeg a moment to start listening
	time.Sleep(100 * time.Millisecond)

	return outputURL, nil
}
