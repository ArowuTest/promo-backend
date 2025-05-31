// internal/handlers/user.go

package handlers

import (
	"errors"
	"net/http"

	"github.com/ArowuTest/promo-backend/internal/config"
	"github.com/ArowuTest/promo-backend/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// createUserRequest defines the expected JSON payload for creating a user.
type createUserRequest struct {
	Username string              `json:"username" binding:"required"`
	Email    string              `json:"email" binding:"required,email"`
	Password string              `json:"password" binding:"required,min=6"`
	Role     models.AdminUserRole `json:"role" binding:"required"`
	Status   models.UserStatus   `json:"status,omitempty"`
}

// CreateUser handles POST /admin/users
func CreateUser(c *gin.Context) {
	var req createUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payload: " + err.Error()})
		return
	}

	// Validate role
	switch req.Role {
	case models.RoleSuperAdmin, models.RoleAdmin, models.RoleSeniorUser,
		models.RoleWinnerReports, models.RoleAllReportsUser:
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user role"})
		return
	}
	// Validate status if provided
	if req.Status != "" {
		switch req.Status {
		case models.StatusActive, models.StatusInactive, models.StatusLocked:
			// ok
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user status"})
			return
		}
	}

	// Hash password
	hashed, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash password"})
		return
	}

	user := models.AdminUser{
		Username:     req.Username,
		Email:        req.Email,
		PasswordHash: string(hashed),
		Role:         req.Role,
		Status:       req.Status,
	}
	// If status not provided, default to Active
	if user.Status == "" {
		user.Status = models.StatusActive
	}

	if err := config.DB.Create(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user: " + err.Error()})
		return
	}
	// Never return the password hash in JSON
	user.PasswordHash = ""
	c.JSON(http.StatusCreated, user)
}

// ListUsers handles GET /admin/users
func ListUsers(c *gin.Context) {
	var users []models.AdminUser
	if err := config.DB.Find(&users).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch users: " + err.Error()})
		return
	}
	// Omit PasswordHash from response
	for i := range users {
		users[i].PasswordHash = ""
	}
	c.JSON(http.StatusOK, users)
}

// GetUser handles GET /admin/users/:id
func GetUser(c *gin.Context) {
	idStr := c.Param("id")
	userID, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}
	var user models.AdminUser
	if err := config.DB.First(&user, "id = ?", userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		}
		return
	}
	user.PasswordHash = ""
	c.JSON(http.StatusOK, user)
}

// updateUserRequest defines JSON payload for updating a user.
type updateUserRequest struct {
	Username string              `json:"username,omitempty"`
	Email    string              `json:"email,omitempty"`
	Role     models.AdminUserRole `json:"role,omitempty"`
	Status   models.UserStatus   `json:"status,omitempty"`
	Password string              `json:"password,omitempty"`
}

// UpdateUser handles PUT /admin/users/:id
func UpdateUser(c *gin.Context) {
	idStr := c.Param("id")
	userID, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	var existing models.AdminUser
	if err := config.DB.First(&existing, "id = ?", userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		}
		return
	}

	var req updateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payload: " + err.Error()})
		return
	}

	// Update fields if provided
	if req.Username != "" {
		existing.Username = req.Username
	}
	if req.Email != "" {
		existing.Email = req.Email
	}
	if req.Role != "" {
		switch req.Role {
		case models.RoleSuperAdmin, models.RoleAdmin, models.RoleSeniorUser,
			models.RoleWinnerReports, models.RoleAllReportsUser:
			existing.Role = req.Role
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user role"})
			return
		}
	}
	if req.Status != "" {
		switch req.Status {
		case models.StatusActive, models.StatusInactive, models.StatusLocked:
			existing.Status = req.Status
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user status"})
			return
		}
	}
	if req.Password != "" {
		if len(req.Password) < 6 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Password must be at least 6 characters"})
			return
		}
		hashed, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash password"})
			return
		}
		existing.PasswordHash = string(hashed)
	}

	if err := config.DB.Save(&existing).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update user: " + err.Error()})
		return
	}
	existing.PasswordHash = ""
	c.JSON(http.StatusOK, existing)
}

// DeleteUser handles DELETE /admin/users/:id
func DeleteUser(c *gin.Context) {
	idStr := c.Param("id")
	userID, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}
	var user models.AdminUser
	if err := config.DB.First(&user, "id = ?", userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		}
		return
	}
	if err := config.DB.Delete(&models.AdminUser{}, "id = ?", userID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete user: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "User deleted successfully"})
}
