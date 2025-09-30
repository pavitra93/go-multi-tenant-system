package main

import (
	"log"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/pavitra93/go-multi-tenant-system/shared/utils"
	"github.com/sirupsen/logrus"
)

func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		logrus.Warn("No .env file found, using environment variables")
	}

	// Initialize Kafka consumer
	kafkaConsumer, err := NewKafkaConsumer(os.Getenv("KAFKA_BROKER"))
	if err != nil {
		log.Fatal("Failed to initialize Kafka consumer:", err)
	}
	defer kafkaConsumer.Close()

	// Initialize third-party client
	thirdPartyClient := NewThirdPartyClient(os.Getenv("THIRD_PARTY_ENDPOINT"))

	// Start Kafka consumers
	go kafkaConsumer.ConsumeLocationUpdates(thirdPartyClient)
	go kafkaConsumer.ConsumeSessionEvents(thirdPartyClient)

	// Initialize Gin router
	router := gin.Default()

	// Health check endpoint
	router.GET("/health", func(c *gin.Context) {
		utils.OKResponse(c, "Streaming service is healthy", nil)
	})

	// Streaming management routes
	streaming := router.Group("/streaming")
	{
		streaming.GET("/status", handleGetStreamingStatus(thirdPartyClient))
		streaming.POST("/reconnect", handleReconnectThirdParty(thirdPartyClient))
		streaming.GET("/metrics", handleGetStreamingMetrics(thirdPartyClient))
	}

	// Start server
	port := os.Getenv("STREAMING_SERVICE_PORT")
	if port == "" {
		port = "8004"
	}

	logrus.Infof("Streaming service starting on port %s", port)
	if err := router.Run(":" + port); err != nil {
		log.Fatal("Failed to start streaming service:", err)
	}
}
