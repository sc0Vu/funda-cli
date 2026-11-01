package store

import (
	"context"
	"github.com/sc0vu/funda-cli/internal/model"
	"path/filepath"
	"testing"
	"time"
)

func TestLifecycleRemovalAndReopen(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	now := time.Now().UTC()
	if err := s.UpsertRefs(ctx, []model.ListingRef{{Source: "funda", ID: "123", URL: "https://example.org/123", City: "Utrecht", TransactionType: "rent", FirstSeenAt: now, LastSeenAt: now}}); err != nil {
		t.Fatal(err)
	}
	interval := func(string) time.Duration { return time.Hour }
	if err := s.RecordFetchSuccess(ctx, "funda", "123", "available", interval); err != nil {
		t.Fatal(err)
	}
	gen, active, err := s.ActiveGeneration(ctx, "funda", "123")
	if err != nil || !active || gen != 0 {
		t.Fatalf("initial: %d %v %v", gen, active, err)
	}
	if err := s.RecordFetchFailure(ctx, "funda", "123", 404, time.Minute); err != nil {
		t.Fatal(err)
	}
	_, active, err = s.ActiveGeneration(ctx, "funda", "123")
	if err != nil || active {
		t.Fatalf("404 should suppress alert: %v %v", active, err)
	}
	if err := s.RecordFetchFailure(ctx, "funda", "123", 404, time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordFetchSuccess(ctx, "funda", "123", "available", interval); err != nil {
		t.Fatal(err)
	}
	gen, active, err = s.ActiveGeneration(ctx, "funda", "123")
	if err != nil || !active || gen != 1 {
		t.Fatalf("reopened: %d %v %v", gen, active, err)
	}
	if err := s.RecordFetchSuccess(ctx, "funda", "123", "available", interval); err != nil {
		t.Fatal(err)
	}
	gen, _, _ = s.ActiveGeneration(ctx, "funda", "123")
	if gen != 1 {
		t.Fatalf("duplicate generation: %d", gen)
	}
}
func TestClassifyAvailability(t *testing.T) {
	for _, tc := range []struct{ raw, want string }{{"te huur", "active"}, {"verhuurd", "unavailable"}, {"rented_subject_to_conditions", "under_option"}, {"", "unknown"}} {
		if got := classifyAvailability(tc.raw); got != tc.want {
			t.Errorf("%q: got %q want %q", tc.raw, got, tc.want)
		}
	}
}
