package jellyfin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// Client represents a Jellyfin API client
type Client struct {
	ServerURL   string
	AccessToken string
	UserID      string
	DeviceID    string
	ClientName  string
	DeviceName  string
}

// NewClient creates a new Jellyfin client
func NewClient(serverURL, accessToken, userID, deviceID, clientName, deviceName string) *Client {
	return &Client{
		ServerURL:   serverURL,
		AccessToken: accessToken,
		UserID:      userID,
		DeviceID:    deviceID,
		ClientName:  clientName,
		DeviceName:  deviceName,
	}
}

// makeAuthHeader creates the X-Emby-Authorization header
func (c *Client) makeAuthHeader() string {
	if c.AccessToken != "" {
		return fmt.Sprintf(
			`MediaBrowser Client="%s", Device="%s", DeviceId="%s", Version="0.0.1", Token="%s"`,
			c.ClientName, c.DeviceName, c.DeviceID, c.AccessToken,
		)
	}
	return fmt.Sprintf(
		`MediaBrowser Client="%s", Device="%s", DeviceId="%s", Version="0.0.1"`,
		c.ClientName, c.DeviceName, c.DeviceID,
	)
}

// GetItemInfo retrieves information about a media item
func (c *Client) GetItemInfo(itemID string) (*ItemInfo, error) {
	endpoint := fmt.Sprintf("%s/Users/%s/Items/%s", c.ServerURL, c.UserID, itemID)

	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", c.makeAuthHeader())

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get item info with status %d: %s", resp.StatusCode, string(body))
	}

	var itemInfo ItemInfo
	if err := json.Unmarshal(body, &itemInfo); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &itemInfo, nil
}

// GetVideoDirectStreamURL returns the direct stream URL for an item
func (c *Client) GetVideoDirectStreamURL(itemID string) string {
	return fmt.Sprintf("%s/Videos/%s/stream?static=true&DeviceId=%s&api_key=%s",
		c.ServerURL, itemID, c.DeviceID, c.AccessToken)
}

// GetSubtitleDownloadURL returns the direct stream URL for a subtitle
func (c *Client) GetSubtitleDownloadURL(itemID, mediaSourceID string, streamIndex int) string {
	return fmt.Sprintf("%s/Videos/%s/%s/Subtitles/%d/Stream.srt?DeviceId=%s&api_key=%s",
		c.ServerURL, itemID, mediaSourceID, streamIndex, c.DeviceID, c.AccessToken)
}

// GetSubtitleInfo returns information about the requested subtitle
func (c *Client) GetSubtitleInfo(playData PlayCommandData, itemInfo *ItemInfo) *SubtitleInfo {
	if playData.SubtitleStreamIndex == nil {
		return nil
	}

	subtitleIndex := *playData.SubtitleStreamIndex
	if subtitleIndex < 0 {
		return nil
	}

	// Find the media source
	var mediaSource *MediaSource
	if playData.MediaSourceId != "" {
		for i := range itemInfo.MediaSources {
			if itemInfo.MediaSources[i].Id == playData.MediaSourceId {
				mediaSource = &itemInfo.MediaSources[i]
				break
			}
		}
	} else if len(itemInfo.MediaSources) > 0 {
		mediaSource = &itemInfo.MediaSources[0]
	}

	if mediaSource == nil {
		return nil
	}

	// Find the subtitle stream
	for _, stream := range mediaSource.MediaStreams {
		if stream.Type == "Subtitle" && stream.Index == subtitleIndex {
			var url *string
			if stream.IsExternal {
				urlStr := c.GetSubtitleDownloadURL(itemInfo.Id, mediaSource.Id, stream.Index)
				url = &urlStr
			}

			return &SubtitleInfo{
				Index:    stream.Index,
				External: stream.IsExternal,
				URL:      url,
			}
		}
	}

	return nil
}

// RegisterCapabilities registers the client capabilities with the server
func (c *Client) RegisterCapabilities() error {
	caps := Capabilities{
		PlayableMediaTypes:   "Video",
		SupportsMediaControl: true,
		SupportedCommands: strings.Join([]string{
			// "MoveUp",
			// "MoveDown",
			// "MoveLeft",
			// "MoveRight",
			// "PageUp",
			// "PageDown",
			// "PreviousLetter",
			// "NextLetter",
			// "ToggleOsd",
			// "ToggleContextMenu",
			// "Select",
			// "Back",
			// "TakeScreenshot",
			// "SendKey",
			// "SendString",
			// "GoHome",
			// "GoToSettings",
			// "VolumeUp",
			// "VolumeDown",
			// "Mute",
			// "Unmute",
			// "ToggleMute",
			// "SetVolume",
			// "SetAudioStreamIndex",
			// "SetSubtitleStreamIndex",
			// "ToggleFullscreen",
			"DisplayContent",
			// "GoToSearch",
			// "DisplayMessage",
			// "SetRepeatMode",
			// "ChannelUp",
			// "ChannelDown",
			// "Guide",
			// "ToggleStats",
			"PlayMediaSource",
			// "PlayTrailers",
			// "SetShuffleQueue",
			"PlayState",
			// "PlayNext",
			// "ToggleOsdMenu",
			"Play",
			// "SetMaxStreamingBitrate",
			// "SetPlaybackOrder",
		}, ","),
	}

	jsonData, err := json.Marshal(caps)
	if err != nil {
		return fmt.Errorf("failed to marshal capabilities: %w", err)
	}

	endpoint := fmt.Sprintf("%s/Sessions/Capabilities/Full", c.ServerURL)
	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", c.makeAuthHeader())

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to register capabilities with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// ReportPlaybackStart reports playback start to the server
func (c *Client) ReportPlaybackStart(itemID string, positionTicks int64) error {
	data := map[string]interface{}{
		"ItemId":        itemID,
		"PositionTicks": positionTicks,
		"IsPaused":      false,
		"IsMuted":       false,
		"VolumeLevel":   100,
		"PlayMethod":    "DirectPlay",
		"CanSeek":       true,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal playback start data: %w", err)
	}

	endpoint := fmt.Sprintf("%s/Sessions/Playing", c.ServerURL)
	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", c.makeAuthHeader())

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to report playback start with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// ReportPlaybackProgress reports playback progress to the server
func (c *Client) ReportPlaybackProgress(itemID string, positionTicks int64, isPaused bool) error {
	data := map[string]interface{}{
		"ItemId":        itemID,
		"PositionTicks": positionTicks,
		"IsPaused":      isPaused,
		"IsMuted":       false,
		"VolumeLevel":   100,
		"PlayMethod":    "DirectPlay",
		"CanSeek":       true,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal playback progress data: %w", err)
	}

	endpoint := fmt.Sprintf("%s/Sessions/Playing/Progress", c.ServerURL)
	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", c.makeAuthHeader())

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to report playback progress with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// ReportPlaybackStopped reports playback stopped to the server
func (c *Client) ReportPlaybackStopped(itemID string, positionTicks int64) error {
	data := map[string]interface{}{
		"ItemId":        itemID,
		"PositionTicks": positionTicks,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal playback stopped data: %w", err)
	}

	endpoint := fmt.Sprintf("%s/Sessions/Playing/Stopped", c.ServerURL)
	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", c.makeAuthHeader())

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to report playback stopped with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// MessageHandler is a callback function for handling WebSocket messages
type MessageHandler func(msg WebSocketMessage) error

// ConnectWebSocket connects to the Jellyfin WebSocket and handles messages
func (c *Client) ConnectWebSocket(ctx context.Context, handler MessageHandler) error {
	// Convert HTTP(S) URL to WS(S)
	u, err := url.Parse(c.ServerURL)
	if err != nil {
		return fmt.Errorf("failed to parse server URL: %w", err)
	}

	wsScheme := "ws"
	if u.Scheme == "https" {
		wsScheme = "wss"
	}

	wsURL := fmt.Sprintf("%s://%s/socket?api_key=%s&device_id=%s", wsScheme, u.Host, c.AccessToken, c.DeviceID)

	log.Printf("Connecting to WebSocket: %s", wsURL)

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to WebSocket: %w", err)
	}
	defer conn.Close()

	log.Println("WebSocket connected successfully!")
	log.Println("Listening for messages from Jellyfin server...")

	done := make(chan struct{})

	// Read messages
	go func() {
		defer close(done)
		for {
			var msg WebSocketMessage
			err := conn.ReadJSON(&msg)
			if err != nil {
				log.Printf("Error reading message: %v", err)
				return
			}

			// Pretty print the message
			jsonData, err := json.MarshalIndent(msg, "", "  ")
			if err != nil {
				log.Printf("Error marshaling message: %v", err)
				continue
			}

			// Handle different message types
			switch msg.MessageType {
			case "ForceKeepAlive":
				//log.Println("Sending KeepAlive response...")
				keepAlive := map[string]string{
					"MessageType": "KeepAlive",
				}
				if err := conn.WriteJSON(keepAlive); err != nil {
					log.Printf("Error sending keep-alive: %v", err)
					return
				}

				// Start periodic keep-alive
				if dataMap, ok := msg.Data.(map[string]interface{}); ok {
					if timeoutSec, ok := dataMap["Timeout"].(float64); ok {
						go func() {
							ticker := time.NewTicker(time.Duration(timeoutSec/2) * time.Second)
							defer ticker.Stop()

							for {
								select {
								case <-ticker.C:
									keepAlive := map[string]string{
										"MessageType": "KeepAlive",
									}
									if err := conn.WriteJSON(keepAlive); err != nil {
										log.Printf("Error sending periodic keep-alive: %v", err)
										return
									}
									log.Println("Sent periodic KeepAlive")
								case <-done:
									return
								}
							}
						}()
					}
				}
			case "KeepAlive":
				continue

			default:
				log.Printf("Received message:\n%s\n", string(jsonData))
				// Call the handler for other message types
				if err := handler(msg); err != nil {
					log.Printf("Error handling message: %v", err)
				}
			}
		}
	}()

	// Wait for either context cancellation or connection close
	select {
	case <-ctx.Done():
		log.Println("Closing WebSocket...")
		// Send close message gracefully
		err := conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		if err != nil {
			log.Printf("Error sending close message: %v", err)
		}
		return nil
	case <-done:
		return nil
	}
}
