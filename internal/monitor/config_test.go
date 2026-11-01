package monitor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMultipleWatches(t *testing.T) {
	p := filepath.Join(t.TempDir(), "monitor.json")
	data := `{"providers":{"tg":{"type":"telegram"}},"watches":[{"name":"one","source":"funda","city":"utrecht","category":"buy","notify":["tg"]},{"name":"two","source":"mvgm","city":"utrecht","category":"rent","notify":["tg"]}]}`
	if err := os.WriteFile(p, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Watches) != 2 || c.Interval != "10m" {
		t.Fatalf("unexpected config: %+v", c)
	}
}
func TestLoadRejectsUnknownProvider(t *testing.T) {
	p := filepath.Join(t.TempDir(), "monitor.json")
	data := `{"watches":[{"name":"one","source":"funda","city":"utrecht","category":"buy","notify":["missing"]}]}`
	if err := os.WriteFile(p, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); err == nil {
		t.Fatal("expected validation error")
	}
}
