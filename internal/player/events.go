package player

import (
	"fmt"

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
