package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/pavitra93/go-multi-tenant-system/shared/utils"
)

// ServiceClient handles HTTP communication with microservices
type ServiceClient struct {
	baseURL    string
	httpClient *http.Client
}

// ServiceClients holds all service clients
type ServiceClients struct {
	AuthService      *ServiceClient
	TenantService    *ServiceClient
	LocationService  *ServiceClient
	StreamingService *ServiceClient
}

// NewServiceClient creates a new service client
func NewServiceClient(baseURL string) *ServiceClient {
	return &ServiceClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// ProxyRequest proxies requests to the appropriate microservice
func (sc *ServiceClient) ProxyRequest(c *gin.Context) {
	// Build target URL
	targetURL := sc.baseURL + c.Request.URL.Path
	if c.Request.URL.RawQuery != "" {
		targetURL += "?" + c.Request.URL.RawQuery
	}

	// Create request
	var body io.Reader
	if c.Request.Body != nil {
		bodyBytes, err := io.ReadAll(c.Request.Body)
		if err != nil {
			utils.InternalServerErrorResponse(c, "Failed to read request body")
			return
		}
		body = bytes.NewBuffer(bodyBytes)
	}

	req, err := http.NewRequest(c.Request.Method, targetURL, body)
	if err != nil {
		utils.InternalServerErrorResponse(c, "Failed to create request")
		return
	}

	// Copy headers
	for key, values := range c.Request.Header {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}

	// Add user context headers
	if userID, exists := c.Get("user_id"); exists {
		req.Header.Set("X-User-ID", userID.(string))
	}
	if email, exists := c.Get("email"); exists {
		req.Header.Set("X-User-Email", email.(string))
	}
	if tenantID, exists := c.Get("tenant_id"); exists {
		req.Header.Set("X-Tenant-ID", tenantID.(string))
	}
	if role, exists := c.Get("role"); exists {
		req.Header.Set("X-User-Role", role.(string))
	}

	// Send request
	resp, err := sc.httpClient.Do(req)
	if err != nil {
		utils.InternalServerErrorResponse(c, "Failed to communicate with service")
		return
	}
	defer resp.Body.Close()

	// Read response body
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		utils.InternalServerErrorResponse(c, "Failed to read response")
		return
	}

	// Copy response headers
	for key, values := range resp.Header {
		for _, value := range values {
			c.Header(key, value)
		}
	}

	// Set status and return response
	c.Data(resp.StatusCode, resp.Header.Get("Content-Type"), responseBody)
}

// HealthCheck checks if a service is healthy
func (sc *ServiceClient) HealthCheck() error {
	req, err := http.NewRequest("GET", sc.baseURL+"/health", nil)
	if err != nil {
		return fmt.Errorf("failed to create health check request: %w", err)
	}

	resp, err := sc.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("health check request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("service returned status %d", resp.StatusCode)
	}

	return nil
}

// GetServiceStatus returns the status of all services
func (scs *ServiceClients) GetServiceStatus() map[string]interface{} {
	status := make(map[string]interface{})

	// Check auth service
	if err := scs.AuthService.HealthCheck(); err != nil {
		status["auth_service"] = map[string]interface{}{
			"healthy": false,
			"error":   err.Error(),
		}
	} else {
		status["auth_service"] = map[string]interface{}{
			"healthy": true,
		}
	}

	// Check tenant service
	if err := scs.TenantService.HealthCheck(); err != nil {
		status["tenant_service"] = map[string]interface{}{
			"healthy": false,
			"error":   err.Error(),
		}
	} else {
		status["tenant_service"] = map[string]interface{}{
			"healthy": true,
		}
	}

	// Check location service
	if err := scs.LocationService.HealthCheck(); err != nil {
		status["location_service"] = map[string]interface{}{
			"healthy": false,
			"error":   err.Error(),
		}
	} else {
		status["location_service"] = map[string]interface{}{
			"healthy": true,
		}
	}

	// Check streaming service (optional - background worker)
	if err := scs.StreamingService.HealthCheck(); err != nil {
		status["streaming_service"] = map[string]interface{}{
			"healthy": false,
			"error":   err.Error(),
			"note":    "Background Kafka consumer",
		}
	} else {
		status["streaming_service"] = map[string]interface{}{
			"healthy": true,
			"note":    "Background Kafka consumer",
		}
	}

	return status
}
