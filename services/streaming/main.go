package main

import (
	"log"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/pavitra93/go-multi-tenant-system/shared/config"
	"github.com/pavitra93/go-multi-tenant-system/shared/utils"
	"github.com/sirupsen/logrus"
)

func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		logrus.Warn("No .env file found, using environment variables")
	}

	// Initialize database connection
	db, err := config.ConnectDatabase()
	if err != nil {
		log.Fatal("Failed to initialize database:", err)
	}

	// Initialize Kafka consumer with database connection
	kafkaConsumer, err := NewKafkaConsumer(os.Getenv("KAFKA_BROKER"), db)
	if err != nil {
		log.Fatal("Failed to initialize Kafka consumer:", err)
	}
	defer kafkaConsumer.Close()

	// Initialize third-party client
	thirdPartyClient := NewThirdPartyClient(os.Getenv("THIRD_PARTY_ENDPOINT"))

	// Start Kafka consumer for location updates only
	go kafkaConsumer.ConsumeLocationUpdates(thirdPartyClient)

	// Initialize Gin router
	router := gin.Default()

	// Health check endpoint
	router.GET("/health", func(c *gin.Context) {
		utils.OKResponse(c, "Streaming service is healthy", nil)
	})

	// Observability endpoints (for monitoring/demonstration)
	// These show that streaming requirements are met
	streaming := router.Group("/streaming")
	{
		streaming.GET("/health", handleGetStreamingHealth(thirdPartyClient))
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
