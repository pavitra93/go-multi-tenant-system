package config

import (
	"fmt"
	"os"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// DatabaseConfig holds database configuration
type DatabaseConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
	SSLMode  string
}

// GetDatabaseConfig returns database configuration from environment variables
func GetDatabaseConfig() *DatabaseConfig {
	return &DatabaseConfig{
		Host:     getEnv("DB_HOST", "localhost"),
		Port:     getEnv("DB_PORT", "5432"),
		User:     getEnv("DB_USER", "postgres"),
		Password: getEnv("DB_PASSWORD", "password"),
		DBName:   getEnv("DB_NAME", "multi_tenant_db"),
		SSLMode:  getEnv("DB_SSL_MODE", "disable"),
	}
}

// GetDSN returns the database connection string
func (c *DatabaseConfig) GetDSN() string {
	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.Password, c.DBName, c.SSLMode)
}

// ConnectDatabase establishes a connection to the database with optimized pool settings
func ConnectDatabase() (*gorm.DB, error) {
	config := GetDatabaseConfig()

	db, err := gorm.Open(postgres.Open(config.GetDSN()), &gorm.Config{
		PrepareStmt: true,                                 // Enable prepared statement cache for better performance
		Logger:      logger.Default.LogMode(logger.Error), // Reduce logging overhead in production
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Configure connection pool for optimal performance
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get underlying SQL DB: %w", err)
	}

	// Connection pool settings (per service)
	// With 4 services × 25 = 100 max connections (PostgreSQL default limit)
	sqlDB.SetMaxOpenConns(25)                  // Maximum open connections per service
	sqlDB.SetMaxIdleConns(10)                  // Keep 10 idle connections for fast reuse
	sqlDB.SetConnMaxLifetime(30 * time.Minute) // Recycle connections every 30 minutes
	sqlDB.SetConnMaxIdleTime(10 * time.Minute) // Close idle connections after 10 minutes

	// Verify connection
	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
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
