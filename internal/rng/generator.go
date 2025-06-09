package rng

import (
	"errors"
	"sort"

	"github.com/ArowuTest/promo-backend/internal/models"
	"github.com/google/uuid"
)

var csprng *CSPRNG

func init() {
	var err error
	csprng, err = NewCSPRNG()
	if err != nil {
		panic("rng: failed to initialize AES-CTR CSPRNG: " + err.Error())
	}
}

type WinnerResult struct {
	TierName   string
	MSISDN     string
	Position   int
	IsRunnerUp bool
}

func BuildWeightedEntries(entries []models.EligibleEntry) ([]models.WeightedEntry, int) {
	var weighted []models.WeightedEntry
	totalPoints := 0
	for _, e := range entries {
		if e.Points > 0 {
			totalPoints += e.Points
			weighted = append(weighted, models.WeightedEntry{MSISDN: e.MSISDN, Weight: e.Points})
		}
	}
	sort.Slice(weighted, func(i, j int) bool { return weighted[i].MSISDN < weighted[j].MSISDN })

	cum := 0
	for i := range weighted {
		cum += weighted[i].Weight
		weighted[i].CumSum = cum
	}
	return weighted, totalPoints
}

func pickOneMSISDN(weighted []models.WeightedEntry, totalPoints int) (string, error) {
	if totalPoints <= 0 {
		return "", errors.New("cannot pick from a pool with zero total points")
	}
	u32, err := csprng.Uint32()
	if err != nil {
		return "", err
	}
	r := int(u32 % uint32(totalPoints))
	idx := sort.Search(len(weighted), func(i int) bool { return r < weighted[i].CumSum })
	if idx >= len(weighted) {
		return "", errors.New("rng: index out of range during winner selection")
	}
	return weighted[idx].MSISDN, nil
}

func DrawWinners(
	entries []models.EligibleEntry,
	tiers []models.PrizeTier,
	pastWinsByTier map[string]map[uuid.UUID]bool,
) ([]WinnerResult, error) {
	weightedPool, totalPoints := BuildWeightedEntries(entries)
	var finalResults []WinnerResult
	winnersThisDraw := make(map[string]bool)

	sort.Slice(tiers, func(i, j int) bool { return tiers[i].OrderIndex < tiers[j].OrderIndex })

	for _, tier := range tiers {
		var mainWinnersForTier []string
		
		for i := 0; i < tier.Quantity; i++ {
			winner, err := drawUniqueWinner(&weightedPool, &totalPoints, winnersThisDraw, pastWinsByTier, tier)
			if err != nil {
				if err.Error() == "no eligible winners left" { break }
				return nil, err
			}
			mainWinnersForTier = append(mainWinnersForTier, winner)
		}

		positionCounter := 1
		for _, winnerMsisdn := range mainWinnersForTier {
			finalResults = append(finalResults, WinnerResult{TierName: tier.TierName, MSISDN: winnerMsisdn, Position: positionCounter, IsRunnerUp: false})
			positionCounter++
		}

		totalRunnerUpsToDraw := len(mainWinnersForTier) * tier.RunnerUpCount
		runnerUpPositionCounter := 1
		for i := 0; i < totalRunnerUpsToDraw; i++ {
			runnerUp, err := drawUniqueWinner(&weightedPool, &totalPoints, winnersThisDraw, pastWinsByTier, tier)
			if err != nil {
				if err.Error() == "no eligible winners left" { break }
				return nil, err
			}
			finalResults = append(finalResults, WinnerResult{TierName: tier.TierName, MSISDN: runnerUp, Position: runnerUpPositionCounter, IsRunnerUp: true})
			runnerUpPositionCounter++
		}
	}
	return finalResults, nil
}

func drawUniqueWinner(
	weightedPool *[]models.WeightedEntry,
	totalPoints *int,
	winnersThisDraw map[string]bool,
	pastWinsByTier map[string]map[uuid.UUID]bool,
	currentTier models.PrizeTier,
) (string, error) {
	const maxAttempts = 20000 
	for i := 0; i < maxAttempts; i++ {
		if *totalPoints <= 0 { return "", errors.New("no eligible winners left") }
		
		selectedMsisdn, err := pickOneMSISDN(*weightedPool, *totalPoints)
		if err != nil { return "", err }

		if winnersThisDraw[selectedMsisdn] { continue }
		
		if pastTiersWon, ok := pastWinsByTier[selectedMsisdn]; ok {
			if _, hasWonThisTier := pastTiersWon[currentTier.ID]; hasWonThisTier {
				continue
			}
		}
		
		winnersThisDraw[selectedMsisdn] = true

		var removedWeight int
		var newPool []models.WeightedEntry
		for _, entry := range *weightedPool {
			if entry.MSISDN != selectedMsisdn {
				newPool = append(newPool, entry)
			} else {
				removedWeight = entry.Weight
			}
		}
		
		if removedWeight > 0 {
			*weightedPool = newPool
			*totalPoints -= removedWeight
			
			cum := 0
			for i := range *weightedPool {
				cum += (*weightedPool)[i].Weight
				(*weightedPool)[i].CumSum = cum
			}
		}

		return selectedMsisdn, nil
	}
	return "", errors.New("max attempts reached to find a unique winner")
}