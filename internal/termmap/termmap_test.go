package termmap

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestRender(t *testing.T) {
	body := `{"elements":[{"type":"way","tags":{"highway":"residential"},"geometry":[{"lat":52.09,"lon":5.115},{"lat":52.09,"lon":5.127}]}]}`
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Status: "200 OK", Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	got, err := Render(context.Background(), Options{Lat: 52.09, Lon: 5.121, Width: 20, Height: 8, RadiusMeters: 600, Client: client, Endpoint: "https://example.test"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.ContainsAny(got, "⠁⠂⠄⡀⢀⣀⣿") {
		t.Fatalf("expected braille map, got %q", got)
	}
	if lines := strings.Count(got, "\n"); lines != 8 {
		t.Fatalf("got %d lines, want 8", lines)
	}
}

func TestRenderRequiresCoordinates(t *testing.T) {
	if _, err := Render(context.Background(), Options{}); err == nil {
		t.Fatal("expected error")
	}
}
