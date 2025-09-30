package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Tenant represents a tenant in the multi-tenant system
type Tenant struct {
	ID        uuid.UUID      `json:"id" gorm:"type:uuid;primary_key;default:gen_random_uuid()"`
	Name      string         `json:"name" gorm:"not null"`
	Domain    string         `json:"domain" gorm:"uniqueIndex"`
	IsActive  bool           `json:"is_active" gorm:"default:true"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Relationships
	Users []User `json:"users,omitempty" gorm:"foreignKey:TenantID"`
}

// TableName returns the table name for the Tenant model
func (Tenant) TableName() string {
	return "tenants"
}
