package game

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAppendResult(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "history.json")
	r := SessionResult{Game: "guess", Mode: "rent", City: "utrecht", Seed: 123, Rounds: 5, Score: 3200, CompletedAt: time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC)}
	for i := 0; i < 2; i++ {
		if err := AppendResult(path, r); err != nil {
			t.Fatal(err)
		}
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got []SessionResult
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[1].Score != 3200 {
		t.Fatalf("unexpected history: %+v", got)
	}
	if strings.Contains(string(b), "asking_price") || strings.Contains(string(b), "listing_url") {
		t.Fatal("history leaked listing details")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0077 != 0 {
		t.Fatalf("history permissions too broad: %v", info.Mode())
	}
}
func TestAppendResultInvalidAndCorrupt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	if err := AppendResult(path, SessionResult{Game: "invalid", Mode: "buy"}); err == nil {
		t.Fatal("expected invalid game error")
	}
	if err := os.WriteFile(path, []byte("bad-json"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := AppendResult(path, SessionResult{Game: "hunter", Mode: "buy"}); err == nil {
		t.Fatal("expected invalid JSON error")
	}
	b, _ := os.ReadFile(path)
	if string(b) != "bad-json" {
		t.Fatal("corrupt file overwritten")
	}
}
