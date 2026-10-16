package game

import "testing"

func TestHintScore(t *testing.T) {
	if got := HintScore(100, 100, 1); got != 900 {
		t.Fatalf("got %d", got)
	}
	if got := HintScore(100, 100, 20); got != 0 {
		t.Fatalf("got %d", got)
	}
	if got := HintScore(100, 100, -1); got != 1000 {
		t.Fatalf("got %d", got)
	}
}
func TestPriceBandDeterministic(t *testing.T) {
	for _, kind := range []string{"buy", "rent"} {
		p := Property{Price: 350000, Kind: kind}
		if kind == "rent" {
			p.Price = 1500
		}
		a, b := PriceBand(p, 17)
		c, d := PriceBand(p, 17)
		if a != c || b != d || a >= b {
			t.Fatalf("bad range: %d %d", a, b)
		}
	}
}
func TestInspectionDeterministic(t *testing.T) {
	p := Property{Price: 350000, Kind: "buy"}
	a := Inspect(p, 19)
	b := Inspect(p, 19)
	if a != b || a.Condition < 55 || a.Condition > 95 || a.RepairCost <= 0 {
		t.Fatal(a, b)
	}
	p.Kind = "rent"
	if Inspect(p, 19).RepairCost != 0 {
		t.Fatal("rent repair cost should not be a purchase estimate")
	}
}
