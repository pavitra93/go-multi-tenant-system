package middleware

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pavitra93/go-multi-tenant-system/shared/config"
	"github.com/pavitra93/go-multi-tenant-system/shared/models"
	"github.com/pavitra93/go-multi-tenant-system/shared/utils"
	"gorm.io/gorm"
)

// AuthMiddleware handles authentication via Redis session lookup
type AuthMiddleware struct {
	db *gorm.DB
}

// NewAuthMiddleware creates a new authentication middleware
func NewAuthMiddleware(region, userPoolID string) (*AuthMiddleware, error) {
	// Initialize database connection using shared config
	db, err := config.ConnectDatabase()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	return &AuthMiddleware{
		db: db,
	}, nil
}

// RequireAuth middleware validates access token via Redis lookup
func (am *AuthMiddleware) RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		accessToken := extractToken(c)
		if accessToken == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization token required"})
			c.Abort()
			return
		}

		// Look up session in Redis
		fmt.Printf("Looking up session for token: %s...\n", accessToken[:20])
		session, err := utils.GetTokenSession(accessToken)
		if err != nil {
			fmt.Printf("Session lookup failed: %v\n", err)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired token"})
			c.Abort()
			return
		}
		fmt.Printf("Session found: %+v\n", session.UserProfile)

		// Update last used timestamp (non-blocking)
		go func() {
			_ = utils.UpdateTokenSessionLastUsed(accessToken)
		}()

		// Set user context from session
		c.Set("user_id", session.UserProfile.CognitoID)
		c.Set("username", session.UserProfile.Username)
		c.Set("email", session.UserProfile.Email)
		c.Set("role", session.UserProfile.Role)
		c.Set("is_admin", session.UserProfile.IsAdmin)
		c.Set("access_token", accessToken)
		c.Set("session", session)

		// Set tenant_id if user has one
		if session.UserProfile.TenantID != nil {
			c.Set("tenant_id", session.UserProfile.TenantID.String())
		}

		// Set tenant context in database for Row-Level Security
		// This activates RLS policies to enforce tenant isolation
		// Only set tenant context for non-admin users
		if !session.UserProfile.IsAdmin && session.UserProfile.TenantID != nil {
			am.db.Exec("SELECT set_tenant_context(?)", *session.UserProfile.TenantID)
			am.db.Exec("SELECT set_user_role(?)", session.UserProfile.Role)
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
		// Check if user is admin (can access any tenant)
		role, _ := c.Get("role")
		if role == "admin" {
			c.Next()
			return
		}

		// For non-admin users, check tenant access
		userTenantID, exists := c.Get("tenant_id")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Tenant information not found"})
			c.Abort()
			return
		}

		// Check if they're accessing their own tenant
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
	// Try to get session first (preferred method)
	if sessionInterface, exists := c.Get("session"); exists {
		if session, ok := sessionInterface.(*models.TokenSession); ok {
			return &models.UserInfo{
				CognitoID: session.UserProfile.CognitoID,
				Username:  session.UserProfile.Username,
				Email:     session.UserProfile.Email,
				Role:      models.UserRole(session.UserProfile.Role),
				TenantID:  session.UserProfile.TenantID,
				IsAdmin:   session.UserProfile.IsAdmin,
			}, nil
		}
	}

	// Fallback to individual context values (for backward compatibility)
	cognitoID := c.GetString("user_id")
	if cognitoID == "" {
		return nil, fmt.Errorf("user_id not found in context")
	}

	email := c.GetString("email")
	tenantIDStr := c.GetString("tenant_id")
	role := c.GetString("role")
	isAdmin, _ := c.Get("is_admin")

	var tenantID *uuid.UUID
	if tenantIDStr != "" {
		parsedTenantID, err := uuid.Parse(tenantIDStr)
		if err != nil {
			return nil, fmt.Errorf("invalid tenant_id in context: %w", err)
		}
		tenantID = &parsedTenantID
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
		IsAdmin:   isAdmin.(bool),
	}, nil
}

// GetTenantIDFromContext extracts tenant ID from the Gin context
// Returns error for admin users who don't have a tenant_id
func GetTenantIDFromContext(c *gin.Context) (uuid.UUID, error) {
	tenantIDStr, exists := c.Get("tenant_id")
	if !exists || tenantIDStr == "" {
		return uuid.Nil, fmt.Errorf("tenant_id not found in context")
	}

	return uuid.Parse(tenantIDStr.(string))
}
