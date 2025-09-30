package main

import (
	"log"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/pavitra93/go-multi-tenant-system/shared/config"
	"github.com/pavitra93/go-multi-tenant-system/shared/middleware"
	"github.com/pavitra93/go-multi-tenant-system/shared/utils"
	"github.com/sirupsen/logrus"
)

func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		logrus.Warn("No .env file found, using environment variables")
	}

	// Initialize Redis for session caching
	if err := utils.InitRedis(); err != nil {
		log.Fatal("Failed to connect to Redis:", err)
	}
	defer utils.CloseRedis()

	// Initialize database
	db, err := config.ConnectDatabase()
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}

	// Initialize authentication middleware
	authMiddleware, err := middleware.NewAuthMiddleware(
		os.Getenv("AWS_REGION"),
		os.Getenv("COGNITO_USER_POOL_ID"),
	)
	if err != nil {
		log.Fatal("Failed to initialize auth middleware:", err)
	}

	// Initialize Kafka producer
	kafkaProducer, err := NewKafkaProducer(os.Getenv("KAFKA_BROKER"))
	if err != nil {
		log.Fatal("Failed to initialize Kafka producer:", err)
	}
	defer kafkaProducer.Close()

	// Initialize Gin router
	router := gin.Default()

	// Health check endpoint
	router.GET("/health", func(c *gin.Context) {
		utils.OKResponse(c, "Location service is healthy", nil)
	})

	// Location tracking routes
	location := router.Group("/location")
	location.Use(authMiddleware.RequireAuth())
	{
		// Session management
		location.POST("/session/start", handleStartSession(db, kafkaProducer))
		location.POST("/session/:id/stop", handleStopSession(db, kafkaProducer))
		location.GET("/session/:id", handleGetSession(db))
		location.GET("/sessions", handleGetUserSessions(db))

		// Location data submission
		location.POST("/update", handleLocationUpdate(db, kafkaProducer))
		location.GET("/session/:id/locations", handleGetSessionLocations(db))
	}

	// Start server
	port := os.Getenv("LOCATION_SERVICE_PORT")
	if port == "" {
		port = "8003"
	}

	logrus.Infof("Location service starting on port %s", port)
	if err := router.Run(":" + port); err != nil {
		log.Fatal("Failed to start location service:", err)
	}
}
