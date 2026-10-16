package game

import (
	"github.com/sc0vu/funda-cli/internal/model"
	"reflect"
	"strings"
	"testing"
)

func TestPublicNeverLeaksPriceOrURL(t *testing.T) {
	p := Property{Price: 365000, URL: "https://example.com/secret", Address: "Secret 1", City: "Utrecht", Area: 70}
	v := p.Public()
	s := strings.ToLower(strings.TrimSpace(v.City))
	if s != "utrecht" {
		t.Fatal(v)
	}
	if strings.Contains(strings.ToLower(strings.Join([]string{v.City, v.Kind, v.Energy}, " ")), "secret") {
		t.Fatal("leak")
	}
	if _, ok := reflect.TypeOf(v).FieldByName("Price"); ok {
		t.Fatal("price exposed")
	}
	if _, ok := reflect.TypeOf(v).FieldByName("URL"); ok {
		t.Fatal("url exposed")
	}
}
func TestFilterAndStableShuffle(t *testing.T) {
	ls := []model.Listing{{Source: "funda", ID: "b", Price: 200, TransactionType: "sale", HasDetails: true}, {Source: "funda", ID: "a", Price: 150, TransactionType: "sale", HasDetails: true}, {Source: "funda", ID: "c", Price: 100, TransactionType: "rental", HasDetails: true}, {Source: "funda", ID: "d", Price: 0, TransactionType: "sale", HasDetails: true}}
	ps := FromListings(ls, "buy")
	if len(ps) != 2 {
		t.Fatalf("got %d", len(ps))
	}
	a := Shuffle(ps, 42)
	b := Shuffle(ps, 42)
	if !reflect.DeepEqual(a, b) {
		t.Fatal("non deterministic")
	}
}
func TestScore(t *testing.T) {
	for _, tc := range []struct {
		actual, guess int64
		score         int
	}{{100, 100, 1000}, {100, 85, 500}, {100, 70, 0}, {100, 150, 0}, {0, 100, 0}} {
		if got := Score(tc.actual, tc.guess); got != tc.score {
			t.Errorf("Score(%d,%d)=%d want %d", tc.actual, tc.guess, got, tc.score)
		}
	}
}
func TestBuyIndependentOfPlayerOffer(t *testing.T) {
	p := Property{Price: 350000}
	a := ResolveBuy(p, Offer{Amount: 340000}, 7)
	b := ResolveBuy(p, Offer{Amount: 390000}, 7)
	if a.RivalAmount != b.RivalAmount {
		t.Fatal("rival bids changed after player bid")
	}
}
func TestRentEligibility(t *testing.T) {
	p := Property{Kind: "rent", Price: 1500}
	a := RentalApplication{MonthlyGrossIncome: 3000, DocumentsComplete: true, HouseholdSize: 2, RequiredMultiplier: 3}
	if ResolveRent(p, a, 1).Won {
		t.Fatal("ineligible applicant won")
	}
	a.MonthlyGrossIncome = 5000
	a.DocumentsComplete = false
	if ResolveRent(p, a, 1).Won {
		t.Fatal("incomplete documents won")
	}
	a.DocumentsComplete = true
	if ResolveRent(p, a, 99) != ResolveRent(p, a, 99) {
		t.Fatal("non deterministic")
	}
}
