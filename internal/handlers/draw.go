package handlers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/ArowuTest/promo-backend/internal/config"
	"github.com/ArowuTest/promo-backend/internal/models"
	"github.com/ArowuTest/promo-backend/internal/posthog"
	"github.com/ArowuTest/promo-backend/internal/rng"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// drawRequest is the JSON payload for executing a draw
type drawRequest struct {
	// DrawDate is expected in "YYYY-MM-DD" format (local date).
	DrawDate string `json:"draw_date" binding:"required"`
}

// ExecuteDraw manually triggers a draw for a given date.
// Only SuperAdmins should call this (enforced in main.go).
func ExecuteDraw(c *gin.Context) {
	var req drawRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payload: " + err.Error()})
		return
	}

	// Parse the draw date (strip time, use midnight local)
	drawDate, err := time.Parse("2006-01-02", req.DrawDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid date format; use YYYY-MM-DD"})
		return
	}

	// Check if a draw already exists on this date
	var existingDraw models.Draw
	if err := config.DB.
		Where("draw_date = ?", drawDate).
		First(&existingDraw).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "Draw already executed for this date"})
		return
	}

	// 1. Fetch eligible entries from PostHog (5pm previous window → 5pm of draw day)
	// For simplicity: we treat “since midnight” to “23:59” as window. Adjust as needed.
	// (User logic: Mon–Fri: prev day 17:00 → today 17:00; Saturday: prev Sat 17:00→Sat 17:00)
	windowStart := drawDate.Add(-1 * 24 * time.Hour).Add(17 * time.Hour) // yesterday 17:00
	windowEnd := drawDate.Add(17 * time.Hour)                           // today    17:00

	phClient, _ := posthog.NewClient(config.Cfg)
	defer phClient.Close()
	entries, err := phClient.FetchEligibleEntries(windowStart, windowEnd)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "PostHog fetch failed: " + err.Error()})
		return
	}
	entryCount := len(entries)

	// 2. Load the PrizeStructure whose EffectiveDate <= drawDate, order by EffectiveDate desc
	var prizeStruct models.PrizeStructure
	if err := config.DB.
		Preload("PrizeTiers", func(db *gorm.DB) *gorm.DB {
			return db.Order("order asc")
		}).
		Where("effective_date <= ?", drawDate).
		Order("effective_date desc").
		First(&prizeStruct).
		Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "No prize structure found for date"})
		return
	}

	// 3. Build weighted draw for each prize tier in order
	winners := []models.Winner{}
	remainingEntries := entries

	for _, tier := range prizeStruct.PrizeTiers {
		// a) Draw main winners
		if tier.Quantity > 0 {
			mainList, err := rng.DrawWeighted(remainingEntries, tier.Quantity)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "RNG error: " + err.Error()})
				return
			}
			for _, msisdn := range mainList {
				winners = append(winners, models.Winner{
					ID:        uuid.New(),
					MSISDN:    msisdn,
					PrizeTier: tier.TierName,
					Position:  "Winner",
				})
			}
			// Remove main winners from remainingEntries
			remainingEntries = filterOut(remainingEntries, mainList)
		}

		// b) Draw runner-ups
		if tier.RunnerUpCount > 0 {
			runList, err := rng.DrawWeighted(remainingEntries, tier.RunnerUpCount)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "RNG error: " + err.Error()})
				return
			}
			for _, msisdn := range runList {
				winners = append(winners, models.Winner{
					ID:        uuid.New(),
					MSISDN:    msisdn,
					PrizeTier: tier.TierName,
					Position:  "RunnerUp",
				})
			}
			// Remove runner-ups as well
			remainingEntries = filterOut(remainingEntries, runList)
		}
	}

	// 4. Persist Draw record
	newDraw := models.Draw{
		ID:               uuid.New(),
		DrawDate:         drawDate,
		PrizeStructureID: prizeStruct.ID,
		EntryCount:       entryCount,
	}
	// AdminUserID from context (set by RequireAuth)
	if userID, ok := c.Get("user_id"); ok {
		parsed, _ := uuid.Parse(userID.(string))
		newDraw.AdminUserID = parsed
	}
	if err := config.DB.Create(&newDraw).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save draw: " + err.Error()})
		return
	}

	// 5. Persist each Winner with DrawID
	for i := range winners {
		winners[i].DrawID = newDraw.ID
		if err := config.DB.Create(&winners[i]).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save winner: " + err.Error()})
			return
		}
	}

	// 6. Mask the MSISDNs in the response (first 3 + last 3)
	type maskedWinner struct {
		PrizeTier string `json:"prize_tier"`
		Position  string `json:"position"`
		MaskedMSISDN string `json:"msisdn"`
	}
	responseWinners := make([]maskedWinner, 0, len(winners))
	for _, w := range winners {
		// assume MSISDN length >= 6
		s := w.MSISDN
		masked := fmt.Sprintf("%s****%s", s[:3], s[len(s)-3:])
		responseWinners = append(responseWinners, maskedWinner{
			PrizeTier:   w.PrizeTier,
			Position:    w.Position,
			MaskedMSISDN: masked,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"draw_id":       newDraw.ID,
		"draw_date":     newDraw.DrawDate.Format("2006-01-02"),
		"entry_count":   entryCount,
		"winners":       responseWinners,
	})
}

// RerunDraw allows an explicit rerun for an existing draw ID (+ “R” suffix in ID).
// It follows the same logic as ExecuteDraw but re‐uses the PrizeStructure & window.
// Only a SuperAdmin should call this.
func RerunDraw(c *gin.Context) {
	idStr := c.Param("id")
	drawID, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid draw ID"})
		return
	}

	// Fetch the existing draw record
	var oldDraw models.Draw
	if err := config.DB.First(&oldDraw, "id = ?", drawID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Draw not found"})
		return
	}

	// Build new “Rerun” draw ID (drawID + "-R")
	rerunID := uuid.New() // for simplicity, generate fresh ID
	drawDate := oldDraw.DrawDate

	// Fetch entries again (same window logic)
	windowStart := drawDate.Add(-1 * 24 * time.Hour).Add(17 * time.Hour)
	windowEnd := drawDate.Add(17 * time.Hour)
	phClient, _ := posthog.NewClient(config.Cfg)
	defer phClient.Close()
	entries, err := phClient.FetchEligibleEntries(windowStart, windowEnd)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "PostHog fetch failed: " + err.Error()})
		return
	}
	entryCount := len(entries)

	// Load the same PrizeStructure
	var prizeStruct models.PrizeStructure
	if err := config.DB.
		Preload("PrizeTiers", func(db *gorm.DB) *gorm.DB {
			return db.Order("order asc")
		}).
		Where("id = ?", oldDraw.PrizeStructureID).
		First(&prizeStruct).
		Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Prize structure not found"})
		return
	}

	winners := []models.Winner{}
	remainingEntries := entries

	for _, tier := range prizeStruct.PrizeTiers {
		// same logic as ExecuteDraw
		if tier.Quantity > 0 {
			mainList, err := rng.DrawWeighted(remainingEntries, tier.Quantity)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "RNG error: " + err.Error()})
				return
			}
			for _, msisdn := range mainList {
				winners = append(winners, models.Winner{
					ID:        uuid.New(),
					MSISDN:    msisdn,
					PrizeTier: tier.TierName,
					Position:  "Winner",
				})
			}
			remainingEntries = filterOut(remainingEntries, mainList)
		}
		if tier.RunnerUpCount > 0 {
			runList, err := rng.DrawWeighted(remainingEntries, tier.RunnerUpCount)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "RNG error: " + err.Error()})
				return
			}
			for _, msisdn := range runList {
				winners = append(winners, models.Winner{
					ID:        uuid.New(),
					MSISDN:    msisdn,
					PrizeTier: tier.TierName,
					Position:  "RunnerUp",
				})
			}
			remainingEntries = filterOut(remainingEntries, runList)
		}
	}

	// Create new “rerun” Draw record
	newDraw := models.Draw{
		ID:               rerunID,
		DrawDate:         drawDate,
		PrizeStructureID: prizeStruct.ID,
		EntryCount:       entryCount,
	}
	if userID, ok := c.Get("user_id"); ok {
		parsed, _ := uuid.Parse(userID.(string))
		newDraw.AdminUserID = parsed
	}
	if err := config.DB.Create(&newDraw).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save rerun draw: " + err.Error()})
		return
	}

	for i := range winners {
		winners[i].DrawID = newDraw.ID
		if err := config.DB.Create(&winners[i]).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save rerun winner: " + err.Error()})
			return
		}
	}

	// Mask MSISDNs in response
	type maskedWinner struct {
		PrizeTier string `json:"prize_tier"`
		Position  string `json:"position"`
		MaskedMSISDN string `json:"msisdn"`
	}
	responseWinners := make([]maskedWinner, 0, len(winners))
	for _, w := range winners {
		s := w.MSISDN
		masked := fmt.Sprintf("%s****%s", s[:3], s[len(s)-3:])
		responseWinners = append(responseWinners, maskedWinner{
			PrizeTier:   w.PrizeTier,
			Position:    w.Position,
			MaskedMSISDN: masked,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"draw_id":       newDraw.ID,
		"draw_date":     newDraw.DrawDate.Format("2006-01-02"),
		"entry_count":   entryCount,
		"winners":       responseWinners,
	})
}

// filterOut removes any EligibleEntry whose MSISDN is in the “remove” list.
func filterOut(entries []models.EligibleEntry, remove []string) []models.EligibleEntry {
	toRemove := make(map[string]struct{}, len(remove))
	for _, m := range remove {
		toRemove[m] = struct{}{}
	}
	result := make([]models.EligibleEntry, 0, len(entries))
	for _, e := range entries {
		if _, found := toRemove[e.MSISDN]; !found {
			result = append(result, e)
		}
	}
	return result
}
