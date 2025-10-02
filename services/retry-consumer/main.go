package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// FailedLocationUpdate represents a failed location update in database
type FailedLocationUpdate struct {
	ID              uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	OriginalEventID string     `gorm:"not null" json:"original_event_id"`
	TenantID        uuid.UUID  `gorm:"type:uuid;not null" json:"tenant_id"`
	UserID          string     `gorm:"not null" json:"user_id"`
	SessionID       *uuid.UUID `gorm:"type:uuid" json:"session_id,omitempty"`
	Latitude        *float64   `json:"latitude,omitempty"`
	Longitude       *float64   `json:"longitude,omitempty"`
	ErrorMessage    string     `gorm:"not null" json:"error_message"`
	RetryCount      int        `gorm:"default:0" json:"retry_count"`
	Status          string     `gorm:"default:'pending'" json:"status"`
	NextRetryAt     *time.Time `json:"next_retry_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	ResolvedAt      *time.Time `json:"resolved_at,omitempty"`
}

// LocationEvent represents a location event for retry
type LocationEvent struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	UserID    string    `json:"user_id"`
	SessionID string    `json:"session_id"`
	Latitude  float64   `json:"latitude"`
	Longitude float64   `json:"longitude"`
	Timestamp time.Time `json:"timestamp"`
	EventType string    `json:"event_type"`
}

// RetryConsumer handles retry of failed location updates
type RetryConsumer struct {
	db            *gorm.DB
	thirdPartyURL string
	httpClient    *http.Client
	maxRetries    int
	batchSize     int
	checkInterval time.Duration
}

// NewRetryConsumer creates a new retry consumer
func NewRetryConsumer() (*RetryConsumer, error) {
	// Initialize database connection
	dbHost := os.Getenv("DB_HOST")
	if dbHost == "" {
		dbHost = "localhost"
	}
	dbPort := os.Getenv("DB_PORT")
	if dbPort == "" {
		dbPort = "5432"
	}
	dbUser := os.Getenv("DB_USER")
	if dbUser == "" {
		dbUser = "postgres"
	}
	dbPassword := os.Getenv("DB_PASSWORD")
	if dbPassword == "" {
		dbPassword = "password"
	}
	dbName := os.Getenv("DB_NAME")
	if dbName == "" {
		dbName = "multi_tenant_db"
	}

	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		dbHost, dbPort, dbUser, dbPassword, dbName)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Auto-migrate the failed location updates table
	if err := db.AutoMigrate(&FailedLocationUpdate{}); err != nil {
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	thirdPartyURL := os.Getenv("THIRD_PARTY_ENDPOINT")
	if thirdPartyURL == "" {
		thirdPartyURL = "http://httpbin.org/post"
	}

	return &RetryConsumer{
		db:            db,
		thirdPartyURL: thirdPartyURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		maxRetries:    8,
		batchSize:     100,
		checkInterval: 30 * time.Second,
	}, nil
}

// ProcessFailedUpdates processes failed location updates for retry
func (rc *RetryConsumer) ProcessFailedUpdates() {
	log.Println("Starting retry consumer...")

	for {
		// Get pending failed updates ready for retry
		var failedUpdates []FailedLocationUpdate
		err := rc.db.Where("status = ? AND next_retry_at <= ?", "pending", time.Now()).
			Order("created_at DESC"). // Latest location updates first
			Limit(rc.batchSize).
			Find(&failedUpdates).Error

		if err != nil {
			log.Printf("Error fetching failed updates: %v", err)
			time.Sleep(rc.checkInterval)
			continue
		}

		if len(failedUpdates) == 0 {
			log.Println("No failed updates to retry")
			time.Sleep(rc.checkInterval)
			continue
		}

		log.Printf("Processing %d failed updates for retry", len(failedUpdates))

		for _, failed := range failedUpdates {
			if err := rc.retryFailedUpdate(failed); err != nil {
				log.Printf("Failed to retry update %s: %v", failed.ID, err)
			}
		}

		time.Sleep(rc.checkInterval)
	}
}

// retryFailedUpdate retries a single failed location update
func (rc *RetryConsumer) retryFailedUpdate(failed FailedLocationUpdate) error {
	// Check if session is still active (if session_id exists)
	if failed.SessionID != nil {
		var sessionStatus string
		err := rc.db.Table("location_sessions").
			Select("status").
			Where("id = ?", failed.SessionID).
			Scan(&sessionStatus).Error

		if err != nil {
			// Session not found or error - mark as permanently failed
			log.Printf("Session %s not found or error checking status: %v", failed.SessionID, err)
			return rc.markPermanentlyFailed(failed, "Session not found or inactive")
		}

		if sessionStatus != "active" {
			// Session is not active - mark as permanently failed
			log.Printf("Session %s is not active (status: %s) - marking as permanently failed", failed.SessionID, sessionStatus)
			return rc.markPermanentlyFailed(failed, fmt.Sprintf("Session inactive (status: %s)", sessionStatus))
		}
	}

	// Create location event from failed update
	event := LocationEvent{
		ID:        failed.OriginalEventID,
		TenantID:  failed.TenantID.String(),
		UserID:    failed.UserID,
		SessionID: "",
		Latitude:  0,
		Longitude: 0,
		Timestamp: time.Now(),
		EventType: "location_update",
	}

	if failed.SessionID != nil {
		event.SessionID = failed.SessionID.String()
	}
	if failed.Latitude != nil {
		event.Latitude = *failed.Latitude
	}
	if failed.Longitude != nil {
		event.Longitude = *failed.Longitude
	}

	// Try to send to third-party
	if err := rc.sendToThirdParty(event); err != nil {
		// Update retry count and next retry time
		return rc.updateRetryStatus(failed, err)
	}

	// Success - mark as resolved
	return rc.markResolved(failed)
}

// sendToThirdParty sends location event to third-party system
func (rc *RetryConsumer) sendToThirdParty(event LocationEvent) error {
	// Prepare payload
	payload := map[string]interface{}{
		"event_type": "location_update",
		"data":       event,
		"timestamp":  time.Now(),
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal location data: %w", err)
	}

	// Send HTTP request
	req, err := http.NewRequest("POST", rc.thirdPartyURL+"/location", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tenant-ID", event.TenantID)
	req.Header.Set("X-User-ID", event.UserID)

	resp, err := rc.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send location update: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("third-party returned status %d", resp.StatusCode)
	}

	return nil
}

// updateRetryStatus updates retry count and next retry time
func (rc *RetryConsumer) updateRetryStatus(failed FailedLocationUpdate, err error) error {
	failed.RetryCount++
	failed.UpdatedAt = time.Now()

	if failed.RetryCount >= rc.maxRetries {
		// Mark as permanently failed
		failed.Status = "permanently_failed"
		now := time.Now()
		failed.ResolvedAt = &now
		failed.ErrorMessage = fmt.Sprintf("Max retries reached: %s", err.Error())
	} else {
		// Calculate next retry time with exponential backoff
		baseDelay := 1 * time.Minute
		delay := baseDelay * time.Duration(1<<(failed.RetryCount-1)) // 1m, 2m, 4m, 8m, 16m
		nextRetryAt := time.Now().Add(delay)
		failed.NextRetryAt = &nextRetryAt
		failed.ErrorMessage = err.Error()
	}

	return rc.db.Save(&failed).Error
}

// markResolved marks a failed update as resolved
func (rc *RetryConsumer) markResolved(failed FailedLocationUpdate) error {
	now := time.Now()
	failed.Status = "resolved"
	failed.UpdatedAt = now
	failed.ResolvedAt = &now

	return rc.db.Save(&failed).Error
}

// markPermanentlyFailed marks a failed update as permanently failed (no more retries)
func (rc *RetryConsumer) markPermanentlyFailed(failed FailedLocationUpdate, reason string) error {
	now := time.Now()
	failed.Status = "permanently_failed"
	failed.UpdatedAt = now
	failed.ResolvedAt = &now
	failed.ErrorMessage = reason

	return rc.db.Save(&failed).Error
}

// GetRetryStats returns retry statistics
func (rc *RetryConsumer) GetRetryStats() map[string]interface{} {
	var stats struct {
		Pending           int64 `json:"pending"`
		Retried           int64 `json:"retried"`
		Resolved          int64 `json:"resolved"`
		PermanentlyFailed int64 `json:"permanently_failed"`
	}

	rc.db.Model(&FailedLocationUpdate{}).Where("status = ?", "pending").Count(&stats.Pending)
	rc.db.Model(&FailedLocationUpdate{}).Where("status = ?", "retried").Count(&stats.Retried)
	rc.db.Model(&FailedLocationUpdate{}).Where("status = ?", "resolved").Count(&stats.Resolved)
	rc.db.Model(&FailedLocationUpdate{}).Where("status = ?", "permanently_failed").Count(&stats.PermanentlyFailed)

	return map[string]interface{}{
		"retry_stats": stats,
		"config": map[string]interface{}{
			"max_retries":    rc.maxRetries,
			"batch_size":     rc.batchSize,
			"check_interval": rc.checkInterval.String(),
		},
	}
}

func main() {
	// Initialize retry consumer
	retryConsumer, err := NewRetryConsumer()
	if err != nil {
		log.Fatal("Failed to create retry consumer:", err)
	}

	// Initialize Gin router
	router := gin.Default()

	// Health check endpoint
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":  "healthy",
			"service": "retry-consumer",
		})
	})

	// Retry statistics endpoint
	router.GET("/stats", func(c *gin.Context) {
		stats := retryConsumer.GetRetryStats()
		c.JSON(200, gin.H{
			"success": true,
			"data":    stats,
		})
	})

	// Start retry consumer in background
	go retryConsumer.ProcessFailedUpdates()

	// Start HTTP server
	port := os.Getenv("RETRY_CONSUMER_PORT")
	if port == "" {
		port = "8085"
	}

	logrus.Infof("Retry Consumer starting on port %s", port)
	if err := router.Run(":" + port); err != nil {
		log.Fatal("Failed to start Retry Consumer:", err)
	}
}
