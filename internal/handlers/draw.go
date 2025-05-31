// internal/handlers/draw.go

package handlers

import (
	"net/http"
	"time"

	"github.com/ArowuTest/promo-backend/internal/config"
	"github.com/ArowuTest/promo-backend/internal/models"
	"github.com/ArowuTest/promo-backend/internal/posthog"
	"github.com/ArowuTest/promo-backend/internal/rng"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// listDrawsResponse is the JSON shape for GET /draws
type listDrawsResponse struct {
	Draws []models.Draw `json:"draws"`
}

// ListDraws handles GET /api/v1/draws
func ListDraws(c *gin.Context) {
	var draws []models.Draw
	if err := config.DB.Preload("Winners").Find(&draws).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch draws: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, listDrawsResponse{Draws: draws})
}

// executeDrawRequest defines payload for POST /draws/execute
type executeDrawRequest struct {
	Date string `json:"date" binding:"required"` // e.g. "2025-06-01"
}

// executeDrawResponse is the JSON shape returned after a draw executes.
type executeDrawResponse struct {
	DrawID    uuid.UUID            `json:"draw_id"`
	Winners   []models.Winner      `json:"winners"`
	EntryCount int                 `json:"entry_count"`
}

// ExecuteDraw handles POST /api/v1/draws/execute
// Only SuperAdmin may call this endpoint.
func ExecuteDraw(c *gin.Context) {
	var req executeDrawRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payload: " + err.Error()})
		return
	}
	// Parse date
	drawDate, err := time.Parse("2006-01-02", req.Date)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid date format. Use YYYY-MM-DD."})
		return
	}

	// Check if a draw already exists for this date (non-rerun)
	var existing models.Draw
	if err := config.DB.Where("DATE(executed_at) = ?", drawDate.Format("2006-01-02")).First(&existing).Error; err == nil {
		// Found a draw with the same date
		c.JSON(http.StatusConflict, gin.H{"error": "A draw has already been executed for this date", "draw_id": existing.ID})
		return
	} else if err != gorm.ErrRecordNotFound {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error: " + err.Error()})
		return
	}

	// Determine if Saturday or not (Saturday draw uses weekly window)
	isSat := drawDate.Weekday() == time.Saturday

	// Compute “eligibility window” boundaries:
	var windowStart, windowEnd time.Time
	if isSat {
		// Saturday draw: from last Saturday 5pm → this Saturday 5pm
		lastSat := drawDate.AddDate(0, 0, -7)
		windowStart = time.Date(lastSat.Year(), lastSat.Month(), lastSat.Day(), 17, 0, 0, 0, drawDate.Location())
		windowEnd = time.Date(drawDate.Year(), drawDate.Month(), drawDate.Day(), 17, 0, 0, 0, drawDate.Location())
	} else {
		// Daily (Mon–Fri): from previous day 5pm → this day 5pm
		prevDay := drawDate.AddDate(0, 0, -1)
		windowStart = time.Date(prevDay.Year(), prevDay.Month(), prevDay.Day(), 17, 0, 0, 0, drawDate.Location())
		windowEnd = time.Date(drawDate.Year(), drawDate.Month(), drawDate.Day(), 17, 0, 0, 0, drawDate.Location())
	}

	// Build a PostHog client (stubbed)
	phClient, err := posthog.NewClient(config.Cfg)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to initialize PostHog: " + err.Error()})
		return
	}
	defer phClient.Close()

	// Fetch eligible entries: []EligibleEntry{MSISDN,Points}
	entries, err := phClient.FetchEligibleEntries(windowStart, windowEnd)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch eligible entries: " + err.Error()})
		return
	}

	// Convert EligibleEntry → rng.WeightedEntry
	var weightedPool []rng.WeightedEntry
	for _, e := range entries {
		weightedPool = append(weightedPool, rng.WeightedEntry{
			MSISDN: e.MSISDN,
			Weight: e.Points,
		})
	}

	// Build cumulative weights
	pool, err := rng.BuildWeighted(weightedPool)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to build weighted pool: " + err.Error()})
		return
	}

	// Look up the PrizeStructure that applies to this date
	var ps models.PrizeStructure
	// We assume you have inserted your PrizeStructures with Effective dates.
	if err := config.DB.Where("DATE(effective) = ?", drawDate.Format("2006-01-02")).Preload("Items").First(&ps).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusBadRequest, gin.H{"error": "No prize structure found for this date"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error fetching prize structure: " + err.Error()})
		}
		return
	}

	// Now we must pick winners *per prize tier*, honoring runner‐ups.
	var allWinners []models.Winner
	rankPosition := 1

	for _, item := range ps.Items {
		// If pool is exhausted, break out
		if len(pool) == 0 {
			break
		}

		// First, pick `item.Quantity` winners for this tier
		winnerMSISDNs, err := rng.DrawMultipleUnique(pool, item.Quantity)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "RNG error: " + err.Error()})
			return
		}
		for _, msisdn := range winnerMSISDNs {
			maskFirst := ""
			maskLast := ""
			if len(msisdn) >= 6 {
				maskFirst = msisdn[:3]
				maskLast = msisdn[len(msisdn)-3:]
			}
			allWinners = append(allWinners, models.Winner{
				DrawID:    uuid.Nil, // fill in after Draw is created
				PrizeTier: item.PrizeName,
				Position:  rankPosition,
				MaskFirst3: maskFirst,
				MaskLast3:  maskLast,
				MSISDN:    msisdn,
			})
			rankPosition++
		}
		// Next, pick runner‐ups
		if item.RunnerUpCount > 0 && len(pool) > 0 {
			runnerMSISDNs, err2 := rng.DrawMultipleUnique(pool, item.RunnerUpCount)
			if err2 != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "RNG error: " + err2.Error()})
				return
			}
			for _, msisdn := range runnerMSISDNs {
				maskFirst := ""
				maskLast := ""
				if len(msisdn) >= 6 {
					maskFirst = msisdn[:3]
					maskLast = msisdn[len(msisdn)-3:]
				}
				allWinners = append(allWinners, models.Winner{
					DrawID:    uuid.Nil, // fill in later
					PrizeTier: item.PrizeName + " (RunnerUp)",
					Position:  rankPosition,
					MaskFirst3: maskFirst,
					MaskLast3:  maskLast,
					MSISDN:    msisdn,
				})
				rankPosition++
			}
		}
		// Note: DrawMultipleUnique removes winners from pool automatically,
		// so no need to manually filter them out here.
	}

	// Create the Draw record
	drawRec := models.Draw{
		ExecutedAt:       time.Now(),
		PrizeStructureID: ps.ID,
		EntryCount:       len(entries),
		IsRerun:          false,
	}

	if err := config.DB.Create(&drawRec).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save draw: " + err.Error()})
		return
	}

	// Save each Winner with the DrawID
	for i := range allWinners {
		allWinners[i].DrawID = drawRec.ID
	}
	if err := config.DB.Create(&allWinners).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save winners: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, executeDrawResponse{
		DrawID:     drawRec.ID,
		Winners:    allWinners,
		EntryCount: len(entries),
	})
}

// rerunDrawRequest defines payload for POST /draws/rerun/:id
type rerunDrawRequest struct{}

// rerunDrawResponse is the JSON returned after rerunning a draw.
type rerunDrawResponse struct {
	OriginalDrawID uuid.UUID       `json:"original_draw_id"`
	NewDrawID      uuid.UUID       `json:"new_draw_id"`
	Winners        []models.Winner `json:"winners"`
}

// RerunDraw handles POST /api/v1/draws/rerun/:id
// It fetches the original draw by ID, repeats the RNG with the same params,
// and creates a brand‐new Draw record marked as `IsRerun = true`.
func RerunDraw(c *gin.Context) {
	origIDStr := c.Param("id")
	origID, err := uuid.Parse(origIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid draw ID"})
		return
	}

	// Fetch original draw (including winners to get prize structure & entryCount)
	var origDraw models.Draw
	if err := config.DB.Preload("Winners").First(&origDraw, "id = ?", origID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Original draw not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error: " + err.Error()})
		}
		return
	}

	// Fetch the same PrizeStructure
	var ps models.PrizeStructure
	if err := config.DB.Preload("Items").First(&ps, "id = ?", origDraw.PrizeStructureID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch prize structure: " + err.Error()})
		return
	}

	// Re‐fetch eligible entries for the original draw (we assume the same window)
	// For simplicity, assume origDraw.ExecutedAt’s date defines the window again.
	drawDate := origDraw.ExecutedAt
	isSat := drawDate.Weekday() == time.Saturday

	var windowStart, windowEnd time.Time
	if isSat {
		lastSat := drawDate.AddDate(0, 0, -7)
		windowStart = time.Date(lastSat.Year(), lastSat.Month(), lastSat.Day(), 17, 0, 0, 0, drawDate.Location())
		windowEnd = time.Date(drawDate.Year(), drawDate.Month(), drawDate.Day(), 17, 0, 0, 0, drawDate.Location())
	} else {
		prevDay := drawDate.AddDate(0, 0, -1)
		windowStart = time.Date(prevDay.Year(), prevDay.Month(), prevDay.Day(), 17, 0, 0, 0, drawDate.Location())
		windowEnd = time.Date(drawDate.Year(), drawDate.Month(), drawDate.Day(), 17, 0, 0, 0, drawDate.Location())
	}

	phClient, err := posthog.NewClient(config.Cfg)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to initialize PostHog: " + err.Error()})
		return
	}
	defer phClient.Close()

	entries, err := phClient.FetchEligibleEntries(windowStart, windowEnd)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch eligible entries: " + err.Error()})
		return
	}

	// Rebuild weighted pool
	var weightedPool []rng.WeightedEntry
	for _, e := range entries {
		weightedPool = append(weightedPool, rng.WeightedEntry{
			MSISDN: e.MSISDN,
			Weight: e.Points,
		})
	}
	pool, err := rng.BuildWeighted(weightedPool)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to build weighted pool: " + err.Error()})
		return
	}

	// Re‐draw exactly as before
	var allWinners []models.Winner
	rankPosition := 1
	for _, item := range ps.Items {
		if len(pool) == 0 {
			break
		}
		winnerMSISDNs, err := rng.DrawMultipleUnique(pool, item.Quantity)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "RNG error: " + err.Error()})
			return
		}
		for _, msisdn := range winnerMSISDNs {
			maskFirst := ""
			maskLast := ""
			if len(msisdn) >= 6 {
				maskFirst = msisdn[:3]
				maskLast = msisdn[len(msisdn)-3:]
			}
			allWinners = append(allWinners, models.Winner{
				DrawID:    uuid.Nil, // fill later
				PrizeTier: item.PrizeName,
				Position:  rankPosition,
				MaskFirst3: maskFirst,
				MaskLast3:  maskLast,
				MSISDN:    msisdn,
			})
			rankPosition++
		}
		if item.RunnerUpCount > 0 && len(pool) > 0 {
			runnerMSISDNs, err2 := rng.DrawMultipleUnique(pool, item.RunnerUpCount)
			if err2 != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "RNG error: " + err2.Error()})
				return
			}
			for _, msisdn := range runnerMSISDNs {
				maskFirst := ""
				maskLast := ""
				if len(msisdn) >= 6 {
					maskFirst = msisdn[:3]
					maskLast = msisdn[len(msisdn)-3:]
				}
				allWinners = append(allWinners, models.Winner{
					DrawID:    uuid.Nil,
					PrizeTier: item.PrizeName + " (RunnerUp)",
					Position:  rankPosition,
					MaskFirst3: maskFirst,
					MaskLast3:  maskLast,
					MSISDN:    msisdn,
				})
				rankPosition++
			}
		}
	}

	// Create a new Draw record with IsRerun=true
	newDraw := models.Draw{
		ExecutedAt:       time.Now(),
		PrizeStructureID: ps.ID,
		EntryCount:       len(entries),
		IsRerun:          true,
	}
	if err := config.DB.Create(&newDraw).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create new draw: " + err.Error()})
		return
	}

	// Save winners
	for i := range allWinners {
		allWinners[i].DrawID = newDraw.ID
	}
	if err := config.DB.Create(&allWinners).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save winners: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, rerunDrawResponse{
		OriginalDrawID: origID,
		NewDrawID:      newDraw.ID,
		Winners:        allWinners,
	})
}
