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

var defaultEndpoints = []string{
	"https://overpass-api.de/api/interpreter",
	"https://overpass.private.coffee/api/interpreter",
	"https://maps.mail.ru/osm/tools/overpass/api/interpreter",
}

type Options struct {
	Lat, Lon      float64
	Width, Height int
	RadiusMeters  float64
	Endpoint      string
	Client        *http.Client
	Color         bool
	Buildings     bool
}

type point struct{ lat, lon float64 }
type feature struct {
	kind string
	pts  []point
}

type overpassResponse struct {
	Elements []struct {
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
		o.Width = 90
	}
	if o.Height <= 0 {
		o.Height = 26
	}
	if o.RadiusMeters <= 0 {
		o.RadiusMeters = 450
	}
	if o.Client == nil {
		o.Client = &http.Client{Timeout: 35 * time.Second}
	}

	features, err := fetch(ctx, o)
	if err != nil {
		return "", err
	}
	c := newCanvas(o.Width, o.Height, o.Color)
	// Low-priority areas first; roads/water then overwrite their colour.
	for _, want := range []string{"park", "building", "road-local", "road-major", "water"} {
		for _, f := range features {
			if f.kind != want {
				continue
			}
			var prev *pixel
			for _, p := range f.pts {
				px := project(p, o, c.pixelWidth(), c.pixelHeight())
				if prev != nil {
					c.featureLine(prev.x, prev.y, px.x, px.y, f.kind)
				}
				q := px
				prev = &q
			}
		}
	}
	cx, cy := c.pixelWidth()/2, c.pixelHeight()/2
	c.line(cx-3, cy, cx+3, cy, "property")
	c.line(cx, cy-3, cx, cy+3, "property")

	var b strings.Builder
	b.WriteString(c.String())
	if o.Color {
		b.WriteString("\x1b[0m")
		b.WriteString("Legend: \x1b[91m+ property\x1b[0m  \x1b[33mmajor road\x1b[0m  road  \x1b[96mwater\x1b[0m  \x1b[32mpark\x1b[0m")
		if o.Buildings {
			b.WriteString("  \x1b[90mbuilding\x1b[0m")
		}
		b.WriteByte('\n')
	} else {
		b.WriteString("Legend: center cross = property; map shows roads, water and parks")
		if o.Buildings {
			b.WriteString(", buildings")
		}
		b.WriteByte('\n')
	}
	return b.String(), nil
}

func fetch(ctx context.Context, o Options) ([]feature, error) {
	bbox := bounds(o)
	selectors := []string{
		`way["highway"](` + bbox + `);`,
		`way["waterway"](` + bbox + `);`,
		`way["natural"="water"](` + bbox + `);`,
		`way["leisure"="park"](` + bbox + `);`,
		`way["landuse"~"^(grass|recreation_ground|forest)$"](` + bbox + `);`,
	}
	// Buildings are intentionally opt-in: in Dutch cities they dominate the
	// Braille canvas and make both the map and Overpass query much heavier.
	if o.Buildings {
		selectors = append(selectors, `way["building"](`+bbox+`);`)
	}
	q := `[out:json][timeout:20];(` + strings.Join(selectors, "") + `);out geom;`

	endpoints := defaultEndpoints
	if o.Endpoint != "" {
		endpoints = []string{o.Endpoint}
	}
	var errs []string
	for _, endpoint := range endpoints {
		data, err := fetchEndpoint(ctx, o.Client, endpoint, q)
		if err == nil {
			return decodeFeatures(data), nil
		}
		errs = append(errs, endpoint+": "+err.Error())
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}
	return nil, fmt.Errorf("OpenStreetMap map request failed on all Overpass endpoints:\n  %s", strings.Join(errs, "\n  "))
}

func bounds(o Options) string {
	latDelta := o.RadiusMeters / 111320.0
	lonScale := math.Max(0.01, math.Abs(math.Cos(o.Lat*math.Pi/180)))
	lonDelta := o.RadiusMeters / (111320.0 * lonScale)
	return fmt.Sprintf("%.6f,%.6f,%.6f,%.6f", o.Lat-latDelta, o.Lon-lonDelta, o.Lat+latDelta, o.Lon+lonDelta)
}

func fetchEndpoint(ctx context.Context, client *http.Client, endpoint, q string) (overpassResponse, error) {
	var out overpassResponse
	form := url.Values{"data": {q}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return out, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "funda-cli/0.2 terminal-map")
	resp, err := client.Do(req)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return out, fmt.Errorf("%s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return out, fmt.Errorf("decode response: %w", err)
	}
	return out, nil
}

func decodeFeatures(data overpassResponse) []feature {
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
	return out
}

func classify(tags map[string]string) string {
	if h := tags["highway"]; h != "" {
		switch h {
		case "motorway", "trunk", "primary", "secondary":
			return "road-major"
		}
		return "road-local"
	}
	if tags["waterway"] != "" || tags["natural"] == "water" {
		return "water"
	}
	if tags["leisure"] == "park" || tags["landuse"] == "grass" || tags["landuse"] == "recreation_ground" || tags["landuse"] == "forest" {
		return "park"
	}
	if tags["building"] != "" {
		return "building"
	}
	return "road-local"
}

type pixel struct{ x, y int }

func project(p point, o Options, w, h int) pixel {
	latDelta := o.RadiusMeters / 111320.0
	lonDelta := o.RadiusMeters / (111320.0 * math.Max(0.01, math.Abs(math.Cos(o.Lat*math.Pi/180))))
	x := int(math.Round(((p.lon - (o.Lon - lonDelta)) / (2 * lonDelta)) * float64(w-1)))
	y := int(math.Round((1 - ((p.lat - (o.Lat - latDelta)) / (2 * latDelta))) * float64(h-1)))
	return pixel{x, y}
}

type cell struct {
	dots uint8
	kind string
}
type canvas struct {
	width, height int
	cells         []cell
	color         bool
}

func newCanvas(w, h int, color bool) *canvas { return &canvas{w, h, make([]cell, w*h), color} }
func (c *canvas) pixelWidth() int            { return c.width * 2 }
func (c *canvas) pixelHeight() int           { return c.height * 4 }
func priority(k string) int {
	switch k {
	case "property":
		return 6
	case "water":
		return 5
	case "road-major":
		return 4
	case "road-local":
		return 3
	case "building":
		return 2
	case "park":
		return 1
	}
	return 0
}
func (c *canvas) set(x, y int, kind string) {
	if x < 0 || y < 0 || x >= c.pixelWidth() || y >= c.pixelHeight() {
		return
	}
	dots := [4][2]uint8{{1, 8}, {2, 16}, {4, 32}, {64, 128}}
	i := (y/4)*c.width + x/2
	c.cells[i].dots |= dots[y%4][x%2]
	if priority(kind) >= priority(c.cells[i].kind) {
		c.cells[i].kind = kind
	}
}
func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
func (c *canvas) featureLine(x0, y0, x1, y1 int, kind string) {
	c.line(x0, y0, x1, y1, kind)
	// Give the most important geography a distinct weight even when ANSI
	// colours are disabled. This also makes canals and arterial roads easier
	// to distinguish on high-DPI terminals.
	switch kind {
	case "water":
		c.line(x0+1, y0, x1+1, y1, kind)
	case "road-major":
		c.line(x0, y0+1, x1, y1+1, kind)
	}
}

func (c *canvas) line(x0, y0, x1, y1 int, kind string) {
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
		c.set(x0, y0, kind)
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
func ansi(k string) string {
	switch k {
	case "property":
		return "\x1b[91m"
	case "water":
		return "\x1b[96m"
	case "road-major":
		return "\x1b[33m"
	case "park":
		return "\x1b[32m"
	case "building":
		return "\x1b[90m"
	}
	return "\x1b[37m"
}
func (c *canvas) String() string {
	var b strings.Builder
	last := ""
	for y := 0; y < c.height; y++ {
		for x := 0; x < c.width; x++ {
			v := c.cells[y*c.width+x]
			if v.dots == 0 {
				if c.color && last != "" {
					b.WriteString("\x1b[0m")
					last = ""
				}
				b.WriteByte(' ')
				continue
			}
			if c.color {
				a := ansi(v.kind)
				if a != last {
					b.WriteString(a)
					last = a
				}
			}
			b.WriteRune(rune(0x2800 + int(v.dots)))
		}
		if c.color && last != "" {
			b.WriteString("\x1b[0m")
			last = ""
		}
		b.WriteByte('\n')
	}
	return b.String()
}
