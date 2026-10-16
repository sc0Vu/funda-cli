// Package game contains deterministic, offline housing games. It never submits real offers.
package game

import (
	"errors"
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strings"

	"github.com/sc0vu/funda-cli/internal/model"
)

var ErrNoListings = errors.New("no enriched listings with valid prices; run funda enrich first")

type Property struct {
	Source, ID, City, Address, URL, Kind, Energy string
	Price                                        int64
	Area                                         float64
	Rooms                                        int
}
type PublicProperty struct {
	City, Kind, Energy string
	Area               float64
	Rooms              int
}

func (p Property) Public() PublicProperty {
	return PublicProperty{City: p.City, Kind: p.Kind, Energy: p.Energy, Area: p.Area, Rooms: p.Rooms}
}
func FromListings(ls []model.Listing, kind string) []Property {
	out := make([]Property, 0, len(ls))
	for _, l := range ls {
		if !l.HasDetails || l.Price <= 0 || math.IsNaN(l.Price) || math.IsInf(l.Price, 0) {
			continue
		}
		k := Kind(l.TransactionType)
		if kind != "all" && k != kind {
			continue
		}
		if k == "" {
			continue
		}
		out = append(out, Property{Source: l.Source, ID: l.ID, City: l.City, Address: l.Address, URL: l.URL, Kind: k, Energy: l.EnergyLabel, Price: int64(math.Round(l.Price)), Area: l.LivingArea, Rooms: l.Rooms})
	}
	// Database iteration order must not influence seeded games.
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Source != b.Source {
			return a.Source < b.Source
		}
		return a.ID < b.ID
	})
	return out
}
func Kind(v string) string {
	switch strings.ToLower(v) {
	case "sale", "buy", "koop":
		return "buy"
	case "rental", "rent", "huur":
		return "rent"
	}
	return ""
}
func Shuffle(in []Property, seed int64) []Property {
	out := append([]Property(nil), in...)
	rand.New(rand.NewSource(seed)).Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	return out
}
func Score(actual, guess int64) int {
	if actual <= 0 || guess < 0 {
		return 0
	}
	errorRate := math.Abs(float64(actual-guess)) / float64(actual)
	return int(math.Round(1000 * math.Max(0, 1-errorRate/0.30)))
}

type Offer struct {
	Amount           int64
	FinancingCertain bool
	FlexibleDate     bool
}
type Result struct {
	Won         bool
	Explanation string
	RivalAmount int64
	Score       int
}

// ResolveBuy generates rival bids independently of the player's offer.
func ResolveBuy(p Property, offer Offer, seed int64) Result {
	r := rand.New(rand.NewSource(seed))
	rivals := 2 + r.Intn(4)
	highest := int64(0)
	for i := 0; i < rivals; i++ {
		bid := int64(math.Round(float64(p.Price) * (0.96 + r.Float64()*0.13)))
		if bid > highest {
			highest = bid
		}
	}
	playerScore := offer.Amount
	if offer.FinancingCertain {
		playerScore += p.Price / 100
	} // 1% fictional preference bonus
	if offer.FlexibleDate {
		playerScore += p.Price / 200
	}
	won := offer.Amount > 0 && playerScore > highest
	explanation := "Another simulated buyer was preferred."
	if won {
		explanation = "The seller selected your simulated offer."
	}
	return Result{Won: won, Explanation: explanation, RivalAmount: highest, Score: Score(p.Price, offer.Amount)}
}

type RentalApplication struct {
	MonthlyGrossIncome int64
	DocumentsComplete  bool
	HouseholdSize      int
	RequiredMultiplier float64
}

func ResolveRent(p Property, a RentalApplication, seed int64) Result {
	if p.Kind != "rent" {
		return Result{Explanation: "Not a rental listing."}
	}
	if a.RequiredMultiplier <= 0 {
		return Result{Explanation: "Income rule is unknown; set --income-multiplier to simulate eligibility."}
	}
	if a.MonthlyGrossIncome < int64(math.Ceil(float64(p.Price)*a.RequiredMultiplier)) {
		return Result{Explanation: "Does not meet the simulated income requirement."}
	}
	if !a.DocumentsComplete {
		return Result{Explanation: "Application documents are incomplete."}
	}
	if a.HouseholdSize < 1 {
		return Result{Explanation: "Household size must be at least one."}
	}
	r := rand.New(rand.NewSource(seed))
	applicants := 2 + r.Intn(9)
	won := r.Intn(applicants) == 0
	reason := fmt.Sprintf("Not selected in a simulated lottery among %d eligible applicants.", applicants)
	if won {
		reason = fmt.Sprintf("Selected in a simulated lottery among %d eligible applicants.", applicants)
	}
	return Result{Won: won, Explanation: reason}
}
