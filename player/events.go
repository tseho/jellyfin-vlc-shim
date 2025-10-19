package player

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	vlc "github.com/adrg/libvlc-go/v3"
)

// ListenEndReachedEvent sets up an event handler for when playback ends
func (p *Player) ListenEndReachedEvent() (chan struct{}, error) {
	manager, err := p.player.EventManager()
	if err != nil {
		return nil, fmt.Errorf("failed to get event manager: %w", err)
	}

	// Channel to signal when playback ends
	done := make(chan struct{})

	// Register end reached event
	eventCallback := func(event vlc.Event, userData interface{}) {
		log.Println("Playback finished")
		close(done)
	}

	eventID, err := manager.Attach(vlc.MediaPlayerEndReached, eventCallback, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to attach event: %w", err)
	}

	// Detach event when done channel is closed
	go func() {
		<-done
		manager.Detach(eventID)
	}()

	return done, nil
}

// WaitForEndOrInterrupt waits for playback to finish or for an interrupt signal
func (p *Player) WaitForEndOrInterrupt(done <-chan struct{}) {
	// Handle Ctrl+C to stop playback gracefully
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)

	log.Println("Playing... Press Ctrl+C to stop")

	// Wait for playback to finish or interrupt
	select {
	case <-done:
		log.Println("Video playback completed")
	case <-interrupt:
		log.Println("Stopping playback...")
		p.Stop()
	}
}
