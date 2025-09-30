package models

import (
	"time"

	"github.com/google/uuid"
)

// User represents a minimal user record in the system
// Most user data (username, email, role) is stored in Cognito and accessed via JWT claims
// This table only stores what's needed for relationships, analytics, and multi-tenancy
type User struct {
	CognitoID   string     `json:"cognito_id" gorm:"type:varchar(255);primaryKey"`
	TenantID    uuid.UUID  `json:"tenant_id" gorm:"type:uuid;not null;index"`
	CreatedAt   time.Time  `json:"created_at" gorm:"default:CURRENT_TIMESTAMP"`
	LastLoginAt *time.Time `json:"last_login_at,omitempty"`
	// Metadata removed - store additional user data in Cognito custom attributes
	// Cognito supports up to 25 custom attributes which is sufficient for most use cases

	// Relationships
	Tenant           *Tenant           `json:"tenant,omitempty" gorm:"foreignKey:TenantID"`
	LocationSessions []LocationSession `json:"location_sessions,omitempty" gorm:"foreignKey:CognitoUserID;references:CognitoID"`
}

// UserRole represents the role of a user (used for validation)
type UserRole string

const (
	RoleAdmin       UserRole = "admin"        // Platform administrator (manages all tenants)
	RoleTenantOwner UserRole = "tenant_owner" // Tenant administrator (manages own tenant)
	RoleUser        UserRole = "user"         // Regular tenant user
)

// TableName returns the table name for the User model
func (User) TableName() string {
	return "users"
}

// GetID returns the Cognito ID as the primary identifier
func (u *User) GetID() string {
	return u.CognitoID
}

// UserInfo represents enriched user information from JWT claims
// This is constructed from the JWT and doesn't require database lookup
type UserInfo struct {
	CognitoID string    `json:"cognito_id"`
	Username  string    `json:"username"`  // From JWT: cognito:username
	Email     string    `json:"email"`     // From JWT: email
	Role      UserRole  `json:"role"`      // From JWT: custom:role
	TenantID  uuid.UUID `json:"tenant_id"` // From JWT: custom:tenant_id
}

// IsAdmin checks if the user has platform admin role (using JWT claims)
func (ui *UserInfo) IsAdmin() bool {
	return ui.Role == RoleAdmin
}

// IsTenantOwner checks if the user is a tenant owner
func (ui *UserInfo) IsTenantOwner() bool {
	return ui.Role == RoleTenantOwner
}

// CanManageTenant checks if the user can manage a specific tenant
func (ui *UserInfo) CanManageTenant(tenantID uuid.UUID) bool {
	// Admin can manage all tenants
	if ui.IsAdmin() {
		return true
	}
	// Tenant owner can manage their own tenant
	if ui.IsTenantOwner() && ui.TenantID == tenantID {
		return true
	}
	return false
}

// CanAccessTenant checks if the user can access a specific tenant
func (ui *UserInfo) CanAccessTenant(tenantID uuid.UUID) bool {
	return ui.TenantID == tenantID || ui.IsAdmin()
}
