package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// ThirdPartyClient handles communication with third-party systems
type ThirdPartyClient struct {
	endpoint    string
	httpClient  *http.Client
	connected   bool
	lastSuccess time.Time
	lastError   error
	mutex       sync.RWMutex
}

// NewThirdPartyClient creates a new third-party client
func NewThirdPartyClient(endpoint string) *ThirdPartyClient {
	return &ThirdPartyClient{
		endpoint: endpoint,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		connected: false,
	}
}

// SendLocationUpdate sends location data to third-party system
func (c *ThirdPartyClient) SendLocationUpdate(event LocationEvent) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Prepare payload
	payload := map[string]interface{}{
		"event_type": "location_update",
		"data":       event,
		"timestamp":  time.Now(),
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		c.lastError = fmt.Errorf("failed to marshal location data: %w", err)
		return err
	}

	// Send HTTP request
	req, err := http.NewRequest("POST", c.endpoint+"/location", bytes.NewBuffer(jsonData))
	if err != nil {
		c.lastError = fmt.Errorf("failed to create request: %w", err)
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tenant-ID", event.TenantID)
	req.Header.Set("X-User-ID", event.UserID)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.lastError = fmt.Errorf("failed to send location update: %w", err)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		c.lastError = fmt.Errorf("third-party returned status %d", resp.StatusCode)
		return fmt.Errorf("third-party returned status %d", resp.StatusCode)
	}

	// Update success status
	c.connected = true
	c.lastSuccess = time.Now()
	c.lastError = nil
	return nil
}

// GetStatus returns the current connection status
func (c *ThirdPartyClient) GetStatus() map[string]interface{} {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	return map[string]interface{}{
		"connected":    c.connected,
		"endpoint":     c.endpoint,
		"last_success": c.lastSuccess,
		"last_error":   c.lastError,
	}
}

// Reconnect attempts to reconnect to the third-party system
func (c *ThirdPartyClient) Reconnect() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Test connection with a simple request
	req, err := http.NewRequest("GET", c.endpoint+"/health", nil)
	if err != nil {
		c.lastError = fmt.Errorf("failed to create health check request: %w", err)
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.lastError = fmt.Errorf("health check failed: %w", err)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		c.lastError = fmt.Errorf("health check returned status %d", resp.StatusCode)
		return fmt.Errorf("health check returned status %d", resp.StatusCode)
	}

	c.connected = true
	c.lastSuccess = time.Now()
	c.lastError = nil
	return nil
}
