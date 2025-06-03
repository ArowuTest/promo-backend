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

// MSISDNEntry represents one CSV row: msisdn + points
type MSISDNEntry struct {
	MSISDN string `json:"msisdn" binding:"required"`
	Points int    `json:"points" binding:"required,gte=1"`
}

// drawRequest is the JSON payload for executing a draw.
// It carries a required draw_date, plus an optional slice of MSISDNEntry from CSV.
type drawRequest struct {
	DrawDate      string        `json:"draw_date" binding:"required"`     // “YYYY-MM-DD”
	MSISDNEntries []MSISDNEntry `json:"msisdn_entries,omitempty"`         // optional CSV data
}

// loadCsvEntries reads a local file named “entries.csv” with a header row (“msisdn,points”)
// and returns []MSISDNEntry so we can expand into EligibleEntry later.
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
			// skip header
			continue
		}
		if len(row) < 2 {
			continue
		}
		pts, err := strconv.Atoi(row[1])
		if err != nil || pts < 1 {
			continue
		}
		list = append(list, MSISDNEntry{
			MSISDN: row[0],
			Points: pts,
		})
	}
	return list, nil
}

// ExecuteDraw handles POST /api/v1/draws/execute
func ExecuteDraw(c *gin.Context) {
	var req drawRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payload: " + err.Error()})
		return
	}

	// 1) Parse draw_date
	drawDate, err := time.Parse("2006-01-02", req.DrawDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid date format; use YYYY-MM-DD"})
		return
	}

	// 2) Check if a draw already exists for this date
	var existing models.Draw
	if err := config.DB.Where("draw_date = ?", drawDate).First(&existing).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "Draw already executed for this date"})
		return
	}

	// 3) Build []models.EligibleEntry (weighted by points)
	var entries []models.EligibleEntry

	if len(req.MSISDNEntries) > 0 {
		// a) Front-end provided CSV entries: convert each MSISDNEntry → repeated EligibleEntry
		for _, row := range req.MSISDNEntries {
			for i := 0; i < row.Points; i++ {
				entries = append(entries, models.EligibleEntry{
					MSISDN: row.MSISDN,
				})
			}
		}
	} else {
		// b) Otherwise compute the correct window and fetch from PostHog
		windowStart, windowEnd := computePostHogWindow(drawDate)

		phClient, _ := posthog.NewClient(config.Cfg)
		defer phClient.Close()

		phEntries, err := phClient.FetchEligibleEntries(windowStart, windowEnd)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "PostHog fetch failed: " + err.Error()})
			return
		}

		// phEntries is already []models.EligibleEntry, so just assign:
		entries = phEntries
	}

	if len(entries) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No eligible entries (zero total points)"})
		return
	}

	// 4) Load the PrizeStructure for this drawDate
	var prizeStruct models.PrizeStructure
	if err := config.DB.
		Preload("Tiers", func(db *gorm.DB) *gorm.DB {
			return db.Order("order_index asc")
		}).
		Where("effective = ?", drawDate).
		First(&prizeStruct).
		Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No prize structure defined for this date"})
		return
	}

	// 5) Build a map of past jackpot winners (to prevent duplicates)
	pastWinners := make(map[string]bool)
	var jackpotWins []models.Winner
	config.DB.
		Joins("JOIN prize_tiers ON prize_tiers.id = winners.prize_tier_id").
		Where("prize_tiers.tier_name = ? AND winners.draw_id IN (?)",
			"Jackpot",
			config.DB.Model(&models.Draw{}).Select("id"),
		).
		Find(&jackpotWins)

	for _, w := range jackpotWins {
		pastWinners[w.MSISDN] = true
	}

	// 6) Run the weighted RNG draw: pass []models.EligibleEntry, []PrizeTier, map
	drawResults, err := rng.DrawWinners(entries, prizeStruct.Tiers, pastWinners)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Draw failed: " + err.Error()})
		return
	}

	// 7) Create new Draw record
	newDrawID := uuid.New()
	newDraw := models.Draw{
		ID:               newDrawID,
		DrawDate:         drawDate,
		PrizeStructureID: prizeStruct.ID,
		TotalEntries:     len(entries),
		AdminUserID:      c.MustGet("user_id").(uuid.UUID), // assume middleware sets user_id
		IsRerun:          false,
	}
	if err := config.DB.Create(&newDraw).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save new draw"})
		return
	}

	// 8) Insert all winners into winners table
	for tierName, tierWinners := range drawResults {
		for _, winnerInfo := range tierWinners {
			// Determine runner-up: if position > Tier.Quantity
			isRunner := false
			for _, pt := range prizeStruct.Tiers {
				if pt.TierName == tierName && winnerInfo.Position > pt.Quantity {
					isRunner = true
					break
				}
			}

			// Find PrizeTierID from tierName
			var tierID uuid.UUID
			for _, pt := range prizeStruct.Tiers {
				if pt.TierName == tierName {
					tierID = pt.ID
					break
				}
			}

			newWinner := models.Winner{
				ID:          uuid.New(),
				DrawID:      newDrawID,
				PrizeTierID: tierID,
				MSISDN:      winnerInfo.MSISDN,
				Position:    winnerInfo.Position,
				IsRunnerUp:  isRunner,
			}
			if err := config.DB.Create(&newWinner).Error; err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save winner"})
				return
			}
		}
	}

	// 9) Build response JSON with masked MSISDN
	respArr := make([]gin.H, 0)
	for tierName, tierWinners := range drawResults {
		for _, winnerInfo := range tierWinners {
			respArr = append(respArr, gin.H{
				"prize_tier":    tierName,
				"position":      winnerInfo.Position,
				"masked_msisdn": maskMSISDN(winnerInfo.MSISDN),
			})
		}
	}

	c.JSON(http.StatusOK, gin.H{"winners": respArr})
}

// RerunDraw handles POST /api/v1/draws/rerun/:drawId
func RerunDraw(c *gin.Context) {
	drawIDParam := c.Param("drawId")
	drawID, err := uuid.Parse(drawIDParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid draw ID"})
		return
	}

	var oldDraw models.Draw
	if err := config.DB.First(&oldDraw, "id = ?", drawID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Draw not found"})
		return
	}

	drawDate := oldDraw.DrawDate
	windowStart, windowEnd := computePostHogWindow(drawDate)

	phClient, _ := posthog.NewClient(config.Cfg)
	defer phClient.Close()

	phEntries, err := phClient.FetchEligibleEntries(windowStart, windowEnd)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "PostHog fetch failed: " + err.Error()})
		return
	}

	entries := phEntries

	var prizeStruct models.PrizeStructure
	if err := config.DB.
		Preload("Tiers", func(db *gorm.DB) *gorm.DB {
			return db.Order("order_index asc")
		}).
		Where("id = ?", oldDraw.PrizeStructureID).
		First(&prizeStruct).
		Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Prize structure not found"})
		return
	}

	pastWinners := make(map[string]bool)
	var prevWinners []models.Winner
	config.DB.
		Joins("JOIN prize_tiers ON prize_tiers.id = winners.prize_tier_id").
		Where("winners.draw_id = ? AND prize_tiers.tier_name = ?", oldDraw.ID, "Jackpot").
		Find(&prevWinners)

	for _, w := range prevWinners {
		pastWinners[w.MSISDN] = true
	}

	rerunRes, err := rng.DrawWinners(entries, prizeStruct.Tiers, pastWinners)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Rerun draw failed: " + err.Error()})
		return
	}

	newDrawID := uuid.New()
	newDraw := models.Draw{
		ID:               newDrawID,
		DrawDate:         drawDate,
		PrizeStructureID: prizeStruct.ID,
		TotalEntries:     len(entries),
		AdminUserID:      c.MustGet("user_id").(uuid.UUID),
		IsRerun:          true,
	}
	if err := config.DB.Create(&newDraw).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create rerun draw"})
		return
	}

	respArr := make([]gin.H, 0)
	for tierName, tierWinners := range rerunRes {
		for _, winnerInfo := range tierWinners {
			isRunner := false
			for _, pt := range prizeStruct.Tiers {
				if pt.TierName == tierName && winnerInfo.Position > pt.Quantity {
					isRunner = true
					break
				}
			}

			var tierID uuid.UUID
			for _, pt := range prizeStruct.Tiers {
				if pt.TierName == tierName {
					tierID = pt.ID
					break
				}
			}

			newWinner := models.Winner{
				ID:          uuid.New(),
				DrawID:      newDrawID,
				PrizeTierID: tierID,
				MSISDN:      winnerInfo.MSISDN,
				Position:    winnerInfo.Position,
				IsRunnerUp:  isRunner,
			}
			if err := config.DB.Create(&newWinner).Error; err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save rerun winner"})
				return
			}

			respArr = append(respArr, gin.H{
				"prize_tier":    tierName,
				"position":      winnerInfo.Position,
				"masked_msisdn": maskMSISDN(winnerInfo.MSISDN),
			})
		}
	}

	c.JSON(http.StatusOK, gin.H{"winners": respArr})
}

// ListDraws handles GET /api/v1/draws
func ListDraws(c *gin.Context) {
	var draws []models.Draw
	if err := config.DB.Order("draw_date desc").Find(&draws).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch draws: " + err.Error()})
		return
	}

	var resp []gin.H
	for _, d := range draws {
		resp = append(resp, gin.H{
			"id":                 d.ID,
			"draw_date":          d.DrawDate,
			"prize_structure_id": d.PrizeStructureID,
			"total_entries":      d.TotalEntries,
			"is_rerun":           d.IsRerun,
			"created_at":         d.CreatedAt,
			"updated_at":         d.UpdatedAt,
		})
	}
	c.JSON(http.StatusOK, resp)
}

// computePostHogWindow returns (windowStart, windowEnd) for a given drawDate
// according to the rules (with a 1‐second offset on the start time):
//   • Monday        → previous Friday 17:00:01  → Monday 17:00:00
//   • Tuesday–Friday → previous day 17:00:01     → draw day 17:00:00
//   • Saturday      → previous Saturday 17:00:01 → Saturday 17:00:00
//   • Other days    → previous day 17:00:01     → draw day 17:00:00
func computePostHogWindow(drawDate time.Time) (time.Time, time.Time) {
	weekday := drawDate.Weekday()

	// windowEnd is always “drawDate at exactly 17:00:00”
	windowEnd := time.Date(
		drawDate.Year(), drawDate.Month(), drawDate.Day(),
		17, 0, 0, 0, drawDate.Location(),
	)

	// We will always add +1 second for windowStart’s “17:00:01”
	var windowStart time.Time

	switch weekday {
	case time.Monday:
		// previous Friday = drawDate minus 3 days
		friday := drawDate.AddDate(0, 0, -3)
		windowStart = time.Date(
			friday.Year(), friday.Month(), friday.Day(),
			17, 0, 1, 0, drawDate.Location(),
		)

	case time.Tuesday, time.Wednesday, time.Thursday, time.Friday:
		// previous day
		prevDay := drawDate.AddDate(0, 0, -1)
		windowStart = time.Date(
			prevDay.Year(), prevDay.Month(), prevDay.Day(),
			17, 0, 1, 0, drawDate.Location(),
		)

	case time.Saturday:
		// previous Saturday = drawDate minus 7 days
		prevSat := drawDate.AddDate(0, 0, -7)
		windowStart = time.Date(
			prevSat.Year(), prevSat.Month(), prevSat.Day(),
			17, 0, 1, 0, drawDate.Location(),
		)

	default:
		// For Sunday (or any unexpected day), use “previous day → today”
		prev := drawDate.AddDate(0, 0, -1)
		windowStart = time.Date(
			prev.Year(), prev.Month(), prev.Day(),
			17, 0, 1, 0, drawDate.Location(),
		)
	}

	return windowStart, windowEnd
}

// maskMSISDN masks a phone number like “08012345678” → “080****5678”
func maskMSISDN(msisdn string) string {
	if len(msisdn) < 7 {
		return msisdn
	}
	return msisdn[:3] + "****" + msisdn[len(msisdn)-4:]
}
