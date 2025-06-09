package handlers

import (
	"encoding/csv"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/ArowuTest/promo-backend/internal/config"
	"github.com/ArowuTest/promo-backend/internal/models"
	"github.com/ArowuTest/promo-backend/internal/posthog"
	"github.com/ArowuTest/promo-backend/internal/rng"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type MSISDNEntry struct {
	MSISDN string `json:"msisdn" binding:"required"`
	Points int    `json:"points" binding:"required,gte=1"`
}

type drawRequest struct {
	DrawDate         string        `json:"draw_date" binding:"required"`
	PrizeStructureID string        `json:"prize_structure_id" binding:"required"`
	MSISDNEntries    []MSISDNEntry `json:"msisdn_entries,omitempty"`
}

func loadCsvEntries() ([]MSISDNEntry, error) {
	f, err := os.Open("entries.csv")
	if err != nil {
		return nil, err
	}
	defer f.Close()

	reader := csv.NewReader(f)
	rows, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}

	var list []MSISDNEntry
	for i, row := range rows {
		if i == 0 {
			continue
		}
		if len(row) < 2 {
			continue
		}
		pts, err := strconv.Atoi(row[1])
		if err != nil || pts < 1 {
			continue
		}
		list = append(list, MSISDNEntry{MSISDN: row[0], Points: pts})
	}
	return list, nil
}

func ExecuteDraw(c *gin.Context) {
	var req drawRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payload: " + err.Error()}); return
	}

	drawDate, err := time.Parse("2006-01-02", req.DrawDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid date format; use foyer-MM-DD"}); return
	}

	var existing models.Draw
	if err := config.DB.Where("draw_date = ?", drawDate).First(&existing).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{
			"error":          "Draw already executed for this date. Use the rerun feature if needed.",
			"rerun_eligible": true,
			"draw_id":        existing.ID,
		}); return
	}

	prizeStructureUUID, err := uuid.Parse(req.PrizeStructureID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid Prize Structure ID format"}); return
	}

	var prizeStruct models.PrizeStructure
	if err := config.DB.Preload("Tiers", func(db *gorm.DB) *gorm.DB {
		return db.Order("order_index asc")
	}).First(&prizeStruct, "id = ?", prizeStructureUUID).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Selected prize structure not found"}); return
	}

	var entries []models.EligibleEntry
	drawSource := "PostHog"
	if len(req.MSISDNEntries) > 0 {
		drawSource = "CSV"
		for _, row := range req.MSISDNEntries {
			entries = append(entries, models.EligibleEntry{MSISDN: row.MSISDN, Points: row.Points})
		}
	} else {
		windowStart, windowEnd := computePostHogWindow(drawDate)
		phClient, _ := posthog.NewClient(config.Cfg)
		defer phClient.Close()
		phEntries, err := phClient.FetchEligibleEntries(windowStart, windowEnd)
		if err != nil { c.JSON(http.StatusInternalServerError, gin.H{"error": "PostHog fetch failed: " + err.Error()}); return }
		entries = phEntries
	}

	if len(entries) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No eligible entries found for this draw"}); return
	}

	var allPastWinners []models.Winner
	config.DB.Find(&allPastWinners)
	pastWinsByTier := make(map[string]map[uuid.UUID]bool)
	for _, w := range allPastWinners {
		if _, ok := pastWinsByTier[w.MSISDN]; !ok {
			pastWinsByTier[w.MSISDN] = make(map[uuid.UUID]bool)
		}
		pastWinsByTier[w.MSISDN][w.PrizeTierID] = true
	}

	drawResults, err := rng.DrawWinners(entries, prizeStruct.Tiers, pastWinsByTier)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Draw failed: " + err.Error()}); return
	}

	tx := config.DB.Begin()
	newDrawID := uuid.New()
	adminIDStr, _ := c.Get("user_id")
	adminUUID, _ := uuid.Parse(adminIDStr.(string))
	totalPoints := 0
	for _, e := range entries { totalPoints += e.Points }

	newDraw := models.Draw{ID: newDrawID, DrawDate: drawDate, PrizeStructureID: prizeStruct.ID, TotalEntries: totalPoints, AdminUserID: adminUUID, Source: drawSource, IsRerun: false}
	if err := tx.Create(&newDraw).Error; err != nil {
		tx.Rollback(); c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save new draw"}); return
	}

	var responseWinners []gin.H
	for _, winnerInfo := range drawResults {
		var tierID uuid.UUID
		for _, pt := range prizeStruct.Tiers {
			if pt.TierName == winnerInfo.TierName { tierID = pt.ID; break }
		}
		newWinner := models.Winner{ID: uuid.New(), DrawID: newDrawID, PrizeTierID: tierID, MSISDN: winnerInfo.MSISDN, Position: winnerInfo.Position, IsRunnerUp: winnerInfo.IsRunnerUp}
		if err := tx.Create(&newWinner).Error; err != nil {
			tx.Rollback(); c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save winner"}); return
		}
		responseWinners = append(responseWinners, gin.H{"prize_tier": winnerInfo.TierName, "position": winnerInfo.Position, "masked_msisdn": maskMSISDN(winnerInfo.MSISDN), "is_runner_up": winnerInfo.IsRunnerUp})
	}
	tx.Commit()

	c.JSON(http.StatusOK, gin.H{"winners": responseWinners})
}

func RerunDraw(c *gin.Context) {
	drawIDParam := c.Param("id")
	origDrawID, err := uuid.Parse(drawIDParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid draw ID"}); return
	}

	var req drawRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payload for rerun: " + err.Error()}); return
	}
	
	var oldDraw models.Draw
	if err := config.DB.First(&oldDraw, "id = ?", origDrawID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Original draw not found"}); return
	}

	drawDate := oldDraw.DrawDate

	var prizeStruct models.PrizeStructure
	if err := config.DB.Preload("Tiers", func(db *gorm.DB) *gorm.DB {
		return db.Order("order_index asc")
	}).First(&prizeStruct, "id = ?", oldDraw.PrizeStructureID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Prize structure for original draw not found"}); return
	}

	var entries []models.EligibleEntry
	drawSource := "PostHog"
	if len(req.MSISDNEntries) > 0 {
		drawSource = "CSV"
		for _, row := range req.MSISDNEntries {
			entries = append(entries, models.EligibleEntry{MSISDN: row.MSISDN, Points: row.Points})
		}
	} else {
		windowStart, windowEnd := computePostHogWindow(drawDate)
		phClient, _ := posthog.NewClient(config.Cfg)
		defer phClient.Close()
		phEntries, err := phClient.FetchEligibleEntries(windowStart, windowEnd)
		if err != nil { c.JSON(http.StatusInternalServerError, gin.H{"error": "PostHog fetch failed for rerun: " + err.Error()}); return }
		entries = phEntries
	}

	if len(entries) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No eligible entries found for this draw's window"}); return
	}

	var allPastWinners []models.Winner
	config.DB.Find(&allPastWinners)
	pastWinsByTier := make(map[string]map[uuid.UUID]bool)
	for _, w := range allPastWinners {
		if _, ok := pastWinsByTier[w.MSISDN]; !ok {
			pastWinsByTier[w.MSISDN] = make(map[uuid.UUID]bool)
		}
		pastWinsByTier[w.MSISDN][w.PrizeTierID] = true
	}

	rerunRes, err := rng.DrawWinners(entries, prizeStruct.Tiers, pastWinsByTier)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Rerun draw failed: " + err.Error()}); return
	}

	tx := config.DB.Begin()
	newDrawID := uuid.New()
	adminID, _ := c.Get("user_id")
	adminUUID, _ := uuid.Parse(adminID.(string))
	totalPoints := 0
	for _, e := range entries { totalPoints += e.Points }

	newDraw := models.Draw{ID: newDrawID, DrawDate: drawDate, PrizeStructureID: prizeStruct.ID, TotalEntries: totalPoints, AdminUserID: adminUUID, Source: drawSource, IsRerun: true}
	if err := tx.Create(&newDraw).Error; err != nil {
		tx.Rollback(); c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create rerun draw"}); return
	}
	
	// The original draw is NOT updated. This was the bug.
	// We simply create a new draw with IsRerun=true.

	var responseWinners []gin.H
	for _, winnerInfo := range rerunRes {
		var tierID uuid.UUID
		for _, pt := range prizeStruct.Tiers {
			if pt.TierName == winnerInfo.TierName { tierID = pt.ID; break }
		}
		newWinner := models.Winner{ID: uuid.New(), DrawID: newDrawID, PrizeTierID: tierID, MSISDN: winnerInfo.MSISDN, Position: winnerInfo.Position, IsRunnerUp: winnerInfo.IsRunnerUp}
		if err := tx.Create(&newWinner).Error; err != nil {
			tx.Rollback(); c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save rerun winner"}); return
		}
		responseWinners = append(responseWinners, gin.H{"prize_tier": winnerInfo.TierName, "position": winnerInfo.Position, "masked_msisdn": maskMSISDN(winnerInfo.MSISDN), "is_runner_up": winnerInfo.IsRunnerUp})
	}
	tx.Commit()

	c.JSON(http.StatusOK, gin.H{"winners": responseWinners})
}

func ListDraws(c *gin.Context) {
	var draws []models.Draw
	if err := config.DB.Preload("AdminUser").Order("draw_date desc").Find(&draws).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch draws: " + err.Error()}); return
	}
	c.JSON(http.StatusOK, draws)
}

func computePostHogWindow(drawDate time.Time) (time.Time, time.Time) {
	weekday := drawDate.Weekday()
	windowEnd := time.Date(drawDate.Year(), drawDate.Month(), drawDate.Day(), 17, 0, 0, 0, drawDate.Location())
	var windowStart time.Time
	switch weekday {
	case time.Monday:
		windowStart = windowEnd.AddDate(0, 0, -3).Add(time.Second)
	case time.Saturday:
		windowStart = windowEnd.AddDate(0, 0, -7).Add(time.Second)
	default:
		windowStart = windowEnd.AddDate(0, 0, -1).Add(time.Second)
	}
	return windowStart, windowEnd
}

func maskMSISDN(msisdn string) string {
	if len(msisdn) < 7 { return msisdn }
	return msisdn[:3] + "****" + msisdn[len(msisdn)-4:]
}