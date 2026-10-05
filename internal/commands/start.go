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

	"jellyfin-vlc-shim/internal/config"
	"jellyfin-vlc-shim/internal/jellyfin"
	"jellyfin-vlc-shim/internal/logger"
	"jellyfin-vlc-shim/internal/player"
	"jellyfin-vlc-shim/internal/screensaver"

	"github.com/spf13/cobra"
	ffmpeg "github.com/u2takey/ffmpeg-go"
)

// playbackSession holds the context of a playback
type playbackSession struct {
	player        *player.Player
	done          chan struct{} // closed once the playback has released its player
	itemID        string
	itemInfo      *jellyfin.ItemInfo
	mediaSourceId string
	// Selected streams, guarded by playerLock once the session is active
	audioStreamIndex    *int64
	subtitleStreamIndex *int64 // -1 when subtitles are disabled
}

// streamIndexes returns the selected audio and subtitle stream indexes
func (s *playbackSession) streamIndexes() (audio *int64, subtitle *int64) {
	playerLock.Lock()
	defer playerLock.Unlock()
	return s.audioStreamIndex, s.subtitleStreamIndex
}

// setAudioStreamIndex records the selected audio stream
func (s *playbackSession) setAudioStreamIndex(index int) {
	idx := int64(index)
	playerLock.Lock()
	defer playerLock.Unlock()
	s.audioStreamIndex = &idx
}

// setSubtitleStreamIndex records the selected subtitle stream, -1 when disabled
func (s *playbackSession) setSubtitleStreamIndex(index int) {
	idx := int64(index)
	playerLock.Lock()
	defer playerLock.Unlock()
	s.subtitleStreamIndex = &idx
}

var (
	// Global player state for handling commands, guarded by playerLock
	activeSession *playbackSession
	playerLock    = &sync.Mutex{}
	// playLock serializes Play commands, so only one replaces the active playback at a time
	playLock          = &sync.Mutex{}
	activeScreensaver *screensaver.Screensaver
)

// getActiveSession returns the active playback session, or nil
func getActiveSession() *playbackSession {
	playerLock.Lock()
	defer playerLock.Unlock()
	return activeSession
}

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

	handleMessage := func(msg jellyfin.WebSocketMessage) error {
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

		default:
			dataJSON, err := json.Marshal(msg.Data)
			if err != nil {
				return fmt.Errorf("error marshaling data: %w", err)
			}

			slog.Debug("Received event", "type", msg.MessageType, "data", string(dataJSON))
		}

		return nil
	}

	if !cfg.Screensaver {
		// Connect to WebSocket and handle messages
		return client.ConnectWebSocket(ctx, handleMessage)
	}

	// The screensaver must run on the main goroutine, so the WebSocket runs in the background
	activeScreensaver = screensaver.New()
	clientErr := make(chan error, 1)
	go func() {
		clientErr <- client.ConnectWebSocket(ctx, handleMessage)
		activeScreensaver.Quit()
	}()

	if err := activeScreensaver.Run(); err != nil {
		slog.Error("Failed to run screensaver", "error", err)
	}

	return <-clientErr
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

	// Store item info and media source ID for later use
	session := &playbackSession{
		itemID:   itemID,
		itemInfo: itemInfo,
	}
	if playData.MediaSourceId != "" {
		session.mediaSourceId = playData.MediaSourceId
	} else if len(itemInfo.MediaSources) > 0 {
		session.mediaSourceId = itemInfo.MediaSources[0].Id
	}

	slog.Debug("Media source", "id", session.mediaSourceId)

	// Get audio info
	audio := client.GetAudioInfo(playData, itemInfo)
	if audio != nil {
		idx := int64(audio.Index)
		session.audioStreamIndex = &idx
		slog.Debug("Audio info", "context", audio)
	}

	// Get subtitle info
	subtitle := client.GetSubtitleInfo(playData, itemInfo)
	if subtitle != nil {
		idx := int64(subtitle.Index)
		session.subtitleStreamIndex = &idx
		slog.Debug("Subtitle info", "context", subtitle)
	}

	// Get the direct stream URL
	videoStreamURL := client.GetVideoDirectStreamURL(session.mediaSourceId)
	slog.Debug("Video URL", "url", videoStreamURL)

	// Get start position from playData
	var startPositionMs int64 = 0
	if playData.StartPositionTicks != nil && *playData.StartPositionTicks > 0 {
		// Convert ticks to milliseconds (1 tick = 100 nanoseconds)
		startPositionMs = int64(*playData.StartPositionTicks) / 10000
		slog.Debug("Starting playback from position", "startPositionMs", startPositionMs, "startPositionTicks", *playData.StartPositionTicks)
	}

	if !cfg.BurnExternalSubtitles {
		slog.Debug("Burning external subtitles is disabled")
	}

	// Burn external subtitles if enabled (slow and CPU intensive)
	if cfg.BurnExternalSubtitles && subtitle != nil && subtitle.External {
		return playJellyfinVideoWithExternalSubtitle(videoStreamURL, subtitle, session, client, cfg, startPositionMs)
	}

	return playJellyfinVideo(videoStreamURL, subtitle, session, client, cfg, startPositionMs)
}

func UpdatePlaybackStatus(client *jellyfin.Client, session *playbackSession) {
	state := session.player.GetState()
	positionMs := state.GetCurrentPositionMs()
	positionTicks := int64(positionMs) * 10000

	audioStreamIndex, subtitleStreamIndex := session.streamIndexes()
	if err := client.ReportPlaybackProgress(session.itemID, session.mediaSourceId, audioStreamIndex, subtitleStreamIndex, positionTicks, state.IsPaused); err != nil {
		slog.Warn("Failed to report playback progress", "error", err)
	} else {
		slog.Debug("Reported playback progress", "paused", state.IsPaused, "positionMs", positionMs)
	}
}

// StopPlayback stops the player. The playback goroutine then reports
// playback stopped to Jellyfin and releases the player.
func StopPlayback(player *player.Player) {
	player.Stop()
}

func handlePlaystateCommand(playstateData jellyfin.PlaystateCommandData, client *jellyfin.Client) error {
	session := getActiveSession()
	if session == nil {
		return fmt.Errorf("no active player")
	}
	player := session.player

	command := playstateData.Command
	slog.Debug("Received Playstate command", "command", command)

	switch command {
	case "Pause":
		if err := player.Pause(); err != nil {
			return fmt.Errorf("failed to pause: %w", err)
		}
		UpdatePlaybackStatus(client, session)
	case "Unpause":
		if err := player.Unpause(); err != nil {
			return fmt.Errorf("failed to unpause: %w", err)
		}
		UpdatePlaybackStatus(client, session)
	case "PlayPause":
		if err := player.TogglePause(); err != nil {
			return fmt.Errorf("failed to toggle pause: %w", err)
		}
		UpdatePlaybackStatus(client, session)
	case "Stop":
		StopPlayback(player)
	case "NextTrack":
		slog.Info("NextTrack not yet implemented")
	case "PreviousTrack":
		slog.Info("PreviousTrack not yet implemented")
	case "Seek":
		if player.GetState().IsPaused {
			slog.Warn("Seek during pause is not supported")
			return nil
		}
		if playstateData.SeekPositionTicks == 0 {
			if err := player.SeekTo(0); err != nil {
				return fmt.Errorf("failed to seek: %w", err)
			}
		}
		if playstateData.SeekPositionTicks > 0 {
			// Convert ticks to milliseconds (1 tick = 100 nanoseconds)
			seekTimeMs := playstateData.SeekPositionTicks / 10000
			if err := player.SeekTo(seekTimeMs); err != nil {
				return fmt.Errorf("failed to seek: %w", err)
			}
		}
		UpdatePlaybackStatus(client, session)
	default:
		slog.Warn("Unknown playstate command", "command", command)
	}

	return nil
}

func handleGeneralCommand(generalData jellyfin.GeneralCommandData, client *jellyfin.Client) error {
	session := getActiveSession()
	var p *player.Player
	if session != nil {
		p = session.player
	}

	command := generalData.Name
	slog.Debug("Received General command", "command", command)

	switch command {
	case "SetAudioStreamIndex":
		if p == nil {
			return fmt.Errorf("no active player")
		}
		if indexValue, ok := generalData.Arguments["Index"]; ok {
			// The index could be a string or a number
			var streamAudioIndex int
			switch v := indexValue.(type) {
			case string:
				// Parse string to int
				if _, err := fmt.Sscanf(v, "%d", &streamAudioIndex); err != nil {
					return fmt.Errorf("failed to parse audio index: %w", err)
				}
			case float64:
				streamAudioIndex = int(v)
			case int:
				streamAudioIndex = v
			default:
				return fmt.Errorf("unexpected type for audio index: %T", indexValue)
			}

			// Convert Jellyfin audio index to VLC audio index
			audioIndex, err := client.GetAudioIndexInStreamAudios(streamAudioIndex, session.mediaSourceId, session.itemInfo)
			if err != nil {
				return fmt.Errorf("failed to convert audio index: %w", err)
			}

			slog.Info("Setting audio stream", "jellyfinIndex", streamAudioIndex, "vlcIndex", audioIndex)
			if err := p.EnableAudio(audioIndex); err != nil {
				return fmt.Errorf("failed to enable audio: %w", err)
			}
			session.setAudioStreamIndex(streamAudioIndex)
			UpdatePlaybackStatus(client, session)
		} else {
			return fmt.Errorf("missing Index argument for SetAudioStreamIndex")
		}
	case "SetSubtitleStreamIndex":
		if p == nil {
			return fmt.Errorf("no active player")
		}
		if indexValue, ok := generalData.Arguments["Index"]; ok {
			// The index could be a string or a number
			var streamSubtitleIndex int
			switch v := indexValue.(type) {
			case string:
				// Parse string to int
				if _, err := fmt.Sscanf(v, "%d", &streamSubtitleIndex); err != nil {
					return fmt.Errorf("failed to parse subtitle index: %w", err)
				}
			case float64:
				streamSubtitleIndex = int(v)
			case int:
				streamSubtitleIndex = v
			default:
				return fmt.Errorf("unexpected type for subtitle index: %T", indexValue)
			}

			if streamSubtitleIndex < 0 {
				// Jellyfin sends -1 to turn subtitles off
				slog.Info("Disabling subtitles")
				if err := p.DisableSubtitle(); err != nil {
					return fmt.Errorf("failed to disable subtitle: %w", err)
				}
				session.setSubtitleStreamIndex(-1)
				UpdatePlaybackStatus(client, session)
				break
			}

			// Convert Jellyfin subtitle index to VLC subtitle index
			subtitleIndex, err := client.GetSubtitleIndexInStreamSubtitles(streamSubtitleIndex, session.mediaSourceId, session.itemInfo)
			if err != nil {
				return fmt.Errorf("failed to convert subtitle index: %w", err)
			}

			slog.Info("Setting subtitle stream", "jellyfinIndex", streamSubtitleIndex, "vlcIndex", subtitleIndex)
			if err := p.EnableSubtitle(subtitleIndex); err != nil {
				return fmt.Errorf("failed to enable subtitle: %w", err)
			}
			session.setSubtitleStreamIndex(streamSubtitleIndex)
			UpdatePlaybackStatus(client, session)
		} else {
			return fmt.Errorf("missing Index argument for SetSubtitleStreamIndex")
		}
	default:
		slog.Warn("Unknown general command", "command", command)
	}

	return nil
}

func playJellyfinVideo(mediaURL string, subtitle *jellyfin.SubtitleInfo, session *playbackSession, client *jellyfin.Client, cfg *config.Config, startPositionMs int64) error {
	itemID := session.itemID

	// Hold playLock until this playback is the active one, so concurrent
	// Play commands can't both replace the same previous playback
	playLock.Lock()
	unlockPlay := sync.OnceFunc(playLock.Unlock)
	defer unlockPlay()

	// Stop any existing playback and wait for it to release its player,
	// as libVLC uses a single global instance
	if previous := getActiveSession(); previous != nil {
		StopPlayback(previous.player)
		<-previous.done
	}

	// Hide screensaver when playback starts
	if activeScreensaver != nil {
		activeScreensaver.Hide()
	}

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
	session.player = p
	session.done = make(chan struct{})

	defer func() {
		defer close(session.done)

		playerLock.Lock()
		if activeSession == session {
			activeSession = nil
		}
		playerLock.Unlock()
		p.Release()

		os.Remove(subtitleTempPath)

		// Show screensaver again after playback ends
		if activeScreensaver != nil {
			activeScreensaver.Show()
		}
	}()

	// Set the global active playback session
	playerLock.Lock()
	activeSession = session
	playerLock.Unlock()
	unlockPlay()

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

	// Setup playback end events
	done, err := p.ListenPlaybackEndEvents()
	if err != nil {
		return err
	}

	// Playback was stopped while loading the media
	select {
	case <-done:
		return nil
	default:
	}

	// Start playing
	if err := p.Play(); err != nil {
		return fmt.Errorf("failed to start playback: %w", err)
	}

	// Wait a bit for the player to actually start playing
	time.Sleep(100 * time.Millisecond)

	// Seek to start position if specified
	if startPositionMs > 0 {
		slog.Debug("Seeking to start position", "positionMs", startPositionMs)
		if err := p.SeekTo(startPositionMs); err != nil {
			slog.Warn("Failed to seek to start position", "error", err, "positionMs", startPositionMs)
		}
	}

	if audioStreamIndex, _ := session.streamIndexes(); audioStreamIndex != nil {
		// Convert Jellyfin audio index to VLC audio index
		audioIndex, err := client.GetAudioIndexInStreamAudios(int(*audioStreamIndex), session.mediaSourceId, session.itemInfo)
		if err != nil {
			slog.Warn("Failed to convert audio index", "error", err)
		} else if err := p.EnableAudio(audioIndex); err != nil {
			slog.Warn("Failed to enable audio", "error", err)
		}
	}

	if subtitle != nil && !subtitle.External {
		// Convert Jellyfin subtitle index to VLC subtitle index
		subtitleIndex, err := client.GetSubtitleIndexInStreamSubtitles(subtitle.Index, session.mediaSourceId, session.itemInfo)
		if err != nil {
			slog.Warn("Failed to convert subtitle index", "error", err)
		} else if err := p.EnableSubtitle(subtitleIndex); err != nil {
			slog.Warn("Failed to enable subtitle", "error", err)
		}
	}

	// Report playback start to Jellyfin with the actual start position
	startPositionTicks := startPositionMs * 10000
	audioStreamIndex, subtitleStreamIndex := session.streamIndexes()
	if err := client.ReportPlaybackStart(itemID, session.mediaSourceId, audioStreamIndex, subtitleStreamIndex, startPositionTicks); err != nil {
		slog.Warn("Failed to report playback start", "error", err)
	}

	// Wait for playback to finish
	<-done
	slog.Info("Video playback ended")

	// Get final position from state and report playback stopped
	state := p.GetState()
	finalPositionMs := state.GetCurrentPositionMs()
	finalPositionTicks := int64(finalPositionMs) * 10000

	if err := client.ReportPlaybackStopped(itemID, finalPositionTicks); err != nil {
		slog.Warn("Failed to report playback stopped", "error", err)
	}

	return nil
}

func playJellyfinVideoWithExternalSubtitle(mediaURL string, subtitle *jellyfin.SubtitleInfo, session *playbackSession, client *jellyfin.Client, cfg *config.Config, startPositionMs int64) error {
	slog.Info("Burning external subtitles into video stream...")

	subtitleTempPath := fmt.Sprintf("/tmp/%s.srt", session.itemID)
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

	return playJellyfinVideo(streamURL, nil, session, client, cfg, startPositionMs)
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
