package app

import (
	"testing"

	"github.com/sc0vu/funda-cli/internal/model"
)

func TestFormatListingMessage(t *testing.T) {
	mv := model.Listing{Source: "mvgm", Address: "A 1", City: "Utrecht", Price: 1500, LivingArea: 60, Bedrooms: 2, URL: "http://x/l"}
	base := "🏠 w (new): A 1\nUtrecht\n€1500 · 60 m² · 2 bedrooms\n"
	fd := mv
	fd.Source = "funda"

	tests := []struct {
		name string
		l    model.Listing
		d    *model.RentalDetails
		want string
	}{
		{"no details", mv, nil, base + "http://x/l"},
		{"drawing with url", mv, &model.RentalDetails{HasDrawing: true, DrawingURLs: []string{"http://d/1", "http://d/2"}}, base + "📐 Drawing: yes\n📐 http://d/1\nhttp://x/l"},
		{"drawing without url", mv, &model.RentalDetails{HasDrawing: true}, base + "📐 Drawing: yes\nhttp://x/l"},
		{"no drawing", mv, &model.RentalDetails{}, base + "📐 Drawing: no\nhttp://x/l"},
		{"funda unaffected", fd, &model.RentalDetails{HasDrawing: true}, base + "http://x/l"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatListingMessage("w", "new", tt.l, tt.d); got != tt.want {
				t.Errorf("got %q want %q", got, tt.want)
			}
		})
	}
}
