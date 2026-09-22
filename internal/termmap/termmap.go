package termmap

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultEndpoint = "https://overpass-api.de/api/interpreter"

type Options struct {
	Lat, Lon      float64
	Width, Height int
	RadiusMeters  float64
	Endpoint      string
	Client        *http.Client
}

type point struct{ lat, lon float64 }
type feature struct {
	kind string
	pts  []point
}

type overpassResponse struct {
	Elements []struct {
		Type     string            `json:"type"`
		Tags     map[string]string `json:"tags"`
		Geometry []struct {
			Lat float64 `json:"lat"`
			Lon float64 `json:"lon"`
		} `json:"geometry"`
	} `json:"elements"`
}

func Render(ctx context.Context, o Options) (string, error) {
	if o.Lat == 0 && o.Lon == 0 {
		return "", fmt.Errorf("listing has no coordinates")
	}
	if o.Width <= 0 {
		o.Width = 80
	}
	if o.Height <= 0 {
		o.Height = 24
	}
	if o.RadiusMeters <= 0 {
		o.RadiusMeters = 600
	}
	if o.Endpoint == "" {
		o.Endpoint = defaultEndpoint
	}
	if o.Client == nil {
		o.Client = &http.Client{Timeout: 20 * time.Second}
	}

	features, err := fetch(ctx, o)
	if err != nil {
		return "", err
	}
	c := newCanvas(o.Width, o.Height)
	for _, f := range features {
		var prev *pixel
		for _, p := range f.pts {
			px := project(p, o, c.pixelWidth(), c.pixelHeight())
			if prev != nil {
				c.line(prev.x, prev.y, px.x, px.y)
			}
			q := px
			prev = &q
		}
	}
	// Property marker: a small cross centered on the listing.
	cx, cy := c.pixelWidth()/2, c.pixelHeight()/2
	c.line(cx-2, cy, cx+2, cy)
	c.line(cx, cy-2, cx, cy+2)
	return c.String(), nil
}

func fetch(ctx context.Context, o Options) ([]feature, error) {
	latDelta := o.RadiusMeters / 111320.0
	lonScale := math.Cos(o.Lat * math.Pi / 180)
	if math.Abs(lonScale) < 0.01 {
		lonScale = 0.01
	}
	lonDelta := o.RadiusMeters / (111320.0 * lonScale)
	south, west := o.Lat-latDelta, o.Lon-lonDelta
	north, east := o.Lat+latDelta, o.Lon+lonDelta
	bbox := fmt.Sprintf("%.6f,%.6f,%.6f,%.6f", south, west, north, east)
	q := fmt.Sprintf(`[out:json][timeout:15];(way["highway"](%s);way["building"](%s);way["waterway"](%s);way["natural"="water"](%s);way["leisure"="park"](%s);way["landuse"="grass"](%s););out geom;`, bbox, bbox, bbox, bbox, bbox, bbox)
	form := url.Values{"data": {q}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.Endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "funda-cli/0.1 terminal-map")
	resp, err := o.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch OpenStreetMap data: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("OpenStreetMap Overpass returned %s: %s", resp.Status, strings.TrimSpace(string(b)))
	}
	var data overpassResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("decode OpenStreetMap data: %w", err)
	}
	out := make([]feature, 0, len(data.Elements))
	for _, e := range data.Elements {
		if len(e.Geometry) < 2 {
			continue
		}
		f := feature{kind: classify(e.Tags), pts: make([]point, 0, len(e.Geometry))}
		for _, g := range e.Geometry {
			f.pts = append(f.pts, point{g.Lat, g.Lon})
		}
		out = append(out, f)
	}
	return out, nil
}

func classify(tags map[string]string) string {
	if tags["highway"] != "" {
		return "road"
	}
	if tags["building"] != "" {
		return "building"
	}
	if tags["waterway"] != "" || tags["natural"] == "water" {
		return "water"
	}
	return "land"
}

type pixel struct{ x, y int }

func project(p point, o Options, w, h int) pixel {
	latDelta := o.RadiusMeters / 111320.0
	lonDelta := o.RadiusMeters / (111320.0 * math.Max(0.01, math.Cos(o.Lat*math.Pi/180)))
	x := int(math.Round(((p.lon - (o.Lon - lonDelta)) / (2 * lonDelta)) * float64(w-1)))
	y := int(math.Round((1 - ((p.lat - (o.Lat - latDelta)) / (2 * latDelta))) * float64(h-1)))
	return pixel{x, y}
}

type canvas struct {
	width, height int
	cells         []uint8
}

func newCanvas(w, h int) *canvas   { return &canvas{w, h, make([]uint8, w*h)} }
func (c *canvas) pixelWidth() int  { return c.width * 2 }
func (c *canvas) pixelHeight() int { return c.height * 4 }
func (c *canvas) set(x, y int) {
	if x < 0 || y < 0 || x >= c.pixelWidth() || y >= c.pixelHeight() {
		return
	}
	dots := [4][2]uint8{{1, 8}, {2, 16}, {4, 32}, {64, 128}}
	idx := (y/4)*c.width + x/2
	c.cells[idx] |= dots[y%4][x%2]
}
func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
func (c *canvas) line(x0, y0, x1, y1 int) {
	dx, sx := abs(x1-x0), 1
	if x0 > x1 {
		sx = -1
	}
	dy, sy := -abs(y1-y0), 1
	if y0 > y1 {
		sy = -1
	}
	err := dx + dy
	for {
		c.set(x0, y0)
		if x0 == x1 && y0 == y1 {
			break
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
}
func (c *canvas) String() string {
	var b strings.Builder
	for y := 0; y < c.height; y++ {
		for x := 0; x < c.width; x++ {
			b.WriteRune(rune(0x2800 + int(c.cells[y*c.width+x])))
		}
		b.WriteByte('\n')
	}
	return b.String()
}
