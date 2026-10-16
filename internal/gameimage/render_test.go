package gameimage

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"
)

func TestRenderLink(t *testing.T) {
	var b bytes.Buffer
	if err := Render(&b, Link, nil, "https://example.com/a.jpg"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "https://example.com/a.jpg") {
		t.Fatal(b.String())
	}
}
func TestRenderKittyChunks(t *testing.T) {
	var b bytes.Buffer
	data := bytes.Repeat([]byte("abc"), 1800)
	if err := Render(&b, Kitty, data, ""); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "m=1;") || !strings.Contains(b.String(), "m=0;") {
		t.Fatal("missing chunk markers")
	}
}
func TestRenderITerm2(t *testing.T) {
	var b bytes.Buffer
	data := []byte("hello")
	if err := Render(&b, ITerm2, data, ""); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), base64.StdEncoding.EncodeToString(data)) {
		t.Fatal("missing image data")
	}
}
func TestRenderFallbackAndSize(t *testing.T) {
	var b bytes.Buffer
	if err := Render(&b, Kitty, nil, ""); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "unavailable") {
		t.Fatal(b.String())
	}
	if err := Render(&b, Kitty, make([]byte, 2<<20+1), ""); err == nil {
		t.Fatal("oversized image accepted")
	}
}
