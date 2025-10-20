package commands

import (
	"fmt"
	"log/slog"

	"jellyfin-vlc-shim/config"
	"jellyfin-vlc-shim/logger"
	"jellyfin-vlc-shim/player"

	"github.com/spf13/cobra"
)

func NewPlayCmd(configDir *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "play [path]",
		Short: "Play a local video file using VLC",
		Long:  "Play a local video file using libVLC. The file path must be an absolute or relative path to a video file.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return playVideo(args[0], *configDir)
		},
	}

	return cmd
}

func playVideo(path string, configDir string) error {
	// Get config directory and load configuration
	dir, err := config.GetConfigDir(configDir)
	if err != nil {
		return fmt.Errorf("failed to get config directory: %w", err)
	}

	cfg, err := config.Load(dir)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Initialize logger with configured log level
	logger.Initialize(cfg.LogLevel)

	// Create player with minimal VLC flags for local playback
	vlcArgs := []string{}
	if cfg.VLCDebug {
		vlcArgs = append(vlcArgs, "--verbose=2")
	} else {
		vlcArgs = append(vlcArgs, "--quiet")
	}

	p, err := player.New(&player.Options{
		VLCArgs: vlcArgs,
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

	// Wait for playback to finish
	<-done
	slog.Info("Video playback completed")

	return nil
}
