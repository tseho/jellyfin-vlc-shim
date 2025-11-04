package main

import (
	"os"

	"jellyfin-vlc-shim/internal/commands"

	"github.com/spf13/cobra"
)

var configDir string

func main() {
	startCmd := commands.NewStartCmd(&configDir)

	rootCmd := &cobra.Command{
		Use:   "jellyfin-vlc-shim",
		Short: "Jellyfin VLC Shim - Control VLC from Jellyfin",
		Long:  "Remote player for Jellyfin using VLC",
		RunE:  startCmd.RunE,
		CompletionOptions: cobra.CompletionOptions{
			DisableDefaultCmd: true,
		},
	}

	rootCmd.PersistentFlags().StringVar(&configDir, "config", "", "Configuration directory (default: ~/.config/jellyfin-vlc-shim)")

	rootCmd.AddCommand(commands.NewAuthCmd(&configDir))
	rootCmd.AddCommand(commands.NewPlayCmd(&configDir))

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
