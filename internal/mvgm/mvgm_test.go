package mvgm

import (
	"reflect"
	"testing"
)

func TestParseCoordinate(t *testing.T) {
	body := `<script type="application/ld+json">{"geo":{"latitude":52.0907,"longitude":"5.1214"}}</script>`
	if got := parseCoordinate(latitudeRE, body); got != 52.0907 {
		t.Fatalf("latitude=%v", got)
	}
	if got := parseCoordinate(longitudeRE, body); got != 5.1214 {
		t.Fatalf("longitude=%v", got)
	}
}

func TestParseDrawing(t *testing.T) {
	tests := []struct {
		name string
		html string
		want []string
	}{
		{"img", `<h2>Plattegrond</h2><img src="https://cdn.example.com/plan.jpg"><h2>Kenmerken</h2>`, []string{"https://cdn.example.com/plan.jpg"}},
		{"pdf", `<h2>Plattegronden</h2><a href="https://x.nl/plan.PDF?v=1">Download</a>`, []string{"https://x.nl/plan.PDF?v=1"}},
		{"relative", `<h2>Floor plan</h2><img data-src="/media/a.png"><source srcset="//img.nl/b.webp 1x, /c.svg 2x">`,
			[]string{"https://ikwilhuren.nu/media/a.png", "https://img.nl/b.webp", "https://ikwilhuren.nu/c.svg"}},
		{"dedupe", `<h2>Plattegrond</h2><img src="/a.jpg"><a href="/a.jpg">x</a><img src="/a.jpg?x=1&amp;y=2"><img src="/b.jpg">`,
			[]string{"https://ikwilhuren.nu/a.jpg", "https://ikwilhuren.nu/a.jpg?x=1&y=2", "https://ikwilhuren.nu/b.jpg"}},
		{"label only", `<nav>Plattegrond</nav><p>Beschikbaar vanaf 1 maart Plattegrond Kenmerken</p><img src="/photo.jpg">`, nil},
		{"no word", `<img src="/photo.jpg"><p>Kenmerken</p>`, nil},
		{"outside section", `<h2>Plattegrond</h2><h2>Kenmerken</h2><img src="/photo.jpg">`, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			has, got := parseDrawing(tc.html)
			if has != (len(tc.want) > 0) {
				t.Fatalf("has=%v", has)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("urls=%v want %v", got, tc.want)
			}
		})
	}
}
