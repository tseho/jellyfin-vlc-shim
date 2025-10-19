package jellyfin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Authenticate authenticates with the Jellyfin server and returns the authentication result
func Authenticate(serverURL, username, password, deviceID, clientName, deviceName string) (*AuthenticationResult, error) {
	authReq := AuthRequest{
		Username: username,
		Pw:       password,
	}

	jsonData, err := json.Marshal(authReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	endpoint := fmt.Sprintf("%s/Users/AuthenticateByName", serverURL)
	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Emby-Authorization", fmt.Sprintf(`MediaBrowser Client="%s", Device="%s", DeviceId="%s", Version="0.0.1"`, clientName, deviceName, deviceID))

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
		return nil, fmt.Errorf("authentication failed with status %d: %s", resp.StatusCode, string(body))
	}

	var authResult AuthenticationResult
	if err := json.Unmarshal(body, &authResult); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &authResult, nil
}
