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
