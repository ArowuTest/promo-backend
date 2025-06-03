// internal/rng/generator.go

package rng

import (
	"sort"
	"errors"

	"github.com/ArowuTest/promo-backend/internal/models"
)

// --------------------------------------------------
// 1) AES‐CTR CSPRNG Integration
// --------------------------------------------------

// csprng is a package‐level AES‐CTR generator, seeded once from crypto/rand.
var csprng *CSPRNG

func init() {
	var err error
	csprng, err = NewCSPRNG()
	if err != nil {
		// In production, log + exit gracefully. For now, panic so the server fails fast.
		panic("rng: failed to initialize AES‐CTR CSPRNG: " + err.Error())
	}
}

// --------------------------------------------------
// 2) Weighted‐Ticket Draw Logic
// --------------------------------------------------

// WinnerResult holds a drawn winner/runner‐up result.
type WinnerResult struct {
	TierName   string
	MSISDN     string
	Position   int
	PointsUsed int
}

// BuildWeightedEntries converts []EligibleEntry → []WeightedEntry with cumulative sums.
func BuildWeightedEntries(entries []models.EligibleEntry) ([]models.WeightedEntry, int) {
	var weighted []models.WeightedEntry
	totalPoints := 0

	// 1) Sum all points, build initial slice
	for _, e := range entries {
		totalPoints += e.Points
		weighted = append(weighted, models.WeightedEntry{
			MSISDN: e.MSISDN,
			Weight: e.Points,
			CumSum: 0, // placeholder; set below
		})
	}

	// 2) Sort by MSISDN (or any stable key to make CumSum deterministic)
	sort.Slice(weighted, func(i, j int) bool {
		return weighted[i].MSISDN < weighted[j].MSISDN
	})

	// 3) Compute cumulative sums
	cum := 0
	for i := range weighted {
		cum += weighted[i].Weight
		weighted[i].CumSum = cum
	}

	return weighted, totalPoints
}

// pickOneMSISDN draws a single MSISDN from the weighted pool using AES‐CTR CSPRNG.
func pickOneMSISDN(weighted []models.WeightedEntry, totalPoints int) (string, error) {
	// 1) Get a 32‐bit random word from AES‐CTR
	u32, err := csprng.Uint32()
	if err != nil {
		return "", err
	}
	// 2) Reduce modulo totalPoints to get an index in [0, totalPoints)
	r := int(u32 % uint32(totalPoints))

	// 3) Binary search on CumSum to pick the winner
	idx := sort.Search(len(weighted), func(i int) bool {
		return r < weighted[i].CumSum
	})
	if idx < 0 || idx >= len(weighted) {
		return "", errors.New("rng: index out of range")
	}
	return weighted[idx].MSISDN, nil
}

// DrawWinners runs a tier‐by‐tier weighted draw, excluding any MSISDN in pastWinners.
// It returns a map: tier name → slice of WinnerResult.
func DrawWinners(
	entries []models.EligibleEntry,
	tiers []models.PrizeTier,
	pastWinners map[string]bool,
) (map[string][]WinnerResult, error) {
	// 1) Build the weighted pool
	weightedPool, totalPoints := BuildWeightedEntries(entries)
	result := make(map[string][]WinnerResult)

	// 2) For each tier (in ascending OrderIndex)
	for _, tier := range tiers {
		var winnersForTier []WinnerResult
		position := 1

		// Draw main winners
		for pos := 0; pos < tier.Quantity; pos++ {
			msisdn, err := pickOneMSISDN(weightedPool, totalPoints)
			if err != nil {
				return nil, err
			}
			// Skip if they have already won this tier previously
			if pastWinners[msisdn] {
				continue
			}
			pastWinners[msisdn] = true

			winnersForTier = append(winnersForTier, WinnerResult{
				TierName:   tier.TierName,
				MSISDN:     msisdn,
				Position:   position,
				PointsUsed: 0,
			})
			position++

			// Remove ALL instances of this MSISDN from weightedPool
			var newPool []models.WeightedEntry
			newPool = make([]models.WeightedEntry, 0, len(weightedPool))
			newTotal := 0
			for _, we := range weightedPool {
				if we.MSISDN != msisdn {
					newPool = append(newPool, we)
					newTotal += we.Weight
				}
			}
			weightedPool = newPool
			totalPoints = newTotal

			// Recompute CumSum on the new pool
			cum := 0
			for i := range weightedPool {
				cum += weightedPool[i].Weight
				weightedPool[i].CumSum = cum
			}
		}

		// Draw runner-ups, if any
		for rp := 0; rp < tier.RunnerUpCount; rp++ {
			msisdn, err := pickOneMSISDN(weightedPool, totalPoints)
			if err != nil {
				return nil, err
			}
			pastWinners[msisdn] = true

			winnersForTier = append(winnersForTier, WinnerResult{
				TierName:   tier.TierName,
				MSISDN:     msisdn,
				Position:   position,
				PointsUsed: 0,
			})
			position++

			// Remove this MSISDN as well
			var newPool []models.WeightedEntry
			newPool = make([]models.WeightedEntry, 0, len(weightedPool))
			newTotal := 0
			for _, we := range weightedPool {
				if we.MSISDN != msisdn {
					newPool = append(newPool, we)
					newTotal += we.Weight
				}
			}
			weightedPool = newPool
			totalPoints = newTotal

			// Recompute CumSum
			cum := 0
			for i := range weightedPool {
				cum += weightedPool[i].Weight
				weightedPool[i].CumSum = cum
			}
		}

		// Save results for this tier
		result[tier.TierName] = winnersForTier
	}

	return result, nil
}
