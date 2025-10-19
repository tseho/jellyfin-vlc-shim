package commands

import (
	"fmt"

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
			return playVideo(args[0])
		},
	}

	return cmd
}

func playVideo(path string) error {
	// Create player with minimal VLC flags for local playback
	p, err := player.New(&player.Options{
		VLCArgs: []string{"--verbose=2"},
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
