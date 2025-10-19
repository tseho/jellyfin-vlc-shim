package jellyfin

// AuthRequest is the request payload for authentication
type AuthRequest struct {
	Username string `json:"Username"`
	Pw       string `json:"Pw"`
}

// AuthenticationResult is the response from authentication
type AuthenticationResult struct {
	User        User   `json:"User"`
	AccessToken string `json:"AccessToken"`
	ServerId    string `json:"ServerId"`
}

// User represents a Jellyfin user
type User struct {
	Name string `json:"Name"`
	Id   string `json:"Id"`
}

// Capabilities defines what the client can do
type Capabilities struct {
	PlayableMediaTypes   string `json:"PlayableMediaTypes"`
	SupportsMediaControl bool   `json:"SupportsMediaControl"`
	SupportedCommands    string `json:"SupportedCommands"`
}

// ItemInfo contains information about a media item
type ItemInfo struct {
	Id           string        `json:"Id"`
	Name         string        `json:"Name"`
	Type         string        `json:"Type"`
	MediaSources []MediaSource `json:"MediaSources"`
}

// MediaSource represents a media source for an item
type MediaSource struct {
	Id                 string `json:"Id"`
	Protocol           string `json:"Protocol"`
	SupportsDirectPlay bool   `json:"SupportsDirectPlay"`
}

// WebSocketMessage is a message received from the Jellyfin WebSocket
type WebSocketMessage struct {
	MessageType string      `json:"MessageType"`
	Data        interface{} `json:"Data,omitempty"`
}

// PlayCommandData contains data for a Play command
type PlayCommandData struct {
	ControllingUserId string   `json:"ControllingUserId"`
	ItemIds           []string `json:"ItemIds"`
	PlayCommand       string   `json:"PlayCommand"`
}

// PlaystateCommandData contains data for playstate commands
type PlaystateCommandData struct {
	Command            string `json:"Command"`
	SeekPositionTicks  int64  `json:"SeekPositionTicks,omitempty"`
	ControllingUserId  string `json:"ControllingUserId,omitempty"`
}
