package mvgm

import "testing"

func TestParseCoordinate(t *testing.T) {
	body := `<script type="application/ld+json">{"geo":{"latitude":52.0907,"longitude":"5.1214"}}</script>`
	if got := parseCoordinate(latitudeRE, body); got != 52.0907 {
		t.Fatalf("latitude=%v", got)
	}
	if got := parseCoordinate(longitudeRE, body); got != 5.1214 {
		t.Fatalf("longitude=%v", got)
	}
}
