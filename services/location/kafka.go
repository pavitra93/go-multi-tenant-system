package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/segmentio/kafka-go"
)

// KafkaProducer handles Kafka message production
type KafkaProducer struct {
	writer *kafka.Writer
}

// NewKafkaProducer creates a new Kafka producer
func NewKafkaProducer(broker string) (*KafkaProducer, error) {
	writer := &kafka.Writer{
		Addr:         kafka.TCP(broker),
		Balancer:     &kafka.LeastBytes{},
		BatchTimeout: 10 * time.Millisecond,
		BatchSize:    100,
	}

	return &KafkaProducer{
		writer: writer,
	}, nil
}

// SendLocationEvent sends a location event to Kafka
func (kp *KafkaProducer) SendLocationEvent(event LocationEvent) error {
	message, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal location event: %w", err)
	}

	msg := kafka.Message{
		Topic: "location-updates",
		Key:   []byte(event.TenantID.String()),
		Value: message,
		Headers: []kafka.Header{
			{Key: "event_type", Value: []byte("location_update")},
			{Key: "tenant_id", Value: []byte(event.TenantID.String())},
			{Key: "cognito_user_id", Value: []byte(event.CognitoUserID)},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := kp.writer.WriteMessages(ctx, msg); err != nil {
		return fmt.Errorf("failed to write location event to Kafka: %w", err)
	}

	return nil
}

// SendSessionEvent sends a session event to Kafka
func (kp *KafkaProducer) SendSessionEvent(event SessionEvent) error {
	message, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal session event: %w", err)
	}

	msg := kafka.Message{
		Topic: "session-events",
		Key:   []byte(event.TenantID.String()),
		Value: message,
		Headers: []kafka.Header{
			{Key: "event_type", Value: []byte("session_event")},
			{Key: "tenant_id", Value: []byte(event.TenantID.String())},
			{Key: "cognito_user_id", Value: []byte(event.CognitoUserID)},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := kp.writer.WriteMessages(ctx, msg); err != nil {
		return fmt.Errorf("failed to write session event to Kafka: %w", err)
	}

	return nil
}

// Close closes the Kafka producer
func (kp *KafkaProducer) Close() error {
	return kp.writer.Close()
}
