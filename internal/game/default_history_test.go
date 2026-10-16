package game

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultHistoryPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")
	p, err := DefaultHistoryPath()
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(p) || filepath.Base(p) != "game-history.json" {
		t.Fatalf("invalid history path %q", p)
	}
}

func TestHistoryRoundTripAndCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "history.json")
	r := SessionResult{Game: "guess", Mode: "buy", City: "utrecht", Seed: 42, Rounds: 5, Score: 4000, CompletedAt: time.Now()}
	if err := AppendResult(path, r); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got []SessionResult
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Seed != 42 {
		t.Fatalf("unexpected history %+v", got)
	}
	if err := os.WriteFile(path, []byte("bad json"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := AppendResult(path, r); err == nil {
		t.Fatal("expected corrupt history error")
	}
	after, _ := os.ReadFile(path)
	if string(after) != "bad json" {
		t.Fatal("corrupt history overwritten")
	}
}
