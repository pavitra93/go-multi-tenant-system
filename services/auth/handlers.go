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
	"github.com/golang-jwt/jwt/v5"
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
	TenantID string `json:"tenant_id,omitempty"` // Optional for admin users
	Role     string `json:"role,omitempty"`      // Optional: admin, tenant_owner, or user (defaults to user)
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
				utils.UnauthorizedResponse(c, "Invalid credentials")
			}
			return
		}

		accessToken := *authResult.AuthenticationResult.AccessToken
		idToken := *authResult.AuthenticationResult.IdToken

		cognitoID, err := extractCognitoIDFromToken(idToken)
		if err != nil {
			utils.InternalServerErrorResponse(c, "Failed to extract user ID from token")
			return
		}

		userProfile, err := buildUserProfileFromDB(db, cognitoID, req.Username)
		if err != nil {
			utils.InternalServerErrorResponse(c, "Failed to build user profile")
			return
		}

		sessionTTL := time.Duration(*authResult.AuthenticationResult.ExpiresIn) * time.Second
		session, err := utils.CreateTokenSession(accessToken, userProfile, sessionTTL)
		if err != nil {
			utils.InternalServerErrorResponse(c, "Failed to create session")
			return
		}

		go func() {
			now := time.Now()
			if userProfile.IsAdmin {
				db.Model(&models.Admin{}).Where("cognito_id = ?", userProfile.CognitoID).Update("last_login_at", now)
			} else {
				db.Model(&models.User{}).Where("cognito_id = ?", userProfile.CognitoID).Update("last_login_at", now)
			}
		}()

		response := map[string]interface{}{
			"access_token": accessToken,
			"expires_in":   *authResult.AuthenticationResult.ExpiresIn,
			"token_type":   "Bearer",
			"user_info":    userProfile,
			"session_id":   session.SessionID,
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

		// Determine user role (default to "user" if not specified)
		userRole := models.RoleUser
		if req.Role != "" {
			// Validate role
			switch req.Role {
			case "tenant_owner", "user":
				userRole = models.UserRole(req.Role)
			case "admin":
				// Admin users are handled separately
				utils.BadRequestResponse(c, "Admin users must be created through a separate process")
				return
			default:
				utils.BadRequestResponse(c, "Invalid role. Must be 'tenant_owner' or 'user'")
				return
			}
		}

		// All users must provide a tenant ID
		if req.TenantID == "" {
			utils.BadRequestResponse(c, "Tenant ID is required")
			return
		}

		parsedTenantID, err := uuid.Parse(req.TenantID)
		if err != nil {
			utils.BadRequestResponse(c, "Invalid tenant ID")
			return
		}

		var tenant models.Tenant
		if err := db.Where("id = ?", parsedTenantID).First(&tenant).Error; err != nil {
			utils.NotFoundResponse(c, "Tenant not found")
			return
		}

		tx := db.Begin()
		defer func() {
			if r := recover(); r != nil {
				tx.Rollback()
			}
		}()

		user := models.User{
			CognitoID: "",
			TenantID:  parsedTenantID,
			Role:      userRole,
			CreatedAt: time.Now(),
		}
		userAttributes := []*cognitoidentityprovider.AttributeType{
			{
				Name:  aws.String("custom:role"),
				Value: aws.String(string(userRole)),
			},
			{
				Name:  aws.String("email"),
				Value: aws.String(req.Username),
			},
		}

		userAttributes = append(userAttributes, &cognitoidentityprovider.AttributeType{
			Name:  aws.String("custom:tenant_id"),
			Value: aws.String(parsedTenantID.String()),
		})

		signUpInput := &cognitoidentityprovider.SignUpInput{
			ClientId:       aws.String(os.Getenv("COGNITO_CLIENT_ID")),
			Username:       aws.String(req.Username),
			Password:       aws.String(req.Password),
			UserAttributes: userAttributes,
		}

		if secretHash := generateSecretHash(req.Username); secretHash != "" {
			signUpInput.SecretHash = aws.String(secretHash)
		}

		var signUpResult *cognitoidentityprovider.SignUpOutput
		cognitoErr := circuitBreaker.Call(func() error {
			var err error
			signUpResult, err = cognitoClient.SignUp(signUpInput)
			return err
		})

		if cognitoErr != nil {
			tx.Rollback()
			if cognitoErr == utils.ErrCircuitOpen {
				utils.ServiceUnavailableResponse(c, "Authentication service temporarily unavailable")
			} else {
				utils.BadRequestResponse(c, "Failed to register user: "+cognitoErr.Error())
			}
			return
		}

		user.CognitoID = *signUpResult.UserSub
		if err := tx.Create(&user).Error; err != nil {
			compensateErr := circuitBreaker.Call(func() error {
				_, deleteErr := cognitoClient.AdminDeleteUser(&cognitoidentityprovider.AdminDeleteUserInput{
					UserPoolId: aws.String(os.Getenv("COGNITO_USER_POOL_ID")),
					Username:   aws.String(req.Username),
				})
				return deleteErr
			})

			if compensateErr != nil {
				logrus.WithFields(logrus.Fields{
					"username": req.Username,
					"error":    compensateErr,
				}).Warn("Failed to compensate orphaned Cognito user")
			}

			tx.Rollback()
			utils.InternalServerErrorResponse(c, "Failed to complete registration")
			return
		}

		if err := tx.Commit().Error; err != nil {
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
			"username":   req.Username,
			"role":       string(userRole),
			"message":    "User registered successfully. Please confirm email before login.",
		}

		// Include tenant_id for tenant users
		userResponse["tenant_id"] = user.TenantID
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

// handleConfirmEmail handles manual email confirmation (admin only)
func handleConfirmEmail(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Username string `json:"username" binding:"required"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			utils.BadRequestResponse(c, "Invalid request format")
			return
		}

		// Confirm user email in Cognito
		err := circuitBreaker.Call(func() error {
			_, confirmErr := cognitoClient.AdminConfirmSignUp(&cognitoidentityprovider.AdminConfirmSignUpInput{
				UserPoolId: aws.String(os.Getenv("COGNITO_USER_POOL_ID")),
				Username:   aws.String(req.Username),
			})
			return confirmErr
		})

		if err != nil {
			utils.BadRequestResponse(c, "Failed to confirm email: "+err.Error())
			return
		}

		utils.OKResponse(c, "Email confirmed successfully", map[string]interface{}{
			"username": req.Username,
			"message":  "User can now login",
		})
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
		userInfo.TenantID = &tenantID
	}

	return userInfo, nil
}

// extractCognitoIDFromToken extracts the Cognito ID from a JWT token
func extractCognitoIDFromToken(tokenString string) (string, error) {
	token, _, err := new(jwt.Parser).ParseUnverified(tokenString, jwt.MapClaims{})
	if err != nil {
		return "", fmt.Errorf("failed to parse token: %w", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", fmt.Errorf("invalid token claims format")
	}

	sub, ok := claims["sub"].(string)
	if !ok {
		return "", fmt.Errorf("sub claim not found or not a string")
	}

	return sub, nil
}

// buildUserProfileFromDB builds a UserProfile from database lookup
func buildUserProfileFromDB(db *gorm.DB, cognitoID, email string) (models.UserProfile, error) {
	// First check if user is an admin
	var admin models.Admin
	if err := db.Where("cognito_id = ?", cognitoID).First(&admin).Error; err == nil {
		// User is an admin
		return models.UserProfile{
			CognitoID: admin.CognitoID,
			Email:     email, // Use actual email from login request
			Role:      "admin",
			TenantID:  nil,
			IsAdmin:   true,
			Metadata:  make(map[string]interface{}),
		}, nil
	}

	// Check if user is a tenant user
	var user models.User
	if err := db.Where("cognito_id = ?", cognitoID).First(&user).Error; err != nil {
		return models.UserProfile{}, fmt.Errorf("user not found: %w", err)
	}

	return models.UserProfile{
		CognitoID: user.CognitoID,
		Email:     email, // Use actual email from login request
		Role:      string(user.Role),
		TenantID:  &user.TenantID,
		IsAdmin:   false,
		Metadata:  make(map[string]interface{}),
	}, nil
}

// handleLogout handles user logout and session revocation
func handleLogout(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get access token from context (set by auth middleware)
		accessToken, exists := c.Get("access_token")
		if !exists {
			utils.UnauthorizedResponse(c, "No active session found")
			return
		}

		// Revoke session in Redis
		err := utils.RevokeTokenSession(accessToken.(string))
		if err != nil {
			utils.InternalServerErrorResponse(c, "Failed to revoke session")
			return
		}

		utils.OKResponse(c, "Logout successful", map[string]interface{}{
			"message": "Session revoked successfully",
		})
	}
}

// handleGetSessions handles getting user's active sessions
func handleGetSessions(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get user info from context (for future use)
		_, err := middleware.GetUserInfoFromContext(c)
		if err != nil {
			utils.UnauthorizedResponse(c, "User info not found")
			return
		}

		// For now, return current session info
		// In a full implementation, you'd scan Redis for all user sessions
		session, exists := c.Get("session")
		if !exists {
			utils.InternalServerErrorResponse(c, "Session not found")
			return
		}

		tokenSession := session.(*models.TokenSession)

		response := map[string]interface{}{
			"active_sessions": []map[string]interface{}{
				{
					"session_id":   tokenSession.SessionID,
					"created_at":   tokenSession.CreatedAt,
					"last_used_at": tokenSession.LastUsedAt,
					"expires_at":   tokenSession.ExpiresAt,
					"is_current":   true,
				},
			},
			"total_sessions": 1,
		}

		utils.OKResponse(c, "Sessions retrieved", response)
	}
}

// handleRevokeSession handles revoking a specific session
func handleRevokeSession(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		sessionID := c.Param("session_id")
		if sessionID == "" {
			utils.BadRequestResponse(c, "Session ID required")
			return
		}

		// Get user info from context (for future use)
		_, err := middleware.GetUserInfoFromContext(c)
		if err != nil {
			utils.UnauthorizedResponse(c, "User info not found")
			return
		}

		// For now, only allow revoking current session
		// In a full implementation, you'd validate the session belongs to the user
		currentSession, exists := c.Get("session")
		if !exists {
			utils.InternalServerErrorResponse(c, "Current session not found")
			return
		}

		tokenSession := currentSession.(*models.TokenSession)
		if tokenSession.SessionID != sessionID {
			utils.ForbiddenResponse(c, "Can only revoke your own sessions")
			return
		}

		// Revoke session
		accessToken, _ := c.Get("access_token")
		err = utils.RevokeTokenSession(accessToken.(string))
		if err != nil {
			utils.InternalServerErrorResponse(c, "Failed to revoke session")
			return
		}

		utils.OKResponse(c, "Session revoked successfully", map[string]interface{}{
			"session_id": sessionID,
			"message":    "Session has been revoked",
		})
	}
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
