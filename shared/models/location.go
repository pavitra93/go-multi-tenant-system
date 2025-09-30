package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// LocationSession represents a location tracking session
type LocationSession struct {
	ID            uuid.UUID      `json:"id" gorm:"type:uuid;primary_key;default:gen_random_uuid()"`
	TenantID      uuid.UUID      `json:"tenant_id" gorm:"type:uuid;not null;index"`
	CognitoUserID string         `json:"cognito_user_id" gorm:"type:varchar(255);not null;index"`
	Status        SessionStatus  `json:"status" gorm:"type:varchar(20);not null;default:'active'"`
	StartedAt     time.Time      `json:"started_at"`
	EndedAt       *time.Time     `json:"ended_at"`
	Duration      int            `json:"duration"` // in seconds
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Relationships
	Tenant    *Tenant    `json:"tenant,omitempty" gorm:"foreignKey:TenantID"`
	User      *User      `json:"user,omitempty" gorm:"foreignKey:CognitoUserID;references:CognitoID"`
	Locations []Location `json:"locations,omitempty" gorm:"foreignKey:SessionID"`
}

// Location represents a single location data point
type Location struct {
	ID            uuid.UUID      `json:"id" gorm:"type:uuid;primary_key;default:gen_random_uuid()"`
	TenantID      uuid.UUID      `json:"tenant_id" gorm:"type:uuid;not null;index"`
	SessionID     uuid.UUID      `json:"session_id" gorm:"type:uuid;not null;index"`
	CognitoUserID string         `json:"cognito_user_id" gorm:"type:varchar(255);not null;index"`
	Latitude      float64        `json:"latitude" gorm:"not null"`
	Longitude     float64        `json:"longitude" gorm:"not null"`
	Timestamp     time.Time      `json:"timestamp" gorm:"not null"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Relationships
	Session *LocationSession `json:"session,omitempty" gorm:"foreignKey:SessionID"`
	User    *User            `json:"user,omitempty" gorm:"foreignKey:CognitoUserID;references:CognitoID"`
}

// SessionStatus represents the status of a location session
type SessionStatus string

const (
	SessionStatusActive    SessionStatus = "active"
	SessionStatusEnded     SessionStatus = "ended"
	SessionStatusExpired   SessionStatus = "expired"
	SessionStatusCancelled SessionStatus = "cancelled"
)

// TableName returns the table name for the LocationSession model
func (LocationSession) TableName() string {
	return "location_sessions"
}

// TableName returns the table name for the Location model
func (Location) TableName() string {
	return "locations"
}

// IsActive checks if the session is currently active
func (s *LocationSession) IsActive() bool {
	return s.Status == SessionStatusActive
}

// GetDuration returns the session duration in seconds
func (s *LocationSession) GetDuration() int {
	if s.EndedAt != nil {
		return int(s.EndedAt.Sub(s.StartedAt).Seconds())
	}
	return int(time.Since(s.StartedAt).Seconds())
}

// EndSession ends the location session
func (s *LocationSession) EndSession() {
	now := time.Now()
	s.EndedAt = &now
	s.Status = SessionStatusEnded
	s.Duration = s.GetDuration()
}
