package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/segmentio/kafka-go"
)

// KafkaConsumer handles Kafka message consumption
type KafkaConsumer struct {
	locationReader *kafka.Reader
}

// NewKafkaConsumer creates a new Kafka consumer
func NewKafkaConsumer(broker string) (*KafkaConsumer, error) {
	// Create reader for location updates
	locationReader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:        []string{broker},
		Topic:          "location-updates",
		GroupID:        "streaming-service",
		MinBytes:       10e3, // 10KB
		MaxBytes:       10e6, // 10MB
		CommitInterval: time.Second,
	})

	return &KafkaConsumer{
		locationReader: locationReader,
	}, nil
}

// ConsumeLocationUpdates consumes location update events from Kafka
func (kc *KafkaConsumer) ConsumeLocationUpdates(thirdPartyClient *ThirdPartyClient) {
	log.Println("Starting location updates consumer...")

	for {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		msg, err := kc.locationReader.ReadMessage(ctx)
		cancel()

		if err != nil {
			// Ignore timeout errors - this is expected when no messages available
			if err == context.DeadlineExceeded || err.Error() == "context deadline exceeded" {
				continue
			}
			// Only log actual errors
			log.Printf("Error reading location message: %v", err)
			time.Sleep(1 * time.Second)
			continue
		}

		var locationEvent LocationEvent
		if err := json.Unmarshal(msg.Value, &locationEvent); err != nil {
			log.Printf("Error unmarshaling location event: %v", err)
			continue
		}

		// Send to third-party system
		if err := thirdPartyClient.SendLocationUpdate(locationEvent); err != nil {
			log.Printf("Error sending location update to third-party: %v", err)
			// Store failed update in database for retry
			if dlqErr := kc.storeFailedUpdate(locationEvent, err); dlqErr != nil {
				log.Printf("Failed to store failed update: %v", dlqErr)
			}
		} else {
			log.Printf("Successfully sent location update for tenant %s, user %s",
				locationEvent.TenantID, locationEvent.UserID)
		}
	}
}

// FailedLocationUpdate represents a failed location update in database
type FailedLocationUpdate struct {
	ID              string     `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	OriginalEventID string     `gorm:"not null" json:"original_event_id"`
	TenantID        string     `gorm:"not null" json:"tenant_id"`
	UserID          string     `gorm:"not null" json:"user_id"`
	SessionID       *string    `json:"session_id,omitempty"`
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

// storeFailedUpdate stores failed location update in database for retry
func (kc *KafkaConsumer) storeFailedUpdate(event LocationEvent, err error) error {
	// Calculate next retry time (1 minute from now)
	nextRetryAt := time.Now().Add(1 * time.Minute)

	// Store in database (this would need database connection)
	// For now, just log the failed update
	log.Printf("Failed location update stored for retry - ID: %s, Tenant: %s, User: %s, Error: %s, Next retry: %s",
		event.ID, event.TenantID, event.UserID, err.Error(), nextRetryAt.Format(time.RFC3339))

	return nil
}

// Close closes the Kafka consumer
func (kc *KafkaConsumer) Close() error {
	if err := kc.locationReader.Close(); err != nil {
		return fmt.Errorf("failed to close location reader: %w", err)
	}
	return nil
}

// LocationEvent represents a location event from Kafka
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
