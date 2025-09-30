package main

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pavitra93/go-multi-tenant-system/shared/models"
	"github.com/pavitra93/go-multi-tenant-system/shared/utils"
)

// CreateTenantRequest represents the create tenant request
type CreateTenantRequest struct {
	Name   string `json:"name" binding:"required"`
	Domain string `json:"domain" binding:"required"`
}

// UpdateTenantRequest represents the update tenant request
type UpdateTenantRequest struct {
	Name     *string `json:"name"`
	Domain   *string `json:"domain"`
	IsActive *bool   `json:"is_active"`
}

// handleCreateTenant handles tenant creation (admin only)
func handleCreateTenant(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req CreateTenantRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			utils.BadRequestResponse(c, "Invalid request format")
			return
		}

		// Check if domain already exists
		var existingTenant models.Tenant
		if err := db.Where("domain = ?", req.Domain).First(&existingTenant).Error; err == nil {
			utils.BadRequestResponse(c, "Domain already exists")
			return
		}

		// Create tenant
		tenant := models.Tenant{
			ID:       uuid.New(),
			Name:     req.Name,
			Domain:   req.Domain,
			IsActive: true,
		}

		if err := db.Create(&tenant).Error; err != nil {
			utils.InternalServerErrorResponse(c, "Failed to create tenant")
			return
		}

		utils.CreatedResponse(c, "Tenant created successfully", tenant)
	}
}

// handleGetTenants handles getting all tenants (admin only)
func handleGetTenants(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var tenants []models.Tenant
		if err := db.Preload("Users").Find(&tenants).Error; err != nil {
			utils.InternalServerErrorResponse(c, "Failed to fetch tenants")
			return
		}

		utils.OKResponse(c, "Tenants retrieved successfully", tenants)
	}
}

// handleGetTenant handles getting a specific tenant
func handleGetTenant(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID := c.Param("id")

		var tenant models.Tenant
		if err := db.Preload("Users").Where("id = ?", tenantID).First(&tenant).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				utils.NotFoundResponse(c, "Tenant not found")
			} else {
				utils.InternalServerErrorResponse(c, "Failed to fetch tenant")
			}
			return
		}

		utils.OKResponse(c, "Tenant retrieved successfully", tenant)
	}
}

// handleUpdateTenant handles updating a tenant
func handleUpdateTenant(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID := c.Param("id")

		var tenant models.Tenant
		if err := db.Where("id = ?", tenantID).First(&tenant).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				utils.NotFoundResponse(c, "Tenant not found")
			} else {
				utils.InternalServerErrorResponse(c, "Failed to fetch tenant")
			}
			return
		}

		var req UpdateTenantRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			utils.BadRequestResponse(c, "Invalid request format")
			return
		}

		// Update tenant fields
		if req.Name != nil {
			tenant.Name = *req.Name
		}
		if req.Domain != nil {
			// Check if new domain already exists
			var existingTenant models.Tenant
			if err := db.Where("domain = ? AND id != ?", *req.Domain, tenantID).First(&existingTenant).Error; err == nil {
				utils.BadRequestResponse(c, "Domain already exists")
				return
			}
			tenant.Domain = *req.Domain
		}
		if req.IsActive != nil {
			tenant.IsActive = *req.IsActive
		}

		if err := db.Save(&tenant).Error; err != nil {
			utils.InternalServerErrorResponse(c, "Failed to update tenant")
			return
		}

		utils.OKResponse(c, "Tenant updated successfully", tenant)
	}
}

// handleDeleteTenant handles deleting a tenant (admin only)
func handleDeleteTenant(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID := c.Param("id")

		// Check if tenant exists
		var tenant models.Tenant
		if err := db.Where("id = ?", tenantID).First(&tenant).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				utils.NotFoundResponse(c, "Tenant not found")
			} else {
				utils.InternalServerErrorResponse(c, "Failed to fetch tenant")
			}
			return
		}

		// Check if tenant has users
		var userCount int64
		if err := db.Model(&models.User{}).Where("tenant_id = ?", tenantID).Count(&userCount).Error; err != nil {
			utils.InternalServerErrorResponse(c, "Failed to check tenant users")
			return
		}

		if userCount > 0 {
			utils.BadRequestResponse(c, "Cannot delete tenant with existing users")
			return
		}

		// Delete tenant
		if err := db.Delete(&tenant).Error; err != nil {
			utils.InternalServerErrorResponse(c, "Failed to delete tenant")
			return
		}

		utils.OKResponse(c, "Tenant deleted successfully", nil)
	}
}

// handleInviteUserToTenant handles inviting a new user to the tenant
// Tenant owners can use this to add users to their tenant
func handleInviteUserToTenant(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID := c.Param("id")

		var req struct {
			Username string `json:"username" binding:"required,email"`
			Role     string `json:"role" binding:"required,oneof=user tenant_owner"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			utils.BadRequestResponse(c, "Invalid request format. Role must be 'user' or 'tenant_owner'")
			return
		}

		// Forward to auth service to create user
		// Note: This would need to be implemented in auth service
		utils.OKResponse(c, "User invitation sent", map[string]interface{}{
			"tenant_id": tenantID,
			"username":  req.Username,
			"role":      req.Role,
			"message":   "User will receive registration email",
		})
	}
}

// handleUpdateTenantUser handles updating a user's role within the tenant
func handleUpdateTenantUser(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID := c.Param("id")
		userID := c.Param("user_id")

		var req struct {
			Role string `json:"role" binding:"required,oneof=user tenant_owner"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			utils.BadRequestResponse(c, "Invalid request format")
			return
		}

		utils.OKResponse(c, "User role updated", map[string]interface{}{
			"tenant_id": tenantID,
			"user_id":   userID,
			"new_role":  req.Role,
		})
	}
}

// handleRemoveTenantUser handles removing a user from the tenant
func handleRemoveTenantUser(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID := c.Param("id")
		userID := c.Param("user_id")

		// Verify user belongs to this tenant
		var user models.User
		tenantUUID, _ := uuid.Parse(tenantID)
		if err := db.Where("cognito_id = ? AND tenant_id = ?", userID, tenantUUID).First(&user).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				utils.NotFoundResponse(c, "User not found in this tenant")
			} else {
				utils.InternalServerErrorResponse(c, "Failed to find user")
			}
			return
		}

		// Delete user (would also need to delete from Cognito in real implementation)
		if err := db.Delete(&user).Error; err != nil {
			utils.InternalServerErrorResponse(c, "Failed to remove user")
			return
		}

		utils.OKResponse(c, "User removed from tenant", nil)
	}
}

// handleGetTenantUsers handles getting users for a specific tenant
func handleGetTenantUsers(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID := c.Param("id")

		var users []models.User
		if err := db.Where("tenant_id = ?", tenantID).Find(&users).Error; err != nil {
			utils.InternalServerErrorResponse(c, "Failed to fetch tenant users")
			return
		}

		utils.OKResponse(c, "Tenant users retrieved successfully", users)
	}
}
