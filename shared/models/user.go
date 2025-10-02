package models

import (
	"time"

	"github.com/google/uuid"
)

// User represents a tenant user record
type User struct {
	CognitoID   string     `json:"cognito_id" gorm:"type:varchar(255);primaryKey"`
	TenantID    uuid.UUID  `json:"tenant_id" gorm:"type:uuid;index"`
	Role        UserRole   `json:"role" gorm:"type:user_role;default:user"`
	CreatedAt   time.Time  `json:"created_at" gorm:"default:CURRENT_TIMESTAMP"`
	LastLoginAt *time.Time `json:"last_login_at,omitempty"`

	Tenant           *Tenant           `json:"tenant,omitempty" gorm:"foreignKey:TenantID"`
	LocationSessions []LocationSession `json:"location_sessions,omitempty" gorm:"foreignKey:CognitoUserID;references:CognitoID"`
}

type UserRole string

const (
	RoleTenantOwner UserRole = "tenant_owner"
	RoleUser        UserRole = "user"
)

func (User) TableName() string {
	return "users"
}

func (u *User) GetID() string {
	return u.CognitoID
}

// Admin represents a platform administrator
type Admin struct {
	CognitoID   string     `json:"cognito_id" gorm:"type:varchar(255);primaryKey"`
	CreatedAt   time.Time  `json:"created_at" gorm:"default:CURRENT_TIMESTAMP"`
	LastLoginAt *time.Time `json:"last_login_at,omitempty"`
	Metadata    string     `json:"metadata" gorm:"type:jsonb;default:'{}'"`
}

func (Admin) TableName() string {
	return "admins"
}

// UserInfo represents user information from JWT claims
type UserInfo struct {
	CognitoID string     `json:"cognito_id"`
	Email     string     `json:"email"`
	Role      UserRole   `json:"role"`
	TenantID  *uuid.UUID `json:"tenant_id,omitempty"`
	IsAdmin   bool       `json:"is_admin"`
}

func (ui *UserInfo) IsAdminUser() bool {
	return ui.IsAdmin
}

func (ui *UserInfo) IsTenantOwner() bool {
	return ui.Role == RoleTenantOwner
}

func (ui *UserInfo) CanManageTenant(tenantID uuid.UUID) bool {
	if ui.IsAdminUser() {
		return true
	}
	if ui.IsTenantOwner() && ui.TenantID != nil && *ui.TenantID == tenantID {
		return true
	}
	return false
}

func (ui *UserInfo) CanAccessTenant(tenantID uuid.UUID) bool {
	if ui.IsAdminUser() {
		return true
	}
	return ui.TenantID != nil && *ui.TenantID == tenantID
}

// UserProfile represents the user profile stored in Redis
type UserProfile struct {
	CognitoID string                 `json:"cognito_id"`
	Email     string                 `json:"email"`
	Role      string                 `json:"role"`
	TenantID  *uuid.UUID             `json:"tenant_id,omitempty"`
	IsAdmin   bool                   `json:"is_admin"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// TokenSession represents a session stored in Redis
type TokenSession struct {
	UserProfile UserProfile `json:"user_profile"`
	CreatedAt   time.Time   `json:"created_at"`
	LastUsedAt  time.Time   `json:"last_used_at"`
	ExpiresAt   time.Time   `json:"expires_at"`
	SessionID   string      `json:"session_id"`
}

func (ts *TokenSession) IsExpired() bool {
	return time.Now().After(ts.ExpiresAt)
}

func (ts *TokenSession) UpdateLastUsed() {
	ts.LastUsedAt = time.Now()
}
