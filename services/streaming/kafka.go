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
	sessionReader  *kafka.Reader
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

	// Create reader for session events
	sessionReader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:        []string{broker},
		Topic:          "session-events",
		GroupID:        "streaming-service",
		MinBytes:       10e3, // 10KB
		MaxBytes:       10e6, // 10MB
		CommitInterval: time.Second,
	})

	return &KafkaConsumer{
		locationReader: locationReader,
		sessionReader:  sessionReader,
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
			// Implement retry logic here
			go kc.retryLocationUpdate(locationEvent, thirdPartyClient)
		} else {
			log.Printf("Successfully sent location update for tenant %s, user %s",
				locationEvent.TenantID, locationEvent.UserID)
		}
	}
}

// ConsumeSessionEvents consumes session events from Kafka
func (kc *KafkaConsumer) ConsumeSessionEvents(thirdPartyClient *ThirdPartyClient) {
	log.Println("Starting session events consumer...")

	for {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		msg, err := kc.sessionReader.ReadMessage(ctx)
		cancel()

		if err != nil {
			// Ignore timeout errors - this is expected when no messages available
			if err == context.DeadlineExceeded || err.Error() == "context deadline exceeded" {
				continue
			}
			// Only log actual errors
			log.Printf("Error reading session message: %v", err)
			time.Sleep(1 * time.Second)
			continue
		}

		var sessionEvent SessionEvent
		if err := json.Unmarshal(msg.Value, &sessionEvent); err != nil {
			log.Printf("Error unmarshaling session event: %v", err)
			continue
		}

		// Send to third-party system
		if err := thirdPartyClient.SendSessionEvent(sessionEvent); err != nil {
			log.Printf("Error sending session event to third-party: %v", err)
			// Implement retry logic here
			go kc.retrySessionEvent(sessionEvent, thirdPartyClient)
		} else {
			log.Printf("Successfully sent session event for tenant %s, user %s",
				sessionEvent.TenantID, sessionEvent.UserID)
		}
	}
}

// retryLocationUpdate retries sending location update with exponential backoff
func (kc *KafkaConsumer) retryLocationUpdate(event LocationEvent, client *ThirdPartyClient) {
	maxRetries := 3
	baseDelay := 1 * time.Second

	for i := 0; i < maxRetries; i++ {
		delay := baseDelay * time.Duration(1<<i) // Exponential backoff
		time.Sleep(delay)

		if err := client.SendLocationUpdate(event); err != nil {
			log.Printf("Retry %d failed for location update: %v", i+1, err)
			if i == maxRetries-1 {
				log.Printf("Max retries reached for location update, giving up")
			}
		} else {
			log.Printf("Location update retry %d successful", i+1)
			return
		}
	}
}

// retrySessionEvent retries sending session event with exponential backoff
func (kc *KafkaConsumer) retrySessionEvent(event SessionEvent, client *ThirdPartyClient) {
	maxRetries := 3
	baseDelay := 1 * time.Second

	for i := 0; i < maxRetries; i++ {
		delay := baseDelay * time.Duration(1<<i) // Exponential backoff
		time.Sleep(delay)

		if err := client.SendSessionEvent(event); err != nil {
			log.Printf("Retry %d failed for session event: %v", i+1, err)
			if i == maxRetries-1 {
				log.Printf("Max retries reached for session event, giving up")
			}
		} else {
			log.Printf("Session event retry %d successful", i+1)
			return
		}
	}
}

// Close closes the Kafka consumer
func (kc *KafkaConsumer) Close() error {
	if err := kc.locationReader.Close(); err != nil {
		return fmt.Errorf("failed to close location reader: %w", err)
	}
	if err := kc.sessionReader.Close(); err != nil {
		return fmt.Errorf("failed to close session reader: %w", err)
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

// SessionEvent represents a session event from Kafka
type SessionEvent struct {
	ID        string     `json:"id"`
	TenantID  string     `json:"tenant_id"`
	UserID    string     `json:"user_id"`
	Status    string     `json:"status"`
	StartedAt time.Time  `json:"started_at"`
	EndedAt   *time.Time `json:"ended_at,omitempty"`
	Duration  int        `json:"duration"`
	EventType string     `json:"event_type"`
}
