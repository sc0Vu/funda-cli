package app

import (
	"bufio"
	"io"
	"strings"
	"testing"
)

func TestAskInt(t *testing.T) {
	r := bufio.NewReader(strings.NewReader("not-a-number\n-1\n12345\n"))
	n, err := askInt(r, "test: ")
	if err != nil || n != 12345 {
		t.Fatalf("got %d, %v", n, err)
	}
}
func TestAskIntQuit(t *testing.T) {
	_, err := askInt(bufio.NewReader(strings.NewReader("q\n")), "test: ")
	if err != io.EOF {
		t.Fatalf("got %v", err)
	}
}
func TestOpenListingRejectsUntrustedURLs(t *testing.T) {
	for _, u := range []string{"", "file:///etc/passwd", "https://evil.example/", "https://www.funda.nl.evil.example/a"} {
		if err := openListing(u); err == nil {
			t.Errorf("accepted %q", u)
		}
	}
}
