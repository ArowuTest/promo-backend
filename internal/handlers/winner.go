package handlers

import (
	"net/http"

	"github.com/ArowuTest/promo-backend/internal/config"
	"github.com/ArowuTest/promo-backend/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ListWinners handles GET /api/v1/draws/:id/winners
func ListWinners(c *gin.Context) {
	drawIDStr := c.Param("id")
	drawID, err := uuid.Parse(drawIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid draw ID format"})
		return
	}

	var draw models.Draw
	if err := config.DB.First(&draw, "id = ?", drawID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Draw not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error fetching draw"})
		}
		return
	}

	var winners []models.Winner
	if err := config.DB.
		Preload("PrizeTier").
		Where("draw_id = ?", drawID).
		Order("position asc").
		Find(&winners).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch winners: " + err.Error()})
		return
	}

	var prizeStruct models.PrizeStructure
	if err := config.DB.
		Preload("Tiers", func(db *gorm.DB) *gorm.DB {
			return db.Order("order_index asc")
		}).
		First(&prizeStruct, "id = ?", draw.PrizeStructureID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not load prize structure for this draw"})
		return
	}

	type winnerResponse struct {
		ID           string `json:"id"`
		MSISDNMasked string `json:"msisdn_masked"`
		MSISDNFull   string `json:"msisdn_full,omitempty"`
		PrizeTier    string `json:"prize_tier"`
		Position     int    `json:"position"`
		IsRunnerUp   bool   `json:"is_runner_up"`
	}

	var resp []winnerResponse
	userRole := c.MustGet("user_role").(string)

	for _, w := range winners {
		wr := winnerResponse{
			ID:           w.ID.String(),
			MSISDNMasked: maskMSISDN(w.MSISDN),
			PrizeTier:    w.PrizeTier.TierName,
			Position:     w.Position,
			IsRunnerUp:   w.IsRunnerUp,
		}
		if userRole == string(models.RoleSuperAdmin) {
			wr.MSISDNFull = w.MSISDN
		}
		resp = append(resp, wr)
	}

	c.JSON(http.StatusOK, gin.H{
		"winners":        resp,
		"prizeStructure": prizeStruct,
	})
}