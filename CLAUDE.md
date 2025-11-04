# Project Overview

This project is a Jellyfin cast client inspired by [jellyfin-mpv-shim](https://github.com/jellyfin/jellyfin-mpv-shim), but using libVLC as the video player instead of mpv.

## Project Structure

```
.
├── cmd/jellyfin-vlc-shim/     # Main application entry point
├── internal/
│   ├── commands/              # CLI commands (auth, start, play)
│   ├── config/                # Configuration management
│   ├── jellyfin/              # Jellyfin API client (auth, types, client)
│   ├── logger/                # Logging utilities
│   └── player/                # VLC player integration (player, events)
├── tests/                     # E2E tests
├── sources/                   # Reference implementations
└── bin/                       # Build output directory
```

# Development Guidelines

## Language & Style
- Language: Go (golang)
- Naming conventions:
  - Go code: Follow standard Go conventions (camelCase for unexported, PascalCase for exported)
  - JSON config keys: Use snake_case (e.g., `log_level`, `jellyfin_device`)

## Important Constraints
- Never run `./bin/jellyfin-vlc-shim` directly during development.
- Always follow Go best practices and conventions.

# Features

## Working Features
- ✅ Authenticate on Jellyfin server
- ✅ Register as a client capable of playing media
- ✅ Play media from Jellyfin using libVLC
- ✅ Pause/Resume playback
- ✅ Seek during playback
- ✅ Change audio track during playback (`SetAudioStreamIndex`)
- ✅ Change subtitle track during playback (`SetSubtitleStreamIndex`)
- ✅ Support for multiple video/audio/subtitle formats

## Known Limitations
- 🔴 External subtitle files (`.srt`) not yet supported
- 🔴 Mirroring not supported
- 🔴 Screensaver not supported

# Commands

## Build & Test
- `make build` - Build the application using Go (outputs to `./bin/jellyfin-vlc-shim`)
- `make tests` - Run E2E tests

## Usage (for reference only)
The built binary supports these commands:
- `jellyfin-vlc-shim auth` - Authenticate with Jellyfin server
- `jellyfin-vlc-shim` - Start the shim client

# Reference Sources

Local source code for inspiration and reference:
- `sources/jellyfin-mpv-shim` - MPV Cast Client for Jellyfin (Python)
- `sources/jellyfin-apiclient-python` - Python API Client for Jellyfin
- `sources/libvlc-go` - Go bindings for libVLC (v2 and v3)
