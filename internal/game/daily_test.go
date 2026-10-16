package game

import (
	"testing"
	"time"
)

func TestDailySeed(t *testing.T) {
	d := time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC)
	a := DailySeed(d, "utrecht", "guess", "buy")
	if a != DailySeed(d.Add(12*time.Hour), "utrecht", "guess", "buy") {
		t.Fatal("not stable within day")
	}
	if a == DailySeed(d.AddDate(0, 0, 1), "utrecht", "guess", "buy") {
		t.Fatal("not changing between days")
	}
	if a == DailySeed(d, "utrecht", "guess", "rent") {
		t.Fatal("not mode specific")
	}
}
