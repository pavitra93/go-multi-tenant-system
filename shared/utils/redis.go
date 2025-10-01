package utils

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"github.com/pavitra93/go-multi-tenant-system/shared/models"
)

var (
	RedisClient *redis.Client
	ctx         = context.Background()
)

// InitRedis initializes the Redis client
func InitRedis() error {
	redisHost := os.Getenv("REDIS_HOST")
	if redisHost == "" {
		redisHost = "localhost"
	}

	redisPort := os.Getenv("REDIS_PORT")
	if redisPort == "" {
		redisPort = "6379"
	}

	addr := fmt.Sprintf("%s:%s", redisHost, redisPort)

	RedisClient = redis.NewClient(&redis.Options{
		Addr:         addr,
		Password:     "", // No password by default
		DB:           0,  // Default DB
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		PoolSize:     10,
		MinIdleConns: 5,
	})

	// Test connection
	_, err := RedisClient.Ping(ctx).Result()
	if err != nil {
		return fmt.Errorf("failed to connect to Redis at %s: %w", addr, err)
	}

	fmt.Printf("✅ Connected to Redis at %s\n", addr)
	return nil
}

// CacheSet stores a value in Redis with expiration
func CacheSet(key string, value string, expiration time.Duration) error {
	if RedisClient == nil {
		return fmt.Errorf("Redis client not initialized")
	}
	return RedisClient.Set(ctx, key, value, expiration).Err()
}

// CacheGet retrieves a value from Redis
func CacheGet(key string) (string, error) {
	if RedisClient == nil {
		return "", fmt.Errorf("Redis client not initialized")
	}
	val, err := RedisClient.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", fmt.Errorf("key not found")
	}
	return val, err
}

// CacheDelete removes a key from Redis
func CacheDelete(key string) error {
	return RedisClient.Del(ctx, key).Err()
}

// CacheExists checks if a key exists in Redis
func CacheExists(key string) (bool, error) {
	count, err := RedisClient.Exists(ctx, key).Result()
	return count > 0, err
}

// GetRedisClient returns the Redis client instance (for advanced operations)
func GetRedisClient() *redis.Client {
	return RedisClient
}

// GetRedisContext returns the Redis context
func GetRedisContext() context.Context {
	return ctx
}

// CloseRedis closes the Redis connection
func CloseRedis() error {
	if RedisClient != nil {
		return RedisClient.Close()
	}
	return nil
}

// Token Session Management Functions

// generateTokenHash creates a SHA256 hash of the access token for use as Redis key
func generateTokenHash(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

// CreateTokenSession creates a new token session in Redis (token hash as key, no token stored)
func CreateTokenSession(accessToken string, userProfile models.UserProfile, ttl time.Duration) (*models.TokenSession, error) {
	if RedisClient == nil {
		return nil, fmt.Errorf("Redis client not initialized")
	}

	sessionID := uuid.New().String()
	now := time.Now()

	session := &models.TokenSession{
		UserProfile: userProfile,
		CreatedAt:   now,
		LastUsedAt:  now,
		ExpiresAt:   now.Add(ttl),
		SessionID:   sessionID,
	}

	// Serialize session to JSON
	sessionData, err := json.Marshal(session)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal session: %w", err)
	}

	// Store in Redis with token hash as key (no token stored)
	tokenHash := generateTokenHash(accessToken)
	key := fmt.Sprintf("token:session:%s", tokenHash)

	err = RedisClient.Set(ctx, key, sessionData, ttl).Err()
	if err != nil {
		return nil, fmt.Errorf("failed to store session in Redis: %w", err)
	}

	return session, nil
}

// GetTokenSession retrieves a token session from Redis (token hash lookup)
func GetTokenSession(accessToken string) (*models.TokenSession, error) {
	if RedisClient == nil {
		return nil, fmt.Errorf("Redis client not initialized")
	}

	tokenHash := generateTokenHash(accessToken)
	key := fmt.Sprintf("token:session:%s", tokenHash)

	sessionData, err := RedisClient.Get(ctx, key).Result()
	if err == redis.Nil {
		return nil, fmt.Errorf("session not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get session from Redis: %w", err)
	}

	var session models.TokenSession
	err = json.Unmarshal([]byte(sessionData), &session)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal session: %w", err)
	}

	// Check if session is expired
	if session.IsExpired() {
		// Clean up expired session
		RedisClient.Del(ctx, key)
		return nil, fmt.Errorf("session expired")
	}

	return &session, nil
}

// UpdateTokenSessionLastUsed updates the last used timestamp for a token session
func UpdateTokenSessionLastUsed(accessToken string) error {
	if RedisClient == nil {
		return fmt.Errorf("Redis client not initialized")
	}

	tokenHash := generateTokenHash(accessToken)
	key := fmt.Sprintf("token:session:%s", tokenHash)

	// Get current session
	session, err := GetTokenSession(accessToken)
	if err != nil {
		return err
	}

	// Update last used timestamp
	session.UpdateLastUsed()

	// Store back to Redis
	sessionData, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("failed to marshal updated session: %w", err)
	}

	// Calculate remaining TTL
	remainingTTL := time.Until(session.ExpiresAt)
	if remainingTTL <= 0 {
		return fmt.Errorf("session expired")
	}

	return RedisClient.Set(ctx, key, sessionData, remainingTTL).Err()
}

// RevokeTokenSession removes a token session from Redis
func RevokeTokenSession(accessToken string) error {
	if RedisClient == nil {
		return fmt.Errorf("Redis client not initialized")
	}

	tokenHash := generateTokenHash(accessToken)
	key := fmt.Sprintf("token:session:%s", tokenHash)

	// Remove token session
	err := RedisClient.Del(ctx, key).Err()
	if err != nil {
		return fmt.Errorf("failed to revoke session: %w", err)
	}

	return nil
}

// RevokeAllUserSessions removes all sessions for a specific user
func RevokeAllUserSessions(cognitoID string) error {
	if RedisClient == nil {
		return fmt.Errorf("Redis client not initialized")
	}

	// Scan all session keys and remove those belonging to the user
	pattern := "token:session:*"
	keys, err := RedisClient.Keys(ctx, pattern).Result()
	if err != nil {
		return fmt.Errorf("failed to scan session keys: %w", err)
	}

	for _, key := range keys {
		sessionData, err := RedisClient.Get(ctx, key).Result()
		if err != nil {
			continue
		}

		var session models.TokenSession
		if json.Unmarshal([]byte(sessionData), &session) == nil {
			if session.UserProfile.CognitoID == cognitoID {
				RedisClient.Del(ctx, key)
			}
		}
	}

	return nil
}
