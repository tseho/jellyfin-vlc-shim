package screensaver

import (
	"bytes"
	"fmt"
	"image/color"
	"log/slog"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"golang.org/x/image/font/gofont/goregular"
)

const (
	fontSize = 48
	padding  = 40
)

// Screensaver displays a black screen with the current time
type Screensaver struct {
	running bool
}

// New creates a new Screensaver instance
func New() *Screensaver {
	return &Screensaver{
		running: false,
	}
}

// Start starts the screensaver in fullscreen mode
func (s *Screensaver) Start() error {
	if s.running {
		return nil
	}

	slog.Info("Starting screensaver")
	s.running = true

	ebiten.SetFullscreen(true)
	ebiten.SetWindowTitle("Jellyfin VLC Shim")
	ebiten.SetCursorMode(ebiten.CursorModeHidden)

	game := &Game{}

	if err := ebiten.RunGame(game); err != nil {
		s.running = false
		return fmt.Errorf("failed to run screensaver: %w", err)
	}

	return nil
}

// Stop stops the screensaver
func (s *Screensaver) Stop() {
	if !s.running {
		return
	}

	slog.Info("Stopping screensaver")
	s.running = false
}

// IsRunning returns whether the screensaver is currently running
func (s *Screensaver) IsRunning() bool {
	return s.running
}

// Game implements ebiten.Game interface
type Game struct {
	faceSource   *text.GoTextFaceSource
	screenWidth  int
	screenHeight int
}

// Update updates the game state
func (g *Game) Update() error {
	ebiten.SetCursorMode(ebiten.CursorModeHidden)

	// Check if Escape key is pressed
	if ebiten.IsKeyPressed(ebiten.KeyEscape) {
		return ebiten.Termination
	}
	return nil
}

// Draw draws the screensaver
func (g *Game) Draw(screen *ebiten.Image) {
	// Fill screen with black
	screen.Fill(color.Black)

	// Get current time in 24h format
	currentTime := time.Now().Format("15:04")

	// Initialize font if needed (use Go's built-in font)
	if g.faceSource == nil {
		s, err := text.NewGoTextFaceSource(bytes.NewReader(goregular.TTF))
		if err != nil {
			slog.Error("Failed to create font face source", "error", err)
			return
		}
		g.faceSource = s
	}

	// Create face with large size for visibility
	face := &text.GoTextFace{
		Source: g.faceSource,
		Size:   fontSize,
	}

	// Calculate text dimensions
	textWidth, textHeight := text.Measure(currentTime, face, 0)

	// Position text at bottom right with padding
	x := float64(g.screenWidth) - textWidth - padding
	y := float64(g.screenHeight) - textHeight - padding

	// Draw the time
	textOp := &text.DrawOptions{}
	textOp.GeoM.Translate(x, y)
	textOp.ColorScale.ScaleWithColor(color.White)
	text.Draw(screen, currentTime, face, textOp)
}

// Layout returns the game's logical screen size
func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	// Store the actual screen dimensions
	g.screenWidth = outsideWidth
	g.screenHeight = outsideHeight
	return outsideWidth, outsideHeight
}
