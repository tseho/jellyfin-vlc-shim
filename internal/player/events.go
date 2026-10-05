package player

import (
	"fmt"
	"log/slog"

	vlc "github.com/adrg/libvlc-go/v3"
)

// ListenPlaybackEndEvents sets up event handlers for when playback ends.
// The returned channel is closed when playback reaches the end, is stopped or fails.
// The handlers are detached by Release.
func (p *Player) ListenPlaybackEndEvents() (<-chan struct{}, error) {
	manager, err := p.player.EventManager()
	if err != nil {
		return nil, fmt.Errorf("failed to get event manager: %w", err)
	}

	eventCallback := func(event vlc.Event, userData interface{}) {
		if event == vlc.MediaPlayerEncounteredError {
			slog.Warn("VLC encountered an error during playback")
		}
		p.markEnded()
	}

	events := []vlc.Event{
		vlc.MediaPlayerEndReached,
		vlc.MediaPlayerStopped,
		vlc.MediaPlayerEncounteredError,
	}
	for _, event := range events {
		eventID, err := manager.Attach(event, eventCallback, nil)
		if err != nil {
			manager.Detach(p.eventIDs...)
			p.eventIDs = nil
			return nil, fmt.Errorf("failed to attach event: %w", err)
		}
		p.eventIDs = append(p.eventIDs, eventID)
	}
	p.eventManager = manager

	return p.ended, nil
}

// markEnded closes the ended channel, at most once
func (p *Player) markEnded() {
	p.endOnce.Do(func() {
		close(p.ended)
	})
}
