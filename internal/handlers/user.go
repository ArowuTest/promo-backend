package handlers

import (
	"errors"
	"net/http"

	"github.com/ArowuTest/promo-backend/internal/config"
	"github.com/ArowuTest/promo-backend/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// CreateUser creates a new admin user.
func CreateUser(c *gin.Context) {
	var input struct {
		Username string            `json:"username" binding:"required"`
		Email    string            `json:"email" binding:"required,email"`
		Password string            `json:"password" binding:"required,min=6"`
		Role     models.AdminUserRole `json:"role" binding:"required"`
		Status   models.UserStatus `json:"status,omitempty"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payload: " + err.Error()})
		return
	}

	// Validate role
	switch input.Role {
	case models.RoleSuperAdmin, models.RoleAdmin, models.RoleSeniorUser, models.RoleWinnerReports, models.RoleAllReportsUser:
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid role"})
		return
	}

	// Hash password
	hashed, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash password"})
		return
	}

	newUser := models.AdminUser{
		ID:           uuid.New(),
		Username:     input.Username,
		Email:        input.Email,
		PasswordHash: string(hashed),
		Role:         input.Role,
	}
	// Status
	if input.Status == "" {
		newUser.Status = models.StatusActive
	} else {
		switch input.Status {
		case models.StatusActive, models.StatusInactive, models.StatusLocked:
			newUser.Status = input.Status
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid status"})
			return
		}
	}

	if err := config.DB.Create(&newUser).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user: " + err.Error()})
		return
	}
	// Remove PasswordHash from response
	newUser.PasswordHash = ""
	c.JSON(http.StatusCreated, newUser)
}

// ListUsers returns all admin users.
func ListUsers(c *gin.Context) {
	var users []models.AdminUser
	if err := config.DB.Find(&users).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list users: " + err.Error()})
		return
	}
	// Omit PasswordHash in response
	for i := range users {
		users[i].PasswordHash = ""
	}
	c.JSON(http.StatusOK, users)
}

// GetUser returns one user by ID.
func GetUser(c *gin.Context) {
	idStr := c.Param("id")
	uid, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid UUID format"})
		return
	}
	var user models.AdminUser
	if err := config.DB.First(&user, "id = ?", uid).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "DB error: " + err.Error()})
		}
		return
	}
	user.PasswordHash = ""
	c.JSON(http.StatusOK, user)
}

// UpdateUser updates an existing user.
func UpdateUser(c *gin.Context) {
	idStr := c.Param("id")
	uid, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid UUID"})
		return
	}

	var existing models.AdminUser
	if err := config.DB.First(&existing, "id = ?", uid).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "DB error: " + err.Error()})
		}
		return
	}

	var payload struct {
		Username string            `json:"username,omitempty"`
		Email    string            `json:"email,omitempty"`
		Role     models.AdminUserRole `json:"role,omitempty"`
		Status   models.UserStatus `json:"status,omitempty"`
		Password string            `json:"password,omitempty"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payload: " + err.Error()})
		return
	}

	if payload.Username != "" {
		existing.Username = payload.Username
	}
	if payload.Email != "" {
		existing.Email = payload.Email
	}
	if payload.Role != "" {
		switch payload.Role {
		case models.RoleSuperAdmin, models.RoleAdmin, models.RoleSeniorUser, models.RoleWinnerReports, models.RoleAllReportsUser:
			existing.Role = payload.Role
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid role"})
			return
		}
	}
	if payload.Status != "" {
		switch payload.Status {
		case models.StatusActive, models.StatusInactive, models.StatusLocked:
			existing.Status = payload.Status
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid status"})
			return
		}
	}
	if payload.Password != "" {
		if len(payload.Password) < 6 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Password too short"})
			return
		}
		h, err := bcrypt.GenerateFromPassword([]byte(payload.Password), bcrypt.DefaultCost)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash new password"})
			return
		}
		existing.PasswordHash = string(h)
	}

	if err := config.DB.Save(&existing).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update: " + err.Error()})
		return
	}
	existing.PasswordHash = ""
	c.JSON(http.StatusOK, existing)
}

// DeleteUser removes a user by ID.
func DeleteUser(c *gin.Context) {
	idStr := c.Param("id")
	uid, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid UUID"})
		return
	}
	if err := config.DB.Delete(&models.AdminUser{}, "id = ?", uid).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Deleted"})
}
