package utils

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/go-redis/redis/v8"
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

// CloseRedis closes the Redis connection
func CloseRedis() error {
	if RedisClient != nil {
		return RedisClient.Close()
	}
	return nil
}
