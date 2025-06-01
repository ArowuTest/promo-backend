package rng

import (
	"crypto/rand"
	"errors"
	"math/big"

	"github.com/ArowuTest/promo-backend/internal/models"
)

// DrawWeighted selects `count` unique winners (MSISDNs) from a slice of EligibleEntry,
// where each entry’s Points represent its weight (# of tickets).  It uses crypto/rand
// internally so it is cryptographically secure.
//
// Returns a slice of unique MSISDN winners (length ≤ count).
func DrawWeighted(entries []models.EligibleEntry, count int) ([]string, error) {
	type bucket struct {
		msisdn string
		weight int
	}

	// Build initial buckets
	pool := make([]bucket, 0, len(entries))
	totalWeight := 0
	for _, e := range entries {
		if e.Points <= 0 {
			continue
		}
		pool = append(pool, bucket{msisdn: e.MSISDN, weight: e.Points})
		totalWeight += e.Points
	}

	if totalWeight == 0 || len(pool) == 0 {
		return []string{}, nil // no eligible entries or no weight
	}

	if count > len(pool) {
		count = len(pool)
	}

	winners := make([]string, 0, count)
	for k := 0; k < count && len(pool) > 0; k++ {
		// Random integer in [1, totalWeight]
		randInt, err := rand.Int(rand.Reader, big.NewInt(int64(totalWeight)))
		if err != nil {
			return nil, err
		}
		// shift to 1-based
		choice := int(randInt.Int64()) + 1

		// locate which bucket contains `choice`
		cum := 0
		var selectedIdx int
		for i, b := range pool {
			cum += b.weight
			if choice <= cum {
				selectedIdx = i
				break
			}
		}

		// record winner
		winners = append(winners, pool[selectedIdx].msisdn)

		// remove that bucket, subtract its weight from totalWeight
		totalWeight -= pool[selectedIdx].weight
		pool = append(pool[:selectedIdx], pool[selectedIdx+1:]...)
	}

	return winners, nil
}
