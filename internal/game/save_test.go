package game

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func exampleSave() HunterSave {
	return HunterSave{Version: 1, Mode: "buy", City: "utrecht", Source: "all", Seed: 42, Rounds: 2, Index: 0, Turns: 2, Score: 1000, Viewed: true, Inspected: true, PropertyIDs: []string{"funda:1", "funda:2"}}
}
func TestHunterSaveRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "save.json")
	s := exampleSave()
	if err := SaveHunter(path, s); err != nil {
		t.Fatal(err)
	}
	got, err := LoadHunter(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, s) {
		t.Fatalf("got %+v want %+v", got, s)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm()&0077 != 0 {
		t.Fatalf("save permissions too broad: %v", fi.Mode())
	}
}
func TestHunterSaveRejectsInvalid(t *testing.T) {
	cases := []func(*HunterSave){func(s *HunterSave) { s.Version = 2 }, func(s *HunterSave) { s.Index = 2 }, func(s *HunterSave) { s.PropertyIDs = nil }, func(s *HunterSave) { s.Viewed = false }, func(s *HunterSave) { s.Turns = 21 }}
	for i, mutate := range cases {
		s := exampleSave()
		mutate(&s)
		if err := SaveHunter(filepath.Join(t.TempDir(), "s.json"), s); err == nil {
			t.Fatalf("case %d accepted", i)
		}
	}
}
func TestHunterSaveCorruptionDoesNotOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "save.json")
	if err := os.WriteFile(path, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadHunter(path); err == nil {
		t.Fatal("corrupt save accepted")
	}
}
func TestHunterSaveReplacesAtomically(t *testing.T) {
	path := filepath.Join(t.TempDir(), "save.json")
	s := exampleSave()
	if err := SaveHunter(path, s); err != nil {
		t.Fatal(err)
	}
	s.Index = 1
	s.Viewed = false
	s.Inspected = false
	s.Score = 2000
	if err := SaveHunter(path, s); err != nil {
		t.Fatal(err)
	}
	got, err := LoadHunter(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, s) {
		t.Fatalf("got %+v want %+v", got, s)
	}
}

func TestHunterSavePriceSnapshot(t *testing.T) {
	s := exampleSave()
	s.Prices = []int64{345000, 355000}
	path := filepath.Join(t.TempDir(), "save.json")
	if err := SaveHunter(path, s); err != nil {
		t.Fatal(err)
	}
	got, err := LoadHunter(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Prices, s.Prices) {
		t.Fatalf("prices = %v", got.Prices)
	}
}

func TestHunterSaveRejectsInvalidPriceSnapshot(t *testing.T) {
	for _, prices := range [][]int64{{345000}, {345000, 0}, {-1, 300000}} {
		s := exampleSave()
		s.Prices = prices
		if err := s.Validate(); err == nil {
			t.Fatalf("accepted %v", prices)
		}
	}
}
