package game

import (
	"fmt"
	"math"
	"math/rand"
)

// HintLevel is the number of optional hints requested in a guessing round.
// It affects scoring, never the hidden asking price.
const HintPenalty = 100

func HintScore(actual, guess int64, hints int) int {
	if hints < 0 {
		hints = 0
	}
	score := Score(actual, guess) - hints*HintPenalty
	if score < 0 {
		return 0
	}
	return score
}

// PriceBand gives a broad estimate without revealing the asking price.
// This is a fictional game hint, not a professional valuation.
func PriceBand(p Property, seed int64) (int64, int64) {
	if p.Price <= 0 {
		return 0, 0
	}
	r := rand.New(rand.NewSource(seed))
	center := float64(p.Price) * (0.90 + 0.20*r.Float64())
	width := float64(p.Price) * 0.20
	unit := float64(5000)
	if p.Kind == "rent" {
		unit = 25
	}
	low := int64(math.Floor((center-width/2)/unit) * unit)
	high := int64(math.Ceil((center+width/2)/unit) * unit)
	if low < 0 {
		low = 0
	}
	return low, high
}

// Inspection is a deterministic fictional inspection result; it is NOT based
// on an actual survey of the property.
type Inspection struct {
	Condition  int
	RepairCost int64
	Finding    string
}

func Inspect(p Property, seed int64) Inspection {
	r := rand.New(rand.NewSource(seed))
	condition := 55 + r.Intn(41)
	var cost int64
	if p.Kind == "buy" {
		cost = int64(1+r.Intn(15)) * 1000
	}
	findings := []string{"Some cosmetic maintenance is needed.", "Windows and insulation should be checked.", "The kitchen may need updating.", "No major visible concerns in this fictional inspection."}
	return Inspection{Condition: condition, RepairCost: cost, Finding: findings[r.Intn(len(findings))]}
}

func (i Inspection) Summary() string {
	return fmt.Sprintf("Condition %d/100 | Estimated repairs €%d | %s", i.Condition, i.RepairCost, i.Finding)
}
