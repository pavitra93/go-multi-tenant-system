package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pavitra93/go-multi-tenant-system/shared/middleware"
	"github.com/pavitra93/go-multi-tenant-system/shared/models"
	"github.com/pavitra93/go-multi-tenant-system/shared/utils"
)

// StartSessionRequest represents the start session request
type StartSessionRequest struct {
	Duration int `json:"duration"` // in seconds, default 600 (10 minutes)
}

// LocationUpdateRequest represents the location update request
type LocationUpdateRequest struct {
	SessionID uuid.UUID  `json:"session_id" binding:"required"`
	Latitude  float64    `json:"latitude" binding:"required"`
	Longitude float64    `json:"longitude" binding:"required"`
	Timestamp *time.Time `json:"timestamp"`
}

// LocationEvent represents a location event for Kafka
type LocationEvent struct {
	ID            uuid.UUID `json:"id"`
	TenantID      uuid.UUID `json:"tenant_id"`
	CognitoUserID string    `json:"cognito_cognito_user_id"`
	SessionID     uuid.UUID `json:"session_id"`
	Latitude      float64   `json:"latitude"`
	Longitude     float64   `json:"longitude"`
	Timestamp     time.Time `json:"timestamp"`
	EventType     string    `json:"event_type"`
}

// handleStartSession handles starting a new location tracking session
func handleStartSession(db *gorm.DB, kafkaProducer *KafkaProducer) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, _, tenantID, _ := middleware.GetUserFromContext(c)

		var req StartSessionRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			utils.BadRequestResponse(c, "Invalid request format")
			return
		}

		// Set default duration if not provided
		if req.Duration == 0 {
			req.Duration = 600 // 10 minutes
		}

		// Check if user has an active session
		var activeSession models.LocationSession
		if err := db.Where("cognito_user_id = ? AND status = ?", userID, models.SessionStatusActive).First(&activeSession).Error; err == nil {
			utils.BadRequestResponse(c, "User already has an active session")
			return
		}

		// Parse tenant UUID
		tenantUUID, err := uuid.Parse(tenantID)
		if err != nil {
			utils.BadRequestResponse(c, "Invalid tenant ID")
			return
		}

		// Create new session
		session := models.LocationSession{
			ID:            uuid.New(),
			TenantID:      tenantUUID,
			CognitoUserID: userID, // userID is cognito_id from JWT
			Status:        models.SessionStatusActive,
			StartedAt:     time.Now(),
			Duration:      req.Duration,
		}

		if err := db.Create(&session).Error; err != nil {
			utils.InternalServerErrorResponse(c, "Failed to create session")
			return
		}

		// Cache the session in Redis with TTL = session duration
		cacheKey := fmt.Sprintf("session:active:%s", session.ID.String())
		if sessionData, err := json.Marshal(session); err == nil {
			cacheDuration := time.Duration(session.Duration) * time.Second
			if err := utils.CacheSet(cacheKey, string(sessionData), cacheDuration); err != nil {
				// Cache failure is non-critical
			}
		}

		// Send session event to Kafka (async with worker pool)

		utils.CreatedResponse(c, "Session started successfully", session)
	}
}

// handleStopSession handles stopping a location tracking session
func handleStopSession(db *gorm.DB, kafkaProducer *KafkaProducer) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, _, tenantID, _ := middleware.GetUserFromContext(c)
		sessionID := c.Param("id")

		// Parse tenant UUID
		tenantUUID, err := uuid.Parse(tenantID)
		if err != nil {
			utils.BadRequestResponse(c, "Invalid tenant ID")
			return
		}

		sessionUUID, err := uuid.Parse(sessionID)
		if err != nil {
			utils.BadRequestResponse(c, "Invalid session ID")
			return
		}

		// Find and update session
		var session models.LocationSession
		if err := db.Where("id = ? AND cognito_user_id = ? AND tenant_id = ?", sessionUUID, userID, tenantUUID).First(&session).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				utils.NotFoundResponse(c, "Session not found")
			} else {
				utils.InternalServerErrorResponse(c, "Failed to fetch session")
			}
			return
		}

		if session.Status != models.SessionStatusActive {
			utils.BadRequestResponse(c, "Session is not active")
			return
		}

		// End the session
		session.EndSession()

		if err := db.Save(&session).Error; err != nil {
			utils.InternalServerErrorResponse(c, "Failed to update session")
			return
		}

		// Invalidate session cache in Redis
		cacheKey := fmt.Sprintf("session:active:%s", sessionUUID.String())
		if redisClient := utils.GetRedisClient(); redisClient != nil {
			redisClient.Del(utils.GetRedisContext(), cacheKey)
		}

		// Send session event to Kafka (async with worker pool)

		utils.OKResponse(c, "Session stopped successfully", session)
	}
}

// handleGetSession handles getting a specific session
func handleGetSession(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, _, tenantID, _ := middleware.GetUserFromContext(c)
		sessionID := c.Param("id")

		// Parse tenant UUID
		tenantUUID, err := uuid.Parse(tenantID)
		if err != nil {
			utils.BadRequestResponse(c, "Invalid tenant ID")
			return
		}

		sessionUUID, err := uuid.Parse(sessionID)
		if err != nil {
			utils.BadRequestResponse(c, "Invalid session ID")
			return
		}

		var session models.LocationSession
		if err := db.Where("id = ? AND cognito_user_id = ? AND tenant_id = ?", sessionUUID, userID, tenantUUID).First(&session).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				utils.NotFoundResponse(c, "Session not found")
			} else {
				utils.InternalServerErrorResponse(c, "Failed to fetch session")
			}
			return
		}

		utils.OKResponse(c, "Session retrieved successfully", session)
	}
}

// handleGetUserSessions handles getting all sessions for a user
func handleGetUserSessions(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, _, tenantID, _ := middleware.GetUserFromContext(c)

		// Parse tenant UUID
		tenantUUID, err := uuid.Parse(tenantID)
		if err != nil {
			utils.BadRequestResponse(c, "Invalid tenant ID")
			return
		}

		var sessions []models.LocationSession
		if err := db.Where("cognito_user_id = ? AND tenant_id = ?", userID, tenantUUID).Order("created_at DESC").Find(&sessions).Error; err != nil {
			utils.InternalServerErrorResponse(c, "Failed to fetch sessions")
			return
		}

		utils.OKResponse(c, "Sessions retrieved successfully", sessions)
	}
}

// handleLocationUpdate handles location data updates
func handleLocationUpdate(db *gorm.DB, kafkaProducer *KafkaProducer) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, _, tenantID, _ := middleware.GetUserFromContext(c)

		var req LocationUpdateRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			utils.BadRequestResponse(c, "Invalid request format")
			return
		}

		// Parse tenant UUID
		tenantUUID, err := uuid.Parse(tenantID)
		if err != nil {
			utils.BadRequestResponse(c, "Invalid tenant ID")
			return
		}

		// Try to get session from Redis cache first (OPTIMIZATION)
		var session models.LocationSession
		cacheKey := fmt.Sprintf("session:active:%s", req.SessionID.String())
		sessionFound := false

		if cachedData, err := utils.CacheGet(cacheKey); err == nil {
			// Cache HIT - parse session from Redis
			if err := json.Unmarshal([]byte(cachedData), &session); err == nil {
				// Verify user and tenant match (security check)
				if session.CognitoUserID == userID && session.TenantID == tenantUUID {
					sessionFound = true
				}
			}
		}

		// Cache MISS - fallback to database
		if !sessionFound {
			if err := db.Where("id = ? AND cognito_user_id = ? AND tenant_id = ? AND status = ?", req.SessionID, userID, tenantUUID, models.SessionStatusActive).First(&session).Error; err != nil {
				if err == gorm.ErrRecordNotFound {
					utils.NotFoundResponse(c, "Active session not found")
				} else {
					utils.InternalServerErrorResponse(c, "Failed to fetch session")
				}
				return
			}

			// Cache the session for future requests (with remaining TTL)
			elapsed := time.Since(session.StartedAt).Seconds()
			remainingTTL := time.Duration(session.Duration)*time.Second - time.Duration(elapsed)*time.Second
			if remainingTTL > 0 {
				if sessionData, err := json.Marshal(session); err == nil {
					_ = utils.CacheSet(cacheKey, string(sessionData), remainingTTL)
				}
			}
		}

		// Check if session has expired (for both cache hit and miss)
		if time.Since(session.StartedAt).Seconds() > float64(session.Duration) {
			// Auto-end expired session
			session.EndSession()
			db.Save(&session)
			// Invalidate cache
			if redisClient := utils.GetRedisClient(); redisClient != nil {
				redisClient.Del(utils.GetRedisContext(), cacheKey)
			}
			utils.BadRequestResponse(c, "Session has expired")
			return
		}

		// Set timestamp if not provided
		timestamp := time.Now()
		if req.Timestamp != nil {
			timestamp = *req.Timestamp
		}

		// Create location record
		location := models.Location{
			ID:            uuid.New(),
			TenantID:      tenantUUID,
			SessionID:     req.SessionID,
			CognitoUserID: userID, // userID is cognito_id from JWT
			Latitude:      req.Latitude,
			Longitude:     req.Longitude,
			Timestamp:     timestamp,
		}

		if err := db.Create(&location).Error; err != nil {
			utils.InternalServerErrorResponse(c, "Failed to save location")
			return
		}

		// Send location event to Kafka (async with worker pool)
		locationEvent := LocationEvent{
			ID:            location.ID,
			TenantID:      tenantUUID,
			CognitoUserID: userID,
			SessionID:     req.SessionID,
			Latitude:      req.Latitude,
			Longitude:     req.Longitude,
			Timestamp:     timestamp,
			EventType:     "location_update",
		}

		if err := kafkaProducer.SendLocationEvent(locationEvent); err != nil {
			// Queue full - event dropped
		}

		utils.OKResponse(c, "Location updated successfully", location)
	}
}

// handleGetSessionLocations handles getting all locations for a session
func handleGetSessionLocations(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, _, tenantID, _ := middleware.GetUserFromContext(c)
		sessionID := c.Param("id")

		// Parse tenant UUID
		tenantUUID, err := uuid.Parse(tenantID)
		if err != nil {
			utils.BadRequestResponse(c, "Invalid tenant ID")
			return
		}

		sessionUUID, err := uuid.Parse(sessionID)
		if err != nil {
			utils.BadRequestResponse(c, "Invalid session ID")
			return
		}

		// Verify session belongs to user
		var session models.LocationSession
		if err := db.Where("id = ? AND cognito_user_id = ? AND tenant_id = ?", sessionUUID, userID, tenantUUID).First(&session).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				utils.NotFoundResponse(c, "Session not found")
			} else {
				utils.InternalServerErrorResponse(c, "Failed to fetch session")
			}
			return
		}

		var locations []models.Location
		if err := db.Where("session_id = ? AND tenant_id = ?", sessionUUID, tenantUUID).Order("timestamp ASC").Find(&locations).Error; err != nil {
			utils.InternalServerErrorResponse(c, "Failed to fetch locations")
			return
		}

		utils.OKResponse(c, "Locations retrieved successfully", locations)
	}
}
