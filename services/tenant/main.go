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

	// Initialize authentication middleware
	authMiddleware, err := middleware.NewAuthMiddleware(
		os.Getenv("AWS_REGION"),
		os.Getenv("COGNITO_USER_POOL_ID"),
	)
	if err != nil {
		log.Fatal("Failed to initialize auth middleware:", err)
	}

	// Initialize Gin router
	router := gin.Default()

	// Health check endpoint
	router.GET("/health", func(c *gin.Context) {
		utils.OKResponse(c, "Tenant service is healthy", nil)
	})

	// Tenant management routes
	tenants := router.Group("/tenants")
	tenants.Use(authMiddleware.RequireAuth())
	{
		// Admin-only routes (platform management)
		tenants.POST("/", authMiddleware.RequireRole("admin"), handleCreateTenant(db))
		tenants.GET("/", authMiddleware.RequireRole("admin"), handleGetTenants(db))

		// Tenant-specific routes
		tenants.GET("/:id", authMiddleware.RequireTenantAccess(), handleGetTenant(db))
		tenants.PUT("/:id", authMiddleware.RequireTenantOwnerOrAdmin(), handleUpdateTenant(db))

		// Tenant user management (tenant owner can manage their users)
		tenants.GET("/:id/users", authMiddleware.RequireTenantOwnerOrAdmin(), handleGetTenantUsers(db))
		tenants.POST("/:id/users", authMiddleware.RequireTenantOwnerOrAdmin(), handleInviteUserToTenant(db))
	}

	// Start server
	port := os.Getenv("TENANT_SERVICE_PORT")
	if port == "" {
		port = "8002"
	}

	logrus.Infof("Tenant service starting on port %s", port)
	if err := router.Run(":" + port); err != nil {
		log.Fatal("Failed to start tenant service:", err)
	}
}
