package handlers

import (
	"errors"
	"net/http"
	"time"

	"github.com/ArowuTest/promo-backend/internal/config"
	"github.com/ArowuTest/promo-backend/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ----- Payloads -----
// The JSON shape for creating/updating a prize structure.
type PrizeTierPayload struct {
	Name          string `json:"name" binding:"required"`
	Quantity      int    `json:"quantity" binding:"required,min=1"`
	RunnerUpCount int    `json:"runner_up_count" binding:"required,min=0"`
}

type PrizeStructureCreatePayload struct {
	EffectiveStart string               `json:"effective_start" binding:"required,datetime=2006-01-02"`
	EffectiveEnd   string               `json:"effective_end" binding:"required,datetime=2006-01-02"`
	Tiers          []PrizeTierPayload   `json:"tiers" binding:"required,dive,required"`
}

type PrizeStructureUpdatePayload struct {
	EffectiveStart *string              `json:"effective_start,omitempty" binding:"omitempty,datetime=2006-01-02"`
	EffectiveEnd   *string              `json:"effective_end,omitempty" binding:"omitempty,datetime=2006-01-02"`
	Tiers          *[]PrizeTierPayload  `json:"tiers,omitempty"`
}

// ----- Handlers -----
// ListPrizeStructures: GET /api/v1/prize-structures
func ListPrizeStructures(c *gin.Context) {
	var psList []models.PrizeStructure
	if err := config.DB.Preload("Tiers").Find(&psList).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list prize structures: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, psList)
}

// GetPrizeStructure: GET /api/v1/prize-structures/:id
func GetPrizeStructure(c *gin.Context) {
	idStr := c.Param("id")
	psID, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid prize structure ID"})
		return
	}

	var ps models.PrizeStructure
	if err := config.DB.Preload("Tiers").
		First(&ps, "id = ?", psID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "prize structure not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "db error: " + err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, ps)
}

// CreatePrizeStructure: POST /api/v1/prize-structures
func CreatePrizeStructure(c *gin.Context) {
	var payload PrizeStructureCreatePayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload: " + err.Error()})
		return
	}

	startDate, err := time.Parse("2006-01-02", payload.EffectiveStart)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid effective_start format"})
		return
	}
	endDate, err := time.Parse("2006-01-02", payload.EffectiveEnd)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid effective_end format"})
		return
	}
	if endDate.Before(startDate) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "effective_end must be >= effective_start"})
		return
	}

	ps := models.PrizeStructure{
		ID:             uuid.New(),
		EffectiveStart: startDate,
		EffectiveEnd:   endDate,
	}

	// Build PrizeTier slices
	for _, tierPayload := range payload.Tiers {
		ps.Tiers = append(ps.Tiers, models.PrizeTier{
			ID:               uuid.New(),
			Name:             tierPayload.Name,
			Quantity:         tierPayload.Quantity,
			RunnerUpCount:    tierPayload.RunnerUpCount,
		})
	}

	if err := config.DB.Create(&ps).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create prize structure: " + err.Error()})
		return
	}
	c.JSON(http.StatusCreated, ps)
}

// UpdatePrizeStructure: PUT /api/v1/prize-structures/:id
func UpdatePrizeStructure(c *gin.Context) {
	idStr := c.Param("id")
	psID, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid prize structure ID"})
		return
	}

	// Fetch existing
	var existing models.PrizeStructure
	if err := config.DB.Preload("Tiers").
		First(&existing, "id = ?", psID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "prize structure not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "db error: " + err.Error()})
		}
		return
	}

	var payload PrizeStructureUpdatePayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload: " + err.Error()})
		return
	}

	// Update dates if provided
	if payload.EffectiveStart != nil {
		startDate, err := time.Parse("2006-01-02", *payload.EffectiveStart)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid effective_start format"})
			return
		}
		existing.EffectiveStart = startDate
	}
	if payload.EffectiveEnd != nil {
		endDate, err := time.Parse("2006-01-02", *payload.EffectiveEnd)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid effective_end format"})
			return
		}
		existing.EffectiveEnd = endDate
	}

	// If tiers were provided, we'll replace them wholesale.
	if payload.Tiers != nil {
		// Delete old tiers
		config.DB.Delete(&models.PrizeTier{}, "prize_structure_id = ?", existing.ID)

		// Build new Tier slice
		var newTiers []models.PrizeTier
		for _, tierPayload := range *payload.Tiers {
			newTiers = append(newTiers, models.PrizeTier{
				ID:               uuid.New(),
				PrizeStructureID: existing.ID,
				Name:             tierPayload.Name,
				Quantity:         tierPayload.Quantity,
				RunnerUpCount:    tierPayload.RunnerUpCount,
			})
		}
		existing.Tiers = newTiers
	}

	if err := config.DB.Session(&gorm.Session{FullSaveAssociations: true}).Save(&existing).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update prize structure: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, existing)
}

// DeletePrizeStructure: DELETE /api/v1/prize-structures/:id
func DeletePrizeStructure(c *gin.Context) {
	idStr := c.Param("id")
	psID, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid prize structure ID"})
		return
	}

	// GORM’s OnDelete:CASCADE on PrizeTier means tiers go away automatically.
	if err := config.DB.Delete(&models.PrizeStructure{}, "id = ?", psID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete prize structure: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "PrizeStructure deleted"})
}
