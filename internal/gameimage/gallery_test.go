package gameimage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGalleryLoadAndShow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "photos.json")
	if err := os.WriteFile(path, []byte(`{"funda:42":["https://images.example.com/a.jpg","https://images.example.com/b.jpg"]}`), 0600); err != nil {
		t.Fatal(err)
	}
	g, err := LoadGallery(path)
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	ok, err := g.Show(&b, "funda", "42")
	if err != nil || !ok || !strings.Contains(b.String(), "Photo 2/2") {
		t.Fatalf("%v %v %s", ok, err, b.String())
	}
	ok, err = g.Show(&b, "funda", "missing")
	if err != nil || ok {
		t.Fatal("unexpected gallery")
	}
}
func TestGalleryRejectsUnsafeURLs(t *testing.T) {
	for _, v := range []string{`{"funda:1":["file:///etc/passwd"]}`, `{"funda:1":["https://example.com/\nunsafe"]}`, `[]`, `{"bad":["https://example.com/a.jpg"]}`} {
		p := filepath.Join(t.TempDir(), "bad.json")
		if err := os.WriteFile(p, []byte(v), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadGallery(p); err == nil {
			t.Fatalf("accepted %s", v)
		}
	}
}
