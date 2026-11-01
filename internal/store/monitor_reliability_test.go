package store

import (
	"context"
	"errors"
	"github.com/sc0vu/funda-cli/internal/model"
	"path/filepath"
	"testing"
	"time"
)

func TestCheckLockAndJobRetries(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	ok, err := s.AcquireCheckLock(ctx, "one", time.Minute)
	if err != nil || !ok {
		t.Fatalf("first lock: %v %v", ok, err)
	}
	ok, err = s.AcquireCheckLock(ctx, "two", time.Minute)
	if err != nil || ok {
		t.Fatalf("overlap: %v %v", ok, err)
	}
	if err := s.ReleaseCheckLock(ctx, "one"); err != nil {
		t.Fatal(err)
	}
	ok, err = s.AcquireCheckLock(ctx, "two", time.Minute)
	if err != nil || !ok {
		t.Fatalf("reacquire: %v %v", ok, err)
	}
	if err := s.QueueJob(ctx, "watch", "funda", "123", "telegram", 0, "new_match", "0"); err != nil {
		t.Fatal(err)
	}
	claimed, err := s.ClaimJob(ctx, "watch", "funda", "123", "telegram", 0, "new_match", "0")
	if err != nil || !claimed {
		t.Fatalf("claim: %v %v", claimed, err)
	}
	claimed, err = s.ClaimJob(ctx, "watch", "funda", "123", "telegram", 0, "new_match", "0")
	if err != nil || claimed {
		t.Fatalf("duplicate claim: %v %v", claimed, err)
	}
	if err := s.CompleteJob(ctx, "watch", "funda", "123", "telegram", 0, "new_match", "0", errors.New("temporary")); err != nil {
		t.Fatal(err)
	}
	claimed, err = s.ClaimJob(ctx, "watch", "funda", "123", "telegram", 0, "new_match", "0")
	if err != nil || claimed {
		t.Fatalf("backoff: %v %v", claimed, err)
	}
}

func TestSuspectedRemovalDoesNotReopen(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	now := time.Now().UTC()
	if err := s.UpsertRefs(ctx, []model.ListingRef{{Source: "funda", ID: "123", URL: "https://example.com/123", City: "Utrecht", TransactionType: "rent", FirstSeenAt: now, LastSeenAt: now}}); err != nil {
		t.Fatal(err)
	}
	refresh := func(string) time.Duration { return time.Minute }
	if err := s.RecordFetchSuccess(ctx, "funda", "123", "beschikbaar", refresh); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordFetchFailure(ctx, "funda", "123", 404, time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordFetchSuccess(ctx, "funda", "123", "beschikbaar", refresh); err != nil {
		t.Fatal(err)
	}
	gen, active, err := s.ActiveGeneration(ctx, "funda", "123")
	if err != nil || !active || gen != 0 {
		t.Fatalf("false reopen: gen=%d active=%v err=%v", gen, active, err)
	}
	if err := s.RecordFetchFailure(ctx, "funda", "123", 410, time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordFetchSuccess(ctx, "funda", "123", "beschikbaar", refresh); err != nil {
		t.Fatal(err)
	}
	gen, active, err = s.ActiveGeneration(ctx, "funda", "123")
	if err != nil || !active || gen != 1 {
		t.Fatalf("real reopen: gen=%d active=%v err=%v", gen, active, err)
	}
}
