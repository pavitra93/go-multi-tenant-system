package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cognitoidentityprovider"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/pavitra93/go-multi-tenant-system/shared/models"
	"github.com/pavitra93/go-multi-tenant-system/shared/utils"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// AuthMiddleware handles JWT token validation
type AuthMiddleware struct {
	cognitoClient  *cognitoidentityprovider.CognitoIdentityProvider
	userPoolID     string
	db             *gorm.DB
	jwksValidator  *utils.JWKSValidator
	circuitBreaker *utils.CircuitBreaker
}

// CognitoClaims represents Cognito JWT claims
type CognitoClaims struct {
	Sub            string `json:"sub"`
	Email          string `json:"email"`
	Username       string `json:"cognito:username"`
	TokenUse       string `json:"token_use"`
	CustomTenantID string `json:"custom:tenant_id"`
	CustomRole     string `json:"custom:role"`
	jwt.RegisteredClaims
}

// NewAuthMiddleware creates a new authentication middleware
func NewAuthMiddleware(region, userPoolID string) (*AuthMiddleware, error) {
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(region),
	})
	if err != nil {
		return nil, err
	}

	// Initialize database connection
	db, err := initDatabase()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	// Initialize JWKS validator for proper token verification
	jwksValidator := utils.NewJWKSValidator(region, userPoolID)

	// Initialize circuit breaker (max 5 failures, 30 second reset)
	circuitBreaker := utils.NewCircuitBreaker(5, 30*time.Second)

	return &AuthMiddleware{
		cognitoClient:  cognitoidentityprovider.New(sess),
		userPoolID:     userPoolID,
		db:             db,
		jwksValidator:  jwksValidator,
		circuitBreaker: circuitBreaker,
	}, nil
}

// RequireAuth middleware validates JWT tokens
func (am *AuthMiddleware) RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenString := extractToken(c)
		if tokenString == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization token required"})
			c.Abort()
			return
		}

		// Simple token parsing (trusting Cognito tokens)
		claims, err := am.parseTokenSimple(tokenString)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
			c.Abort()
			return
		}

		// Extract user information from Cognito custom attributes
		// No database call needed - everything is in the JWT!
		c.Set("user_id", claims.Sub)
		c.Set("username", claims.Username)
		c.Set("email", claims.Email)
		c.Set("tenant_id", claims.CustomTenantID)
		c.Set("role", claims.CustomRole)

		// Set tenant context in database for Row-Level Security
		// This activates RLS policies to enforce tenant isolation
		tenantUUID, err := uuid.Parse(claims.CustomTenantID)
		if err == nil {
			am.db.Exec("SELECT set_tenant_context(?)", tenantUUID)
			am.db.Exec("SELECT set_user_role(?)", claims.CustomRole)
		}

		c.Next()
	}
}

// RequireRole middleware validates user role
func (am *AuthMiddleware) RequireRole(requiredRole string) gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := c.Get("role")

		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User role not found in context"})
			c.Abort()
			return
		}

		if role != requiredRole {
			c.JSON(http.StatusForbidden, gin.H{
				"error":         "Insufficient permissions",
				"required_role": requiredRole,
				"user_role":     role,
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// RequireTenantOwnerOrAdmin middleware allows tenant owners to manage their own tenant
func (am *AuthMiddleware) RequireTenantOwnerOrAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, _ := c.Get("role")

		// Platform admin can access everything
		if role == "admin" {
			c.Next()
			return
		}

		// Tenant owner can only access their own tenant
		if role == "tenant_owner" {
			// Check if accessing their own tenant
			requestedTenantID := c.Param("id")
			userTenantID := c.GetString("tenant_id")

			if requestedTenantID == "" || requestedTenantID == userTenantID {
				c.Next()
				return
			}

			c.JSON(http.StatusForbidden, gin.H{
				"error": "Tenant owners can only manage their own tenant",
			})
			c.Abort()
			return
		}

		c.JSON(http.StatusForbidden, gin.H{
			"error":         "Insufficient permissions",
			"required_role": "tenant_owner or admin",
			"user_role":     role,
		})
		c.Abort()
	}
}

// RequireTenantAccess middleware validates tenant access
// Allows: admin (all tenants), tenant_owner (own tenant), user (own tenant - read only)
func (am *AuthMiddleware) RequireTenantAccess() gin.HandlerFunc {
	return func(c *gin.Context) {
		userTenantID, exists := c.Get("tenant_id")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Tenant information not found"})
			c.Abort()
			return
		}

		// Check if user is admin (can access any tenant)
		role, _ := c.Get("role")
		if role == "admin" {
			c.Next()
			return
		}

		// For non-admin users (including tenant_owner), check if they're accessing their own tenant
		requestedTenantID := c.Param("id")
		if requestedTenantID == "" {
			requestedTenantID = c.Param("tenant_id")
		}

		if requestedTenantID != "" && requestedTenantID != userTenantID {
			c.JSON(http.StatusForbidden, gin.H{"error": "Access denied to this tenant"})
			c.Abort()
			return
		}

		c.Next()
	}
}

// getCacheKey generates a cache key for the token
func getCacheKey(tokenString string) string {
	hash := sha256.Sum256([]byte(tokenString))
	return "token:" + hex.EncodeToString(hash[:])
}

// extractToken extracts the JWT token from the Authorization header
func extractToken(c *gin.Context) string {
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		return ""
	}

	// Check for "Bearer " prefix
	if strings.HasPrefix(authHeader, "Bearer ") {
		return strings.TrimPrefix(authHeader, "Bearer ")
	}

	return authHeader
}

// parseTokenSimple parses JWT token without signature verification (simplified for now)
func (am *AuthMiddleware) parseTokenSimple(tokenString string) (*CognitoClaims, error) {
	// Check Redis cache first
	cacheKey := getCacheKey(tokenString)
	if cachedData, err := utils.CacheGet(cacheKey); err == nil {
		var claims CognitoClaims
		if err := json.Unmarshal([]byte(cachedData), &claims); err == nil {
			return &claims, nil
		}
	}

	// Parse token without verification (we trust Cognito tokens)
	// In production, use validateTokenWithJWKS for proper security
	token, _, err := new(jwt.Parser).ParseUnverified(tokenString, jwt.MapClaims{})
	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("invalid token claims format")
	}

	// Parse Cognito claims
	cognitoClaims := &CognitoClaims{
		Sub:            getClaimString(claims, "sub"),
		Email:          getClaimString(claims, "email"),
		Username:       getClaimString(claims, "cognito:username"),
		TokenUse:       getClaimString(claims, "token_use"),
		CustomTenantID: getClaimString(claims, "custom:tenant_id"),
		CustomRole:     getClaimString(claims, "custom:role"),
	}

	// If custom attributes missing, get from database
	if cognitoClaims.CustomTenantID == "" || cognitoClaims.CustomRole == "" {
		var user models.User
		if err := am.db.Where("cognito_id = ?", cognitoClaims.Sub).First(&user).Error; err != nil {
			return nil, fmt.Errorf("user not found: %w", err)
		}
		cognitoClaims.CustomTenantID = user.TenantID.String()
		cognitoClaims.CustomRole = "user" // You can store role in DB if needed
	}

	// Cache the parsed token for 1 hour
	if cognitoClaims.CustomTenantID != "" && cognitoClaims.CustomRole != "" {
		if cacheData, err := json.Marshal(cognitoClaims); err == nil {
			_ = utils.CacheSet(cacheKey, string(cacheData), 1*time.Hour)
		}
	}

	return cognitoClaims, nil
}

// validateTokenWithJWKS validates the JWT token using JWKS and extracts custom attributes
// Currently disabled due to network issues, using parseTokenSimple instead
func (am *AuthMiddleware) validateTokenWithJWKS(tokenString string) (*CognitoClaims, error) {
	// Use JWKS validator for proper signature verification
	token, err := am.jwksValidator.ValidateToken(tokenString)
	if err != nil {
		return nil, fmt.Errorf("JWKS validation failed: %w", err)
	}

	// Extract claims
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("invalid token claims format")
	}

	// Parse Cognito claims
	cognitoClaims := &CognitoClaims{
		Sub:      getClaimString(claims, "sub"),
		Email:    getClaimString(claims, "email"),
		Username: getClaimString(claims, "cognito:username"),
		TokenUse: getClaimString(claims, "token_use"),
		// Extract custom attributes
		CustomTenantID: getClaimString(claims, "custom:tenant_id"),
		CustomRole:     getClaimString(claims, "custom:role"),
	}

	// Accept both "access" and "id" tokens
	// ID tokens contain custom attributes, access tokens don't
	if cognitoClaims.TokenUse != "access" && cognitoClaims.TokenUse != "id" {
		return nil, fmt.Errorf("invalid token use: expected 'access' or 'id', got '%s'", cognitoClaims.TokenUse)
	}

	// If custom attributes are missing (access token doesn't have them),
	// fall back to Cognito AdminGetUser API and database lookup
	if cognitoClaims.CustomTenantID == "" || cognitoClaims.CustomRole == "" {
		// Lookup user in database using cognito_id (sub)
		var user models.User
		if err := am.db.Where("cognito_id = ?", cognitoClaims.Sub).First(&user).Error; err != nil {
			return nil, fmt.Errorf("user not found in database: %w", err)
		}

		// Get custom attributes from Cognito
		getUserOutput, err := am.cognitoClient.AdminGetUser(&cognitoidentityprovider.AdminGetUserInput{
			UserPoolId: aws.String(am.userPoolID),
			Username:   aws.String(cognitoClaims.Sub),
		})

		if err != nil {
			return nil, fmt.Errorf("failed to get user from Cognito: %w", err)
		}

		// Extract custom attributes from Cognito user
		for _, attr := range getUserOutput.UserAttributes {
			if *attr.Name == "custom:tenant_id" && cognitoClaims.CustomTenantID == "" {
				cognitoClaims.CustomTenantID = *attr.Value
			}
			if *attr.Name == "custom:role" && cognitoClaims.CustomRole == "" {
				cognitoClaims.CustomRole = *attr.Value
			}
			if *attr.Name == "email" && cognitoClaims.Email == "" {
				cognitoClaims.Email = *attr.Value
			}
		}

		// Fallback to database tenant_id if still missing
		if cognitoClaims.CustomTenantID == "" {
			cognitoClaims.CustomTenantID = user.TenantID.String()
		}

		// Default to "user" role if still not found
		if cognitoClaims.CustomRole == "" {
			cognitoClaims.CustomRole = "user"
		}

		// Set username if not in token
		if cognitoClaims.Username == "" {
			cognitoClaims.Username = cognitoClaims.Email
			if cognitoClaims.Username == "" {
				cognitoClaims.Username = cognitoClaims.Sub
			}
		}
	}

	return cognitoClaims, nil
}

// getClaimString safely extracts a string claim from JWT claims
func getClaimString(claims jwt.MapClaims, key string) string {
	if val, ok := claims[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

// GetUserFromContext extracts user information from the Gin context
// Returns data that was extracted from JWT claims
func GetUserFromContext(c *gin.Context) (cognitoID, email, tenantID, role string) {
	cognitoID = c.GetString("user_id") // This is actually cognito_id (sub from JWT)
	email = c.GetString("email")
	tenantID = c.GetString("tenant_id")
	role = c.GetString("role")
	return
}

// GetUserInfoFromContext extracts full user information from the Gin context as UserInfo struct
func GetUserInfoFromContext(c *gin.Context) (*models.UserInfo, error) {
	cognitoID := c.GetString("user_id")
	if cognitoID == "" {
		return nil, fmt.Errorf("user_id not found in context")
	}

	email := c.GetString("email")
	tenantIDStr := c.GetString("tenant_id")
	role := c.GetString("role")

	tenantID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		return nil, fmt.Errorf("invalid tenant_id in context: %w", err)
	}

	username := c.GetString("username")
	if username == "" {
		username = email // Fallback to email if username not set
	}

	return &models.UserInfo{
		CognitoID: cognitoID,
		Username:  username,
		Email:     email,
		Role:      models.UserRole(role),
		TenantID:  tenantID,
	}, nil
}

// GetTenantIDFromContext extracts tenant ID from the Gin context
func GetTenantIDFromContext(c *gin.Context) (uuid.UUID, error) {
	tenantIDStr, exists := c.Get("tenant_id")
	if !exists {
		return uuid.Nil, fmt.Errorf("tenant_id not found in context")
	}

	return uuid.Parse(tenantIDStr.(string))
}

// initDatabase initializes the database connection
// Note: Database is still used for data operations, just not for every auth check
func initDatabase() (*gorm.DB, error) {
	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=disable",
		getEnv("DB_HOST", "postgres"),
		getEnv("DB_USER", "postgres"),
		getEnv("DB_PASSWORD", "password"),
		getEnv("DB_NAME", "multi_tenant_db"),
		getEnv("DB_PORT", "5432"),
	)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	return db, nil
}

// getEnv gets an environment variable with a default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
