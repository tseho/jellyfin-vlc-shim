package screensaver

import (
	"bytes"
	"fmt"
	"image/color"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"golang.org/x/image/font/gofont/goregular"
)

const (
	fontSize = 48
	padding  = 40
	// Maximum time Update waits when there is nothing to redraw
	idleWait = 5 * time.Second
)

// Screensaver displays a black screen with the current time.
// Ebitengine only allows one game per process, so the screensaver window lives
// for the whole process and is hidden (minimized) during playback.
type Screensaver struct {
	visible atomic.Bool
	quit    atomic.Bool
	wake    chan struct{}
}

// New creates a new Screensaver instance, visible by default
func New() *Screensaver {
	s := &Screensaver{
		wake: make(chan struct{}, 1),
	}
	s.visible.Store(true)
	return s
}

// Run runs the screensaver until Quit is called.
// It must be called on the main goroutine, and only once.
func (s *Screensaver) Run() error {
	slog.Info("Starting screensaver")

	ebiten.SetFullscreen(true)
	ebiten.SetWindowTitle("Jellyfin VLC Shim")
	ebiten.SetCursorMode(ebiten.CursorModeHidden)
	ebiten.SetTPS(1)                         // Only update once per second to save CPU (there is an additional wait in Update)
	ebiten.SetVsyncEnabled(false)            // Disable VSync to save CPU
	ebiten.SetScreenClearedEveryFrame(false) // Don't clear screen every frame to save CPU

	game := &Game{
		screensaver: s,
		shown:       true,
		needsRedraw: true,
	}

	if err := ebiten.RunGame(game); err != nil {
		return fmt.Errorf("failed to run screensaver: %w", err)
	}

	return nil
}

// Show shows the screensaver
func (s *Screensaver) Show() {
	if !s.visible.Swap(true) {
		slog.Info("Showing screensaver")
		s.notify()
	}
}

// Hide hides the screensaver
func (s *Screensaver) Hide() {
	if s.visible.Swap(false) {
		slog.Info("Hiding screensaver")
		s.notify()
	}
}

// Quit makes Run return
func (s *Screensaver) Quit() {
	s.quit.Store(true)
	s.notify()
}

// notify wakes up Update if it is waiting
func (s *Screensaver) notify() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// wait blocks until notified or until the timeout expires
func (s *Screensaver) wait(timeout time.Duration) {
	select {
	case <-s.wake:
	case <-time.After(timeout):
	}
}

// Game implements ebiten.Game interface
type Game struct {
	screensaver  *Screensaver
	shown        bool
	faceSource   *text.GoTextFaceSource
	face         *text.GoTextFace
	screenWidth  int
	screenHeight int
	time         string
	needsRedraw  bool
}

// Update updates the game state
func (g *Game) Update() error {
	if g.screensaver.quit.Load() {
		return ebiten.Termination
	}

	// Apply visibility changes requested by Show/Hide
	if visible := g.screensaver.visible.Load(); visible != g.shown {
		g.shown = visible
		if visible {
			ebiten.RestoreWindow()
			ebiten.SetFullscreen(true)
			g.needsRedraw = true
		} else {
			ebiten.SetFullscreen(false)
			ebiten.MinimizeWindow()
		}
	}

	ebiten.SetCursorMode(ebiten.CursorModeHidden)

	// Check if time has changed
	currentTime := time.Now().Format("15:04")
	if currentTime != g.time {
		g.time = currentTime
		g.needsRedraw = true
	}

	// Wait a bit to reduce CPU usage
	if !g.needsRedraw || !g.shown {
		g.screensaver.wait(idleWait)
	}

	return nil
}

// Draw draws the screensaver
func (g *Game) Draw(screen *ebiten.Image) {
	// Only redraw text if time has changed
	if !g.needsRedraw || !g.shown {
		return
	}

	screen.Fill(color.Black)

	// Initialize font if needed (use Go's built-in font)
	if g.faceSource == nil {
		s, err := text.NewGoTextFaceSource(bytes.NewReader(goregular.TTF))
		if err != nil {
			slog.Error("Failed to create font face source", "error", err)
			return
		}
		g.faceSource = s
	}

	// Create face once and cache it
	if g.face == nil {
		g.face = &text.GoTextFace{
			Source: g.faceSource,
			Size:   fontSize,
		}
	}

	// Calculate text dimensions
	textWidth, textHeight := text.Measure(g.time, g.face, 0)

	// Position text at bottom right with padding
	x := float64(g.screenWidth) - textWidth - padding
	y := float64(g.screenHeight) - textHeight - padding

	// Draw the time
	textOp := &text.DrawOptions{}
	textOp.GeoM.Translate(x, y)
	textOp.ColorScale.ScaleWithColor(color.White)
	text.Draw(screen, g.time, g.face, textOp)

	g.needsRedraw = false
}

// Layout returns the game's logical screen size
func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	// Store the actual screen dimensions
	g.screenWidth = outsideWidth
	g.screenHeight = outsideHeight
	return outsideWidth, outsideHeight
}
