// internal/rng/sampler.go

package rng

import (
	"crypto/rand"
	"errors"
	"math/big"
)

// WeightedEntry represents one MSISDN with its “weight” (points) and running cumulative.
type WeightedEntry struct {
	MSISDN     string
	Weight     int // number of “tickets”
	Cumulative int // running total up through this entry
}

// BuildWeighted takes a slice of EligibleEntry (with MSISDN+Points) and
// returns a slice of WeightedEntry with cumulative weights computed.
// In production, EligibleEntry → WeightedEntry conversion happens before calling BuildWeighted.
func BuildWeighted(entries []WeightedEntry) ([]WeightedEntry, error) {
	if len(entries) == 0 {
		return nil, errors.New("no entries to weight")
	}
	total := 0
	for i := range entries {
		if entries[i].Weight <= 0 {
			return nil, errors.New("entry weight must be > 0")
		}
		total += entries[i].Weight
		entries[i].Cumulative = total
	}
	return entries, nil
}

// drawOneIndex picks one random index in [0..(totalWeight-1)] using crypto/rand.
func drawOneIndex(totalWeight int) (int, error) {
	if totalWeight <= 0 {
		return 0, errors.New("totalWeight must be > 0")
	}
	rndBig, err := rand.Int(rand.Reader, big.NewInt(int64(totalWeight)))
	if err != nil {
		return 0, err
	}
	// rnd is an integer [0..totalWeight-1]
	rnd := int(rndBig.Int64())
	return rnd, nil
}

// DrawMultipleUnique picks `count` distinct MSISDNs from a pre‐built weighted pool.
// It returns exactly `count` winners (or fewer if pool runs out).
// Internally, we remove each chosen entry from the pool to ensure uniqueness.
//
// For each pick:
//  1. Look up the current pool’s total weight (pool[len(pool)-1].Cumulative).
//  2. Generate a cryptographic random integer in [0..(totalWeight-1)].
//  3. Find the first WeightedEntry whose Cumulative > rnd.
//  4. Remove it from the slice and subtract its weight from subsequent entries’ Cumulative values.
//  5. Repeat until count winners or pool is empty.
func DrawMultipleUnique(pool []WeightedEntry, count int) ([]string, error) {
	if count <= 0 {
		return nil, errors.New("must draw at least 1 winner")
	}
	if len(pool) == 0 {
		return nil, errors.New("pool is empty")
	}

	// Copy the pool so we can mutate it
	tmp := make([]WeightedEntry, len(pool))
	copy(tmp, pool)

	winners := make([]string, 0, count)
	for i := 0; i < count; i++ {
		// Current total weight:
		totalWeight := tmp[len(tmp)-1].Cumulative

		// Draw one index
		selectedIdx, err := drawOneIndex(totalWeight)
		if err != nil {
			return nil, err
		}

		// Find the entry whose Cumulative > selectedIdx
		var pickIdx int
		for idx := 0; idx < len(tmp); idx++ {
			if selectedIdx < tmp[idx].Cumulative {
				pickIdx = idx
				break
			}
		}

		// Record the MSISDN
		winners = append(winners, tmp[pickIdx].MSISDN)

		// Remove that entry from tmp, adjusting cumulatives
		weightRemoved := tmp[pickIdx].Weight
		tmp = append(tmp[:pickIdx], tmp[pickIdx+1:]...)
		for j := pickIdx; j < len(tmp); j++ {
			tmp[j].Cumulative -= weightRemoved
		}

		if len(tmp) == 0 && i < count-1 {
			// pool exhausted; return what we have
			return winners, nil
		}
	}
	return winners, nil
}
