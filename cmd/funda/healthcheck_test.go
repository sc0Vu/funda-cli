package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRunHealth(t *testing.T) {
	path := filepath.Join(t.TempDir(), "heartbeat")
	if err := os.WriteFile(path, []byte(time.Now().UTC().Format(time.RFC3339Nano)+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := runHealth([]string{"--file", path, "--max-age", "30m"}); err != nil {
		t.Fatalf("fresh heartbeat rejected: %v", err)
	}
	if err := os.WriteFile(path, []byte(time.Now().Add(-time.Hour).UTC().Format(time.RFC3339Nano)+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := runHealth([]string{"--file", path, "--max-age", "30m"}); err == nil {
		t.Fatal("expected stale heartbeat to fail")
	}
	if err := os.WriteFile(path, []byte("invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := runHealth([]string{"--file", path}); err == nil {
		t.Fatal("expected malformed heartbeat to fail")
	}
}
