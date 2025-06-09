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

type prizeStructureRequest struct {
	Name         string   `json:"name" binding:"required"`
	Effective    string   `json:"effective" binding:"required"`
	EligibleDays []string `json:"eligible_days" binding:"required,min=1"`
	Tiers        []struct {
		TierName      string `json:"tier_name" binding:"required"`
		Amount        int    `json:"amount" binding:"required,gte=0"`
		Quantity      int    `json:"quantity" binding:"required,gte=1"`
		RunnerUpCount int    `json:"runner_up_count" binding:"required,gte=0"`
		OrderIndex    int    `json:"order_index" binding:"required,gte=1"`
	} `json:"tiers" binding:"required,min=1,dive"`
}

func CreatePrizeStructure(c *gin.Context) {
	var req prizeStructureRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payload: " + err.Error()}); return
	}
	effDate, err := time.Parse("2006-01-02", req.Effective)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid effective date; use yyyy-MM-dd"}); return
	}
	var tiers []models.PrizeTier
	for _, t := range req.Tiers {
		tiers = append(tiers, models.PrizeTier{ID: uuid.New(), TierName: t.TierName, Amount: t.Amount, Quantity: t.Quantity, RunnerUpCount: t.RunnerUpCount, OrderIndex: t.OrderIndex})
	}
	ps := models.PrizeStructure{ID: uuid.New(), Name: req.Name, Effective: effDate, EligibleDays: req.EligibleDays, Tiers: tiers}
	if err := config.DB.Create(&ps).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create prize structure: " + err.Error()}); return
	}
	c.JSON(http.StatusCreated, ps)
}

func GetPrizeStructure(c *gin.Context) {
	idParam := c.Param("id")
	pid, err := uuid.Parse(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid prize structure ID"}); return
	}
	var ps models.PrizeStructure
	if err := config.DB.
		Preload("Tiers", func(db *gorm.DB) *gorm.DB {
			return db.Order("prize_tiers.order_index asc")
		}).
		First(&ps, "id = ?", pid).Error; err != nil {
		if err == gorm.ErrRecordNotFound { c.JSON(http.StatusNotFound, gin.H{"error": "Prize structure not found"})
		} else { c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error: " + err.Error()}) }
		return
	}
	c.JSON(http.StatusOK, ps)
}

func ListPrizeStructures(c *gin.Context) {
	dateQuery := c.Query("date")
	if dateQuery != "" {
		parsedDate, err := time.Parse("2006-01-02", dateQuery)
		if err != nil { c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid date format; use yyyy-mm-dd"}); return }
		dayOfWeek := parsedDate.Weekday().String()
		var validStructures []models.PrizeStructure
		if err := config.DB.
			Preload("Tiers", func(db *gorm.DB) *gorm.DB {
				return db.Order("prize_tiers.order_index asc")
			}).
			Where("effective <= ? AND ? = ANY(eligible_days)", parsedDate, dayOfWeek).
			Order("effective desc").
			Find(&validStructures).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error fetching valid structures: " + err.Error()}); return
		}
		c.JSON(http.StatusOK, validStructures)
		return
	}

	var all []models.PrizeStructure
	if err := config.DB.
		Preload("Tiers", func(db *gorm.DB) *gorm.DB {
			return db.Order("prize_tiers.order_index asc")
		}).
		Order("name asc").
		Find(&all).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list prize structures: " + err.Error()}); return
	}
	c.JSON(http.StatusOK, all)
}

func UpdatePrizeStructure(c *gin.Context) {
	idParam := c.Param("id")
	pid, err := uuid.Parse(idParam)
	if err != nil { c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid prize structure ID"}); return }
	var req prizeStructureRequest
	if err := c.ShouldBindJSON(&req); err != nil { c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payload: " + err.Error()}); return }
	effDate, err := time.Parse("2006-01-02", req.Effective)
	if err != nil { c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid effective date; use yyyy-MM-dd"}); return }
	
	tx := config.DB.Begin()
	var existing models.PrizeStructure
	if err := tx.First(&existing, "id = ?", pid).Error; err != nil {
		tx.Rollback(); c.JSON(http.StatusNotFound, gin.H{"error": "Prize structure not found"}); return
	}

	existing.Name = req.Name
	existing.Effective = effDate
	existing.EligibleDays = req.EligibleDays

	if err := tx.Save(&existing).Error; err != nil {
		tx.Rollback(); c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update prize structure details"}); return
	}

	if err := tx.Where("prize_structure_id = ?", pid).Delete(&models.PrizeTier{}).Error; err != nil {
		tx.Rollback(); c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete old tiers"}); return
	}

	for _, t := range req.Tiers {
		newTier := models.PrizeTier{ID: uuid.New(), PrizeStructureID: pid, TierName: t.TierName, Amount: t.Amount, Quantity: t.Quantity, RunnerUpCount: t.RunnerUpCount, OrderIndex: t.OrderIndex}
		if err := tx.Create(&newTier).Error; err != nil {
			tx.Rollback(); c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create new tier"}); return
		}
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Transaction commit error"}); return
	}

	var updatedPs models.PrizeStructure
	// This is the line that has been corrected to fix the compiler error.
	config.DB.Preload("Tiers", func(db *gorm.DB) *gorm.DB { return db.Order("order_index asc") }).First(&updatedPs, "id = ?", pid)
	c.JSON(http.StatusOK, updatedPs)
}

func DeletePrizeStructure(c *gin.Context) {
	idParam := c.Param("id")
	pid, err := uuid.Parse(idParam)
	if err != nil { c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid prize structure ID"}); return }
	var drawCount int64
	config.DB.Model(&models.Draw{}).Where("prize_structure_id = ?", pid).Count(&drawCount)
	if drawCount > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "Cannot delete structure that is already in use by a draw"}); return
	}
	if err := config.DB.Select("Tiers").Delete(&models.PrizeStructure{ID: pid}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete prize structure: " + err.Error()}); return
	}
	c.Status(http.StatusNoContent)
}