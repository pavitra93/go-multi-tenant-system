package models

import (
	"time"

	"github.com/google/uuid"
)

// User represents a tenant user record in the system
// Admin users are stored in a separate admins table
// Most user data (username, email) is stored in Cognito and accessed via JWT claims
// This table only stores what's needed for relationships, analytics, and multi-tenancy
type User struct {
	CognitoID   string     `json:"cognito_id" gorm:"type:varchar(255);primaryKey"`
	TenantID    uuid.UUID  `json:"tenant_id" gorm:"type:uuid;index"`        // Required for all tenant users
	Role        UserRole   `json:"role" gorm:"type:user_role;default:user"` // Role within tenant
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

// Admin represents a platform administrator
// Admins are not associated with any tenant and have system-wide access
type Admin struct {
	CognitoID   string     `json:"cognito_id" gorm:"type:varchar(255);primaryKey"`
	CreatedAt   time.Time  `json:"created_at" gorm:"default:CURRENT_TIMESTAMP"`
	LastLoginAt *time.Time `json:"last_login_at,omitempty"`
	Metadata    string     `json:"metadata" gorm:"type:jsonb;default:'{}'"`
}

// TableName returns the table name for the Admin model
func (Admin) TableName() string {
	return "admins"
}

// UserInfo represents enriched user information from JWT claims
// This is constructed from the JWT and doesn't require database lookup
type UserInfo struct {
	CognitoID string     `json:"cognito_id"`
	Username  string     `json:"username"`            // From JWT: cognito:username
	Email     string     `json:"email"`               // From JWT: email
	Role      UserRole   `json:"role"`                // From JWT: custom:role
	TenantID  *uuid.UUID `json:"tenant_id,omitempty"` // From JWT: custom:tenant_id (nil for admin users)
	IsAdmin   bool       `json:"is_admin"`            // True if user is a platform admin
}

// IsAdminUser checks if the user is a platform admin
func (ui *UserInfo) IsAdminUser() bool {
	return ui.IsAdmin
}

// IsTenantOwner checks if the user is a tenant owner
func (ui *UserInfo) IsTenantOwner() bool {
	return ui.Role == RoleTenantOwner
}

// CanManageTenant checks if the user can manage a specific tenant
func (ui *UserInfo) CanManageTenant(tenantID uuid.UUID) bool {
	// Admin can manage all tenants
	if ui.IsAdminUser() {
		return true
	}
	// Tenant owner can manage their own tenant
	if ui.IsTenantOwner() && ui.TenantID != nil && *ui.TenantID == tenantID {
		return true
	}
	return false
}

// CanAccessTenant checks if the user can access a specific tenant
func (ui *UserInfo) CanAccessTenant(tenantID uuid.UUID) bool {
	// Admin can access all tenants
	if ui.IsAdminUser() {
		return true
	}
	// Regular users can only access their own tenant
	return ui.TenantID != nil && *ui.TenantID == tenantID
}

// UserProfile represents the user profile stored in Redis
type UserProfile struct {
	CognitoID   string                 `json:"cognito_id"`
	Email       string                 `json:"email"`
	Username    string                 `json:"username"`
	Role        string                 `json:"role"`
	TenantID    *uuid.UUID             `json:"tenant_id,omitempty"`
	IsAdmin     bool                   `json:"is_admin"`
	Permissions []string               `json:"permissions,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// TokenSession represents a session stored in Redis (token hash as key, no token stored)
type TokenSession struct {
	UserProfile UserProfile `json:"user_profile"`
	CreatedAt   time.Time   `json:"created_at"`
	LastUsedAt  time.Time   `json:"last_used_at"`
	ExpiresAt   time.Time   `json:"expires_at"`
	SessionID   string      `json:"session_id"`
}

// IsExpired checks if the session has expired
func (ts *TokenSession) IsExpired() bool {
	return time.Now().After(ts.ExpiresAt)
}

// UpdateLastUsed updates the last used timestamp
func (ts *TokenSession) UpdateLastUsed() {
	ts.LastUsedAt = time.Now()
}
