package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/segmentio/kafka-go"
)

// KafkaProducer handles Kafka message production with worker pool
type KafkaProducer struct {
	writer            *kafka.Writer
	locationEventChan chan LocationEvent
	workerCount       int
	shutdownChan      chan struct{}
	wg                sync.WaitGroup
}

// NewKafkaProducer creates a new Kafka producer with worker pool
func NewKafkaProducer(broker string) (*KafkaProducer, error) {
	writer := &kafka.Writer{
		Addr:         kafka.TCP(broker),
		Balancer:     &kafka.LeastBytes{},
		BatchTimeout: 10 * time.Millisecond,
		BatchSize:    100,
	}

	kp := &KafkaProducer{
		writer:            writer,
		locationEventChan: make(chan LocationEvent, 1000), // Buffered channel for 1000 events
		workerCount:       10,                             // 10 worker goroutines
		shutdownChan:      make(chan struct{}),
	}

	// Start worker pool
	kp.startWorkers()

	return kp, nil
}

// startWorkers starts the worker pool for async event processing
func (kp *KafkaProducer) startWorkers() {
	// Location event workers
	for i := 0; i < kp.workerCount; i++ {
		kp.wg.Add(1)
		go kp.locationEventWorker(i)
	}

	fmt.Printf("[Kafka] Started %d location workers\n", kp.workerCount)
}

// locationEventWorker processes location events from the channel
func (kp *KafkaProducer) locationEventWorker(id int) {
	defer kp.wg.Done()

	for {
		select {
		case event := <-kp.locationEventChan:
			if err := kp.sendLocationEventSync(event); err != nil {
				fmt.Printf("[Kafka Worker %d] Failed to send location event: %v\n", id, err)
			}
		case <-kp.shutdownChan:
			fmt.Printf("[Kafka Worker %d] Shutting down location worker\n", id)
			return
		}
	}
}

// SendLocationEvent queues a location event asynchronously (non-blocking)
func (kp *KafkaProducer) SendLocationEvent(event LocationEvent) error {
	select {
	case kp.locationEventChan <- event:
		return nil
	default:
		// Channel full - drop event
		return fmt.Errorf("location event queue full, event dropped")
	}
}

// sendLocationEventSync sends location event to Kafka synchronously (called by workers)
func (kp *KafkaProducer) sendLocationEventSync(event LocationEvent) error {
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

// Close gracefully shuts down the Kafka producer and workers
func (kp *KafkaProducer) Close() error {
	fmt.Println("[Kafka] Initiating graceful shutdown...")

	// Signal all workers to stop
	close(kp.shutdownChan)

	// Wait for all workers to finish processing
	kp.wg.Wait()

	// Close channels
	close(kp.locationEventChan)

	// Close Kafka writer
	if err := kp.writer.Close(); err != nil {
		return fmt.Errorf("failed to close Kafka writer: %w", err)
	}

	fmt.Println("[Kafka] Graceful shutdown complete")
	return nil
}
