package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cognitoidentityprovider"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/pavitra93/go-multi-tenant-system/shared/middleware"
	"github.com/pavitra93/go-multi-tenant-system/shared/models"
	"github.com/pavitra93/go-multi-tenant-system/shared/utils"
)

var (
	cognitoClient  *cognitoidentityprovider.CognitoIdentityProvider
	circuitBreaker *utils.CircuitBreaker
)

// generateSecretHash creates a secret hash for Cognito authentication
func generateSecretHash(username string) string {
	clientSecret := os.Getenv("COGNITO_CLIENT_SECRET")
	clientId := os.Getenv("COGNITO_CLIENT_ID")

	if clientSecret == "" {
		return ""
	}

	mac := hmac.New(sha256.New, []byte(clientSecret))
	mac.Write([]byte(username + clientId))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func init() {
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(os.Getenv("AWS_REGION")),
	})
	if err != nil {
		panic("Failed to create AWS session: " + err.Error())
	}
	cognitoClient = cognitoidentityprovider.New(sess)

	// Initialize circuit breaker for Cognito calls (max 5 failures, 30 second reset)
	circuitBreaker = utils.NewCircuitBreaker(5, 30*time.Second)
}

// LoginRequest represents the login request
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// RegisterRequest represents the registration request
type RegisterRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required,min=8"`
	TenantID string `json:"tenant_id" binding:"required"`
	Role     string `json:"role,omitempty"` // Optional: admin, tenant_owner, or user (defaults to user)
}

// LoginResponse represents the login response
type LoginResponse struct {
	AccessToken  string           `json:"access_token"`
	IdToken      string           `json:"id_token"` // ID token contains custom attributes
	RefreshToken string           `json:"refresh_token"`
	ExpiresIn    int64            `json:"expires_in"`
	TokenType    string           `json:"token_type"`
	UserInfo     *models.UserInfo `json:"user_info"` // Constructed from JWT claims
}

// handleLogin handles user login with circuit breaker
func handleLogin(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req LoginRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			utils.BadRequestResponse(c, "Invalid request format")
			return
		}

		// Note: We don't query the database here - user info comes from Cognito JWT
		// The database only stores minimal data for relationships

		// Authenticate with Cognito
		authParams := map[string]*string{
			"USERNAME": aws.String(req.Username),
			"PASSWORD": aws.String(req.Password),
		}

		// Add secret hash if client secret is configured
		if secretHash := generateSecretHash(req.Username); secretHash != "" {
			authParams["SECRET_HASH"] = aws.String(secretHash)
		}

		authInput := &cognitoidentityprovider.InitiateAuthInput{
			AuthFlow:       aws.String("USER_PASSWORD_AUTH"),
			ClientId:       aws.String(os.Getenv("COGNITO_CLIENT_ID")),
			AuthParameters: authParams,
		}

		// Use circuit breaker for Cognito call
		var authResult *cognitoidentityprovider.InitiateAuthOutput
		err := circuitBreaker.Call(func() error {
			var cognitoErr error
			authResult, cognitoErr = cognitoClient.InitiateAuth(authInput)
			return cognitoErr
		})

		if err != nil {
			if err == utils.ErrCircuitOpen {
				utils.ServiceUnavailableResponse(c, "Authentication service temporarily unavailable")
			} else {
				fmt.Printf("Cognito login error: %v\n", err)
				utils.UnauthorizedResponse(c, "Invalid credentials")
			}
			return
		}

		// Parse ID token to extract user info
		idToken := *authResult.AuthenticationResult.IdToken
		userInfo, err := extractUserInfoFromToken(idToken)
		if err != nil {
			// Fallback: try access token
			accessToken := *authResult.AuthenticationResult.AccessToken
			userInfo, _ = extractUserInfoFromToken(accessToken)
		} else {
			// Update last_login_at asynchronously (non-blocking)
			go func() {
				now := time.Now()
				db.Model(&models.User{}).Where("cognito_id = ?", userInfo.CognitoID).Update("last_login_at", now)
			}()
		}

		// Prepare response with user info from JWT (no DB query needed!)
		response := map[string]interface{}{
			"access_token":  *authResult.AuthenticationResult.AccessToken,
			"id_token":      idToken,
			"refresh_token": *authResult.AuthenticationResult.RefreshToken,
			"expires_in":    *authResult.AuthenticationResult.ExpiresIn,
			"token_type":    "Bearer",
			"user_info":     userInfo,
			// Clear instruction for developers
			"_note": "Use id_token (not access_token) for API calls - it contains custom attributes",
		}

		utils.OKResponse(c, "Login successful", response)
	}
}

// handleRegister handles user registration with proper distributed transaction handling
func handleRegister(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req RegisterRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			utils.BadRequestResponse(c, "Invalid request format")
			return
		}

		// Validate tenant exists
		tenantID, err := uuid.Parse(req.TenantID)
		if err != nil {
			utils.BadRequestResponse(c, "Invalid tenant ID")
			return
		}

		var tenant models.Tenant
		if err := db.Where("id = ?", tenantID).First(&tenant).Error; err != nil {
			utils.NotFoundResponse(c, "Tenant not found")
			return
		}

		// Step 1: Start database transaction
		tx := db.Begin()
		defer func() {
			if r := recover(); r != nil {
				tx.Rollback()
			}
		}()

		// Step 2: Create minimal user in database (will update with Cognito ID later)
		// Note: CognitoID is required, so we'll set it after Cognito signup succeeds
		user := models.User{
			CognitoID: "", // Will be set after Cognito signup
			TenantID:  tenantID,
			CreatedAt: time.Now(),
		}

		// Don't create user in DB yet - wait for Cognito success first
		// This ensures we don't have orphaned DB records

		// Determine user role (default to "user" if not specified)
		userRole := models.RoleUser
		if req.Role != "" {
			// Validate role
			switch req.Role {
			case "admin", "tenant_owner", "user":
				userRole = models.UserRole(req.Role)
			default:
				utils.BadRequestResponse(c, "Invalid role. Must be 'admin', 'tenant_owner', or 'user'")
				return
			}
		}

		// Step 3: Register user with Cognito with custom attributes
		signUpInput := &cognitoidentityprovider.SignUpInput{
			ClientId: aws.String(os.Getenv("COGNITO_CLIENT_ID")),
			Username: aws.String(req.Username),
			Password: aws.String(req.Password),
			UserAttributes: []*cognitoidentityprovider.AttributeType{
				{
					Name:  aws.String("custom:tenant_id"),
					Value: aws.String(user.TenantID.String()),
				},
				{
					Name:  aws.String("custom:role"),
					Value: aws.String(string(userRole)),
				},
				{
					Name:  aws.String("email"),
					Value: aws.String(req.Username), // Using username as email
				},
			},
		}

		// Add secret hash if client secret is configured
		if secretHash := generateSecretHash(req.Username); secretHash != "" {
			signUpInput.SecretHash = aws.String(secretHash)
		}

		// Use circuit breaker for Cognito call
		var signUpResult *cognitoidentityprovider.SignUpOutput
		err = circuitBreaker.Call(func() error {
			var cognitoErr error
			signUpResult, cognitoErr = cognitoClient.SignUp(signUpInput)
			return cognitoErr
		})

		if err != nil {
			// Rollback database changes if Cognito fails
			tx.Rollback()

			if err == utils.ErrCircuitOpen {
				utils.ServiceUnavailableResponse(c, "Authentication service temporarily unavailable")
			} else {
				utils.BadRequestResponse(c, "Failed to register user: "+err.Error())
			}
			return
		}

		// Step 4: Create user in database with Cognito ID (primary key)
		user.CognitoID = *signUpResult.UserSub
		if err := tx.Create(&user).Error; err != nil {
			// Compensate: Delete from Cognito if database creation fails
			compensateErr := circuitBreaker.Call(func() error {
				_, deleteErr := cognitoClient.AdminDeleteUser(&cognitoidentityprovider.AdminDeleteUserInput{
					UserPoolId: aws.String(os.Getenv("COGNITO_USER_POOL_ID")),
					Username:   aws.String(req.Username),
				})
				return deleteErr
			})

			if compensateErr != nil {
				// Log compensation failure for monitoring
				logrus.WithFields(logrus.Fields{
					"username": req.Username,
					"error":    compensateErr,
				}).Warn("Failed to compensate orphaned Cognito user")
			}

			tx.Rollback()
			utils.InternalServerErrorResponse(c, "Failed to complete registration")
			return
		}

		// Step 5: Commit transaction
		if err := tx.Commit().Error; err != nil {
			// Last resort compensation
			_ = circuitBreaker.Call(func() error {
				_, deleteErr := cognitoClient.AdminDeleteUser(&cognitoidentityprovider.AdminDeleteUserInput{
					UserPoolId: aws.String(os.Getenv("COGNITO_USER_POOL_ID")),
					Username:   aws.String(req.Username),
				})
				return deleteErr
			})

			utils.InternalServerErrorResponse(c, "Failed to complete registration")
			return
		}

		// Return success with user info (no sensitive data exposed)
		userResponse := map[string]interface{}{
			"cognito_id": user.CognitoID,
			"tenant_id":  user.TenantID,
			"username":   req.Username,
			"role":       "user",
			"message":    "Please check your email to verify your account",
		}
		utils.CreatedResponse(c, "User registered successfully", userResponse)
	}
}

// handleRefreshToken handles token refresh
func handleRefreshToken(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			RefreshToken string `json:"refresh_token" binding:"required"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			utils.BadRequestResponse(c, "Invalid request format")
			return
		}

		// Refresh token with Cognito
		authParams := map[string]*string{
			"REFRESH_TOKEN": aws.String(req.RefreshToken),
		}

		// Add secret hash if client secret is configured
		// For refresh token, we need to get username from the token or context
		if secretHash := generateSecretHash(""); secretHash != "" {
			// Note: For refresh token, we might need to get username differently
			// This is a simplified approach - in production, you'd extract username from the refresh token
		}

		authInput := &cognitoidentityprovider.InitiateAuthInput{
			AuthFlow:       aws.String("REFRESH_TOKEN_AUTH"),
			ClientId:       aws.String(os.Getenv("COGNITO_CLIENT_ID")),
			AuthParameters: authParams,
		}

		authResult, err := cognitoClient.InitiateAuth(authInput)
		if err != nil {
			utils.UnauthorizedResponse(c, "Invalid refresh token")
			return
		}

		response := map[string]interface{}{
			"access_token": *authResult.AuthenticationResult.AccessToken,
			"expires_in":   *authResult.AuthenticationResult.ExpiresIn,
			"token_type":   "Bearer",
		}

		utils.OKResponse(c, "Token refreshed successfully", response)
	}
}

// handleVerifyToken handles token verification
func handleVerifyToken() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, email, tenantID, role := middleware.GetUserFromContext(c)

		response := map[string]interface{}{
			"user_id":   userID,
			"email":     email,
			"tenant_id": tenantID,
			"role":      role,
		}

		utils.OKResponse(c, "Token is valid", response)
	}
}

// handleLogout handles user logout
func handleLogout() gin.HandlerFunc {
	return func(c *gin.Context) {
		// In a real implementation, you might want to blacklist the token
		// For now, we'll just return a success response
		utils.OKResponse(c, "Logged out successfully", nil)
	}
}

// handleGetUsers handles getting all users (admin only)
// Returns minimal user data from DB + enriched data from Cognito if needed
func handleGetUsers(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var users []models.User

		// Get minimal user data from database (fast!)
		if err := db.Preload("Tenant").Find(&users).Error; err != nil {
			utils.InternalServerErrorResponse(c, "Failed to fetch users")
			return
		}

		// Transform to include tenant name
		type UserResponse struct {
			CognitoID   string     `json:"cognito_id"`
			TenantID    uuid.UUID  `json:"tenant_id"`
			TenantName  string     `json:"tenant_name,omitempty"`
			CreatedAt   time.Time  `json:"created_at"`
			LastLoginAt *time.Time `json:"last_login_at,omitempty"`
		}

		response := make([]UserResponse, len(users))
		for i, user := range users {
			response[i] = UserResponse{
				CognitoID:   user.CognitoID,
				TenantID:    user.TenantID,
				CreatedAt:   user.CreatedAt,
				LastLoginAt: user.LastLoginAt,
			}
			if user.Tenant != nil {
				response[i].TenantName = user.Tenant.Name
			}
		}

		utils.OKResponse(c, "Users retrieved successfully", response)
	}
}

// handleGetUser handles getting a specific user
func handleGetUser(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		cognitoID := c.Param("id") // Now expecting cognito_id instead of UUID

		var user models.User
		if err := db.Preload("Tenant").Where("cognito_id = ?", cognitoID).First(&user).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				utils.NotFoundResponse(c, "User not found")
			} else {
				utils.InternalServerErrorResponse(c, "Failed to fetch user")
			}
			return
		}

		// Optionally enrich with Cognito data
		// For now, return minimal DB data
		response := map[string]interface{}{
			"cognito_id":    user.CognitoID,
			"tenant_id":     user.TenantID,
			"tenant_name":   "",
			"created_at":    user.CreatedAt,
			"last_login_at": user.LastLoginAt,
		}

		if user.Tenant != nil {
			response["tenant_name"] = user.Tenant.Name
		}

		utils.OKResponse(c, "User retrieved successfully", response)
	}
}

// handleUpdateUser handles updating a user's role in Cognito
// Note: Role is stored in Cognito, not in the database
func handleUpdateUser(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		cognitoID := c.Param("id")

		// Verify user exists in database
		var user models.User
		if err := db.Where("cognito_id = ?", cognitoID).First(&user).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				utils.NotFoundResponse(c, "User not found")
			} else {
				utils.InternalServerErrorResponse(c, "Failed to fetch user")
			}
			return
		}

		var updateData struct {
			Role *string `json:"role"`
		}

		if err := c.ShouldBindJSON(&updateData); err != nil {
			utils.BadRequestResponse(c, "Invalid request format")
			return
		}

		// Update role in Cognito (source of truth for user attributes)
		if updateData.Role != nil {
			err := circuitBreaker.Call(func() error {
				_, updateErr := cognitoClient.AdminUpdateUserAttributes(&cognitoidentityprovider.AdminUpdateUserAttributesInput{
					UserPoolId: aws.String(os.Getenv("COGNITO_USER_POOL_ID")),
					Username:   aws.String(cognitoID), // Cognito username or sub
					UserAttributes: []*cognitoidentityprovider.AttributeType{
						{
							Name:  aws.String("custom:role"),
							Value: aws.String(*updateData.Role),
						},
					},
				})
				return updateErr
			})

			if err != nil {
				if err == utils.ErrCircuitOpen {
					utils.ServiceUnavailableResponse(c, "Authentication service temporarily unavailable")
				} else {
					utils.InternalServerErrorResponse(c, "Failed to update user role: "+err.Error())
				}
				return
			}
		}

		utils.OKResponse(c, "User updated successfully. Changes will take effect on next login.", map[string]interface{}{
			"cognito_id": cognitoID,
			"role":       *updateData.Role,
		})
	}
}

// handleDeleteUser handles deleting a user from both Cognito and database
func handleDeleteUser(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		cognitoID := c.Param("id")

		// Delete from Cognito first
		err := circuitBreaker.Call(func() error {
			_, deleteErr := cognitoClient.AdminDeleteUser(&cognitoidentityprovider.AdminDeleteUserInput{
				UserPoolId: aws.String(os.Getenv("COGNITO_USER_POOL_ID")),
				Username:   aws.String(cognitoID),
			})
			return deleteErr
		})

		if err != nil {
			if err == utils.ErrCircuitOpen {
				utils.ServiceUnavailableResponse(c, "Authentication service temporarily unavailable")
			} else {
				utils.InternalServerErrorResponse(c, "Failed to delete user from Cognito: "+err.Error())
			}
			return
		}

		// Delete from database (cascade will handle related records)
		if err := db.Delete(&models.User{}, "cognito_id = ?", cognitoID).Error; err != nil {
			utils.InternalServerErrorResponse(c, "Failed to delete user from database")
			return
		}

		utils.OKResponse(c, "User deleted successfully", nil)
	}
}

// extractUserInfoFromToken parses the JWT access token and extracts user information
// This allows us to get user details without a database query
func extractUserInfoFromToken(tokenString string) (*models.UserInfo, error) {
	// Parse the JWT token (we don't verify signature here since it's already verified by Cognito)
	// In production, you might want to use the JWKS validator here as well
	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid token format")
	}

	// Decode the payload (second part)
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("failed to decode token payload: %w", err)
	}

	// Parse claims
	var claims map[string]interface{}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("failed to parse token claims: %w", err)
	}

	// Extract user information
	userInfo := &models.UserInfo{
		CognitoID: getStringClaim(claims, "sub"),
		Username:  getStringClaim(claims, "cognito:username"),
		Email:     getStringClaim(claims, "email"),
		Role:      models.UserRole(getStringClaim(claims, "custom:role")),
	}

	// Parse tenant_id UUID
	tenantIDStr := getStringClaim(claims, "custom:tenant_id")
	if tenantIDStr != "" {
		tenantID, err := uuid.Parse(tenantIDStr)
		if err != nil {
			return nil, fmt.Errorf("invalid tenant_id in token: %w", err)
		}
		userInfo.TenantID = tenantID
	}

	return userInfo, nil
}

// getStringClaim safely extracts a string claim
func getStringClaim(claims map[string]interface{}, key string) string {
	if val, ok := claims[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}
