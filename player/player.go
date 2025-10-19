package player

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	vlc "github.com/adrg/libvlc-go/v3"
)

// State represents the playback state of the player
type State struct {
	IsPaused       bool
	StartTime      time.Time
	PausedAt       time.Time
	TotalPausedDur time.Duration
	SeekOffset     int64 // in milliseconds
}

// GetCurrentPositionMs returns the current playback position in milliseconds
func (s *State) GetCurrentPositionMs() int {
	if s.IsPaused {
		// Return position at pause time
		elapsed := s.PausedAt.Sub(s.StartTime) - s.TotalPausedDur
		return int(elapsed.Milliseconds()) + int(s.SeekOffset)
	}
	// Return current position
	elapsed := time.Since(s.StartTime) - s.TotalPausedDur
	return int(elapsed.Milliseconds()) + int(s.SeekOffset)
}

// Player wraps a VLC player instance with state management
type Player struct {
	player *vlc.Player
	state  *State
	lock   sync.Mutex
}

// Options configures player initialization
type Options struct {
	VLCArgs []string
}

// DefaultOptions returns default player options with fullscreen enabled
func DefaultOptions() *Options {
	return &Options{
		VLCArgs: []string{"--fullscreen"},
	}
}

// New creates a new Player instance with the given options
func New(opts *Options) (*Player, error) {
	if opts == nil {
		opts = DefaultOptions()
	}

	// Initialize libVLC
	if err := vlc.Init(opts.VLCArgs...); err != nil {
		return nil, fmt.Errorf("failed to initialize libVLC: %w", err)
	}

	// Create a new VLC player
	vlcPlayer, err := vlc.NewPlayer()
	if err != nil {
		vlc.Release()
		return nil, fmt.Errorf("failed to create player: %w", err)
	}

	return &Player{
		player: vlcPlayer,
		state: &State{
			IsPaused:       false,
			StartTime:      time.Now(),
			TotalPausedDur: 0,
			SeekOffset:     0,
		},
	}, nil
}

// Release cleans up player resources
func (p *Player) Release() {
	p.player.Stop()
	p.player.Release()
	vlc.Release()
}

// LoadMediaFromPath loads a media file from a local path
func (p *Player) LoadMediaFromPath(path string) (*vlc.Media, error) {
	// Convert to absolute path if relative
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve path: %w", err)
	}

	// Check if file exists
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("file does not exist: %s", absPath)
	}

	log.Printf("Loading media: %s", absPath)

	// Load media from path
	media, err := p.player.LoadMediaFromPath(absPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load media: %w", err)
	}

	return media, nil
}

// LoadMediaFromURL loads a media file from a URL
func (p *Player) LoadMediaFromURL(url string) (*vlc.Media, error) {
	log.Printf("Loading media from URL: %s", url)

	// Load media from URL
	media, err := p.player.LoadMediaFromURL(url)
	if err != nil {
		return nil, fmt.Errorf("failed to load media: %w", err)
	}

	return media, nil
}

// Play starts playback
func (p *Player) Play() error {
	p.lock.Lock()
	defer p.lock.Unlock()

	log.Println("Starting playback...")
	if err := p.player.Play(); err != nil {
		return fmt.Errorf("failed to start playback: %w", err)
	}

	// Reset state
	p.state.StartTime = time.Now()
	p.state.IsPaused = false
	p.state.TotalPausedDur = 0
	p.state.SeekOffset = 0

	return nil
}

// Stop stops playback
func (p *Player) Stop() {
	p.lock.Lock()
	defer p.lock.Unlock()

	p.player.Stop()
	log.Println("Player stopped")
}

// GetState returns a copy of the current playback state
func (p *Player) GetState() State {
	p.lock.Lock()
	defer p.lock.Unlock()

	return *p.state
}

// Pause pauses playback
func (p *Player) Pause() error {
	p.lock.Lock()
	defer p.lock.Unlock()

	if !p.state.IsPaused {
		if err := p.player.SetPause(true); err != nil {
			return fmt.Errorf("failed to pause: %w", err)
		}
		p.state.IsPaused = true
		p.state.PausedAt = time.Now()
		log.Println("Player paused")
	}
	return nil
}

// Unpause resumes playback
func (p *Player) Unpause() error {
	p.lock.Lock()
	defer p.lock.Unlock()

	if p.state.IsPaused {
		if err := p.player.SetPause(false); err != nil {
			return fmt.Errorf("failed to unpause: %w", err)
		}
		// Add the paused duration to total
		p.state.TotalPausedDur += time.Since(p.state.PausedAt)
		p.state.IsPaused = false
		log.Println("Player unpaused")
	}
	return nil
}

// TogglePause toggles the pause state
func (p *Player) TogglePause() error {
	p.lock.Lock()
	isPaused := p.state.IsPaused
	p.lock.Unlock()

	if isPaused {
		return p.Unpause()
	}
	return p.Pause()
}

// Seek seeks to a specific position in milliseconds
func (p *Player) Seek(positionMs int64) error {
	p.lock.Lock()
	defer p.lock.Unlock()

	if err := p.player.SetMediaTime(int(positionMs)); err != nil {
		return fmt.Errorf("failed to seek: %w", err)
	}

	// Update our local state tracking
	p.state.SeekOffset = positionMs
	p.state.StartTime = time.Now()
	p.state.TotalPausedDur = 0
	if p.state.IsPaused {
		p.state.PausedAt = time.Now()
	}
	log.Printf("Seeked to position: %d ms", positionMs)

	return nil
}
