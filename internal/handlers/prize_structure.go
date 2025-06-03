package handlers

import (
	"net/http"
	"time"

	"github.com/ArowuTest/promo-backend/internal/config"
	"github.com/ArowuTest/promo-backend/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// prizeStructureRequest represents the JSON payload for creating/updating a prize structure.
type prizeStructureRequest struct {
	// Effective is the date (YYYY-MM-DD) on which this structure applies.
	Effective string `json:"effective" binding:"required"`
	Name      string `json:"name" binding:"required"`
	Tiers     []struct {
		TierName      string `json:"tier_name" binding:"required"`
		Amount        int    `json:"amount" binding:"required"`
		Quantity      int    `json:"quantity" binding:"required"`
		RunnerUpCount int    `json:"runner_up_count" binding:"required"`
		OrderIndex    int    `json:"order_index" binding:"required"`
	} `json:"tiers" binding:"required,dive"`
}

// CreatePrizeStructure handles POST /api/v1/prize_structures.
func CreatePrizeStructure(c *gin.Context) {
	var req prizeStructureRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payload: " + err.Error()})
		return
	}

	// Parse the effective date
	effDate, err := time.Parse("2006-01-02", req.Effective)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid effective date; use YYYY-MM-DD"})
		return
	}

	// Check if a structure already exists for this date
	var existing models.PrizeStructure
	if err := config.DB.
		Where("effective = ?", effDate).
		First(&existing).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "Prize structure already exists for that date"})
		return
	}

	// Build PrizeTier slice (omit CreatedAt/UpdatedAt; GORM will auto-populate)
	var tiers []models.PrizeTier
	for _, t := range req.Tiers {
		tiers = append(tiers, models.PrizeTier{
			ID:               uuid.New(),
			TierName:         t.TierName,
			Amount:           t.Amount,
			Quantity:         t.Quantity,
			RunnerUpCount:    t.RunnerUpCount,
			OrderIndex:       t.OrderIndex,
			PrizeStructureID: uuid.Nil, // will be set once parent is saved
		})
	}

	// Create the PrizeStructure
	ps := models.PrizeStructure{
		ID:        uuid.New(),
		Effective: effDate,
		Name:      req.Name,
		Tiers:     tiers,
	}
	if err := config.DB.Create(&ps).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create prize structure: " + err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"id":        ps.ID,
		"effective": ps.Effective,
		"name":      ps.Name,
		"tiers":     ps.Tiers,
	})
}

// GetPrizeStructure handles GET /api/v1/prize_structures/:id
func GetPrizeStructure(c *gin.Context) {
	idParam := c.Param("id")
	pid, err := uuid.Parse(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid prize structure ID"})
		return
	}

	var ps models.PrizeStructure
	if err := config.DB.
		Preload("Tiers", func(db *gorm.DB) *gorm.DB {
			return db.Order("order_index asc")
		}).
		First(&ps, "id = ?", pid).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Prize structure not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error: " + err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, ps)
}

// UpdatePrizeStructure handles PUT /api/v1/prize_structures/:id
func UpdatePrizeStructure(c *gin.Context) {
	idParam := c.Param("id")
	pid, err := uuid.Parse(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid prize structure ID"})
		return
	}

	var req prizeStructureRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payload: " + err.Error()})
		return
	}

	// Parse the effective date
	effDate, err := time.Parse("2006-01-02", req.Effective)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid effective date; use YYYY-MM-DD"})
		return
	}

	var existing models.PrizeStructure
	if err := config.DB.
		Preload("Tiers").
		First(&existing, "id = ?", pid).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Prize structure not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error: " + err.Error()})
		}
		return
	}

	// Check if any draw has used this structure
	var drawCount int64
	config.DB.Model(&models.Draw{}).
		Where("prize_structure_id = ?", pid).
		Count(&drawCount)
	if drawCount > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "Cannot modify structure already used in a draw"})
		return
	}

	// Build the new tiers slice (omit CreatedAt/UpdatedAt)
	var newTiers []models.PrizeTier
	for _, t := range req.Tiers {
		newTiers = append(newTiers, models.PrizeTier{
			ID:               uuid.New(),
			PrizeStructureID: existing.ID,
			TierName:         t.TierName,
			Amount:           t.Amount,
			Quantity:         t.Quantity,
			RunnerUpCount:    t.RunnerUpCount,
			OrderIndex:       t.OrderIndex,
		})
	}

	existing.Effective = effDate
	existing.Name = req.Name

	// Use a transaction to replace tiers atomically
	tx := config.DB.Begin()
	if err := tx.Save(&existing).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update prize structure: " + err.Error()})
		return
	}

	// Delete old tiers
	if err := tx.Where("prize_structure_id = ?", existing.ID).Delete(&models.PrizeTier{}).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to clear old tiers: " + err.Error()})
		return
	}

	// Insert new tiers
	for i := range newTiers {
		newTiers[i].PrizeStructureID = existing.ID
		if err := tx.Create(&newTiers[i]).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to insert new tiers: " + err.Error()})
			return
		}
	}
	tx.Commit()

	c.JSON(http.StatusOK, gin.H{
		"id":        existing.ID,
		"effective": existing.Effective,
		"name":      existing.Name,
		"tiers":     newTiers,
	})
}

// DeletePrizeStructure handles DELETE /api/v1/prize_structures/:id
func DeletePrizeStructure(c *gin.Context) {
	idParam := c.Param("id")
	pid, err := uuid.Parse(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid prize structure ID"})
		return
	}

	// Check for any draws that used this structure
	var drawCount int64
	config.DB.Model(&models.Draw{}).
		Where("prize_structure_id = ?", pid).
		Count(&drawCount)
	if drawCount > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "Cannot delete structure in use by a draw"})
		return
	}

	// Delete the structure (tiers will cascade)
	if err := config.DB.Delete(&models.PrizeStructure{}, "id = ?", pid).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete prize structure: " + err.Error()})
		return
	}

	c.Status(http.StatusNoContent)
}

// ListPrizeStructures handles GET /api/v1/prize_structures?date=YYYY-MM-DD
func ListPrizeStructures(c *gin.Context) {
	dateQuery := c.Query("date")
	if dateQuery != "" {
		parsed, err := time.Parse("2006-01-02", dateQuery)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid date format; use YYYY-MM-DD"})
			return
		}
		var ps models.PrizeStructure
		if err := config.DB.
			Preload("Tiers", func(db *gorm.DB) *gorm.DB {
				return db.Order("order_index asc")
			}).
			Where("effective = ?", parsed).
			First(&ps).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				c.JSON(http.StatusNotFound, gin.H{"error": "No prize structure for that date"})
			} else {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error: " + err.Error()})
			}
			return
		}
		c.JSON(http.StatusOK, ps)
		return
	}

	var all []models.PrizeStructure
	if err := config.DB.
		Preload("Tiers", func(db *gorm.DB) *gorm.DB {
			return db.Order("order_index asc")
		}).
		Order("effective desc").
		Find(&all).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list prize structures: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, all)
}
