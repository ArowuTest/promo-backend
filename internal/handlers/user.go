package handlers

import (
	"net/http"
	"time"

	"github.com/ArowuTest/promo-backend/internal/config"
	"github.com/ArowuTest/promo-backend/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// userRequest represents the JSON payload for creating/updating a user.
type userRequest struct {
	Username string `json:"username" binding:"required"`
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password,omitempty"`
	Role     string `json:"role" binding:"required,oneof=SUPERADMIN ADMIN SENIORUSER WINNERREPORTS ALLREPORTS"`
	Status   string `json:"status" binding:"required,oneof=Active Inactive Locked"`
}

// ListUsers handles GET /api/v1/admin/users
func ListUsers(c *gin.Context) {
	var users []models.AdminUser
	if err := config.DB.Find(&users).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch users: " + err.Error()})
		return
	}

	// Remove password hashes before sending
	var sanitized []gin.H
	for _, u := range users {
		sanitized = append(sanitized, gin.H{
			"id":       u.ID,
			"username": u.Username,
			"email":    u.Email,
			"role":     u.Role,
			"status":   u.Status,
			"created":  u.CreatedAt,
			"updated":  u.UpdatedAt,
		})
	}

	c.JSON(http.StatusOK, sanitized)
}

// GetUser handles GET /api/v1/admin/users/:id
func GetUser(c *gin.Context) {
	idParam := c.Param("id")
	uid, err := uuid.Parse(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	var user models.AdminUser
	if err := config.DB.First(&user, "id = ?", uid).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error: " + err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":       user.ID,
		"username": user.Username,
		"email":    user.Email,
		"role":     user.Role,
		"status":   user.Status,
		"created":  user.CreatedAt,
		"updated":  user.UpdatedAt,
	})
}

// CreateUser handles POST /api/v1/admin/users
func CreateUser(c *gin.Context) {
	var req userRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payload: " + err.Error()})
		return
	}

	// Check if username or email already exists
	var existing models.AdminUser
	if err := config.DB.Where("username = ? OR email = ?", req.Username, req.Email).First(&existing).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "Username or email already in use"})
		return
	} else if err != gorm.ErrRecordNotFound {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error: " + err.Error()})
		return
	}

	// Hash the password
	pwHash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash password"})
		return
	}

	newUser := models.AdminUser{
		ID:           uuid.New(),
		Username:     req.Username,
		Email:        req.Email,
		PasswordHash: string(pwHash),
		Role:         models.AdminUserRole(req.Role),
		Status:       models.UserStatus(req.Status),
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	if err := config.DB.Create(&newUser).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user: " + err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"id":       newUser.ID,
		"username": newUser.Username,
		"email":    newUser.Email,
		"role":     newUser.Role,
		"status":   newUser.Status,
	})
}

// UpdateUser handles PUT /api/v1/admin/users/:id
func UpdateUser(c *gin.Context) {
	idParam := c.Param("id")
	uid, err := uuid.Parse(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	var req userRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payload: " + err.Error()})
		return
	}

	var existing models.AdminUser
	if err := config.DB.First(&existing, "id = ?", uid).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error: " + err.Error()})
		}
		return
	}

	// Role‐change rules
	requestorRole := c.MustGet("role").(string)
	if existing.Role == models.RoleSuperAdmin && requestorRole != string(models.RoleSuperAdmin) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Cannot modify another SUPERADMIN unless you are SUPERADMIN"})
		return
	}
	if req.Role == string(models.RoleSuperAdmin) && requestorRole != string(models.RoleSuperAdmin) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Only a SUPERADMIN can promote another user to SUPERADMIN"})
		return
	}

	existing.Username = req.Username
	existing.Email = req.Email
	existing.Role = models.AdminUserRole(req.Role)
	existing.Status = models.UserStatus(req.Status)
	existing.UpdatedAt = time.Now()

	if err := config.DB.Save(&existing).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update user: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":       existing.ID,
		"username": existing.Username,
		"email":    existing.Email,
		"role":     existing.Role,
		"status":   existing.Status,
	})
}

// DeleteUser handles DELETE /api/v1/admin/users/:id
func DeleteUser(c *gin.Context) {
	idParam := c.Param("id")
	uid, err := uuid.Parse(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	var existing models.AdminUser
	if err := config.DB.First(&existing, "id = ?", uid).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error: " + err.Error()})
		}
		return
	}

	requestorRole := c.MustGet("role").(string)
	if existing.Role == models.RoleSuperAdmin {
		if requestorRole != string(models.RoleSuperAdmin) {
			c.JSON(http.StatusForbidden, gin.H{"error": "Only a SUPERADMIN can delete a SUPERADMIN"})
			return
		}
		var count int64
		config.DB.Model(&models.AdminUser{}).
			Where("role = ?", models.RoleSuperAdmin).
			Count(&count)
		if count <= 1 {
			c.JSON(http.StatusForbidden, gin.H{"error": "Cannot delete the last remaining SUPERADMIN"})
			return
		}
	}

	if err := config.DB.Delete(&models.AdminUser{}, "id = ?", uid).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete user: " + err.Error()})
		return
	}

	c.Status(http.StatusNoContent)
}
