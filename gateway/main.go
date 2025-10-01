package main

import (
	"log"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/pavitra93/go-multi-tenant-system/shared/middleware"
	"github.com/pavitra93/go-multi-tenant-system/shared/utils"
	"github.com/sirupsen/logrus"
)

func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		logrus.Warn("No .env file found, using environment variables")
	}

	// Initialize Redis for caching
	if err := utils.InitRedis(); err != nil {
		logrus.Warnf("Failed to connect to Redis, caching disabled: %v", err)
	}

	// Get AWS configuration
	awsRegion := os.Getenv("AWS_REGION")
	cognitoUserPoolID := os.Getenv("COGNITO_USER_POOL_ID")

	if awsRegion == "" || cognitoUserPoolID == "" {
		log.Fatal("AWS_REGION and COGNITO_USER_POOL_ID must be set")
	}

	// Initialize authentication middleware
	authMiddleware, err := middleware.NewAuthMiddleware(
		awsRegion,
		cognitoUserPoolID,
	)
	if err != nil {
		log.Fatal("Failed to initialize auth middleware:", err)
	}

	// Initialize service clients
	serviceClients := &ServiceClients{
		AuthService:      NewServiceClient(os.Getenv("AUTH_SERVICE_URL")),
		TenantService:    NewServiceClient(os.Getenv("TENANT_SERVICE_URL")),
		LocationService:  NewServiceClient(os.Getenv("LOCATION_SERVICE_URL")),
		StreamingService: NewServiceClient(os.Getenv("STREAMING_SERVICE_URL")),
	}

	// Initialize Gin router
	router := gin.Default()

	// Add CORS middleware
	router.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	})

	// Health check endpoint
	router.GET("/health", func(c *gin.Context) {
		utils.OKResponse(c, "API Gateway is healthy", nil)
	})

	// Authentication routes
	auth := router.Group("/auth")
	{
		auth.POST("/login", serviceClients.AuthService.ProxyRequest)
		auth.POST("/register", serviceClients.AuthService.ProxyRequest)
		auth.POST("/refresh", serviceClients.AuthService.ProxyRequest)
		// Note: /auth/confirm endpoint removed - email confirmation handled manually in AWS console
		auth.POST("/logout", authMiddleware.RequireAuth(), serviceClients.AuthService.ProxyRequest)
		auth.GET("/sessions", authMiddleware.RequireAuth(), serviceClients.AuthService.ProxyRequest)
		auth.DELETE("/sessions/:session_id", authMiddleware.RequireAuth(), serviceClients.AuthService.ProxyRequest)
	}

	// User management routes (admin only)
	users := router.Group("/users")
	users.Use(authMiddleware.RequireAuth(), authMiddleware.RequireRole("admin"))
	{
		users.GET("/", serviceClients.AuthService.ProxyRequest)
		users.GET("/:id", serviceClients.AuthService.ProxyRequest)
		users.PUT("/:id", serviceClients.AuthService.ProxyRequest)
		users.DELETE("/:id", serviceClients.AuthService.ProxyRequest)
	}

	// Tenant management routes
	tenants := router.Group("/tenants")
	tenants.Use(authMiddleware.RequireAuth())
	{
		// Admin-only routes (platform management)
		tenants.POST("/", authMiddleware.RequireRole("admin"), serviceClients.TenantService.ProxyRequest)
		tenants.GET("/", authMiddleware.RequireRole("admin"), serviceClients.TenantService.ProxyRequest)
		tenants.DELETE("/:id", authMiddleware.RequireRole("admin"), serviceClients.TenantService.ProxyRequest)

		// Tenant-specific routes (tenant owner can manage their own tenant)
		tenants.GET("/:id", authMiddleware.RequireTenantAccess(), serviceClients.TenantService.ProxyRequest)
		tenants.PUT("/:id", authMiddleware.RequireTenantOwnerOrAdmin(), serviceClients.TenantService.ProxyRequest)
		tenants.GET("/:id/users", authMiddleware.RequireTenantOwnerOrAdmin(), serviceClients.TenantService.ProxyRequest)
		tenants.GET("/:id/settings", authMiddleware.RequireTenantAccess(), serviceClients.TenantService.ProxyRequest)
		tenants.PUT("/:id/settings", authMiddleware.RequireTenantOwnerOrAdmin(), serviceClients.TenantService.ProxyRequest)

		// User management within tenant (tenant owner can invite/manage users)
		tenants.POST("/:id/users", authMiddleware.RequireTenantOwnerOrAdmin(), serviceClients.TenantService.ProxyRequest)
		tenants.PUT("/:id/users/:user_id", authMiddleware.RequireTenantOwnerOrAdmin(), serviceClients.TenantService.ProxyRequest)
		tenants.DELETE("/:id/users/:user_id", authMiddleware.RequireTenantOwnerOrAdmin(), serviceClients.TenantService.ProxyRequest)
	}

	// Location tracking routes
	location := router.Group("/location")
	location.Use(authMiddleware.RequireAuth())
	{
		// Session management
		location.POST("/session/start", serviceClients.LocationService.ProxyRequest)
		location.POST("/session/:id/stop", serviceClients.LocationService.ProxyRequest)
		location.GET("/session/:id", serviceClients.LocationService.ProxyRequest)
		location.GET("/sessions", serviceClients.LocationService.ProxyRequest)

		// Location data submission
		location.POST("/update", serviceClients.LocationService.ProxyRequest)
		location.GET("/session/:id/locations", serviceClients.LocationService.ProxyRequest)
	}

	// Streaming observability routes (read-only, for monitoring)
	// These demonstrate that streaming requirements are met
	streaming := router.Group("/streaming")
	streaming.Use(authMiddleware.RequireAuth())
	{
		streaming.GET("/health", serviceClients.StreamingService.ProxyRequest)
		streaming.GET("/metrics", serviceClients.StreamingService.ProxyRequest)
	}

	// Start server
	port := os.Getenv("API_GATEWAY_PORT")
	if port == "" {
		port = "8080"
	}

	logrus.Infof("API Gateway starting on port %s", port)
	if err := router.Run(":" + port); err != nil {
		log.Fatal("Failed to start API Gateway:", err)
	}
}
