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

	// Initialize Redis for session management
	if err := utils.InitRedis(); err != nil {
		log.Fatal("Failed to connect to Redis:", err)
	}
	defer utils.CloseRedis()

	// Initialize database
	db, err := config.ConnectDatabase()
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}

	// Initialize Gin router
	router := gin.Default()

	// Health check endpoint
	router.GET("/health", func(c *gin.Context) {
		utils.OKResponse(c, "Auth service is healthy", nil)
	})

	// Authentication routes
	auth := router.Group("/auth")
	{
		auth.POST("/login", handleLogin(db))
		auth.POST("/register", handleRegister(db))
		auth.POST("/refresh", handleRefreshToken(db))
		auth.POST("/logout", handleLogout(db))
	}

	// Start server
	port := os.Getenv("AUTH_SERVICE_PORT")
	if port == "" {
		port = "8001"
	}

	logrus.Infof("Auth service starting on port %s", port)
	if err := router.Run(":" + port); err != nil {
		log.Fatal("Failed to start auth service:", err)
	}
}
