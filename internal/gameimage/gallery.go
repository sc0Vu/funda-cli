package gameimage

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// Gallery is an optional, user-provided mapping of source:listingID to
// photo URLs. Images are never fetched or redistributed by this package.
type Gallery map[string][]string

func LoadGallery(path string) (Gallery, error) {
	if path == "" {
		return Gallery{}, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(b) > 2<<20 {
		return nil, fmt.Errorf("photo manifest exceeds 2 MiB")
	}
	var g Gallery
	if err := json.Unmarshal(b, &g); err != nil {
		return nil, fmt.Errorf("decode photo manifest: %w", err)
	}
	if g == nil {
		return nil, fmt.Errorf("photo manifest must be a JSON object")
	}
	for key, urls := range g {
		if !strings.Contains(key, ":") || len(urls) > 32 {
			return nil, fmt.Errorf("invalid photo entry %q", key)
		}
		for _, u := range urls {
			if err := SafePhotoURL(u); err != nil {
				return nil, fmt.Errorf("photo %q: %w", key, err)
			}
		}
	}
	return g, nil
}

func (g Gallery) Show(w io.Writer, source, id string) (bool, error) {
	urls := g[source+":"+id]
	if len(urls) == 0 {
		return false, nil
	}
	for i, u := range urls {
		if _, err := fmt.Fprintf(w, "Photo %d/%d: %s\n", i+1, len(urls), u); err != nil {
			return true, err
		}
	}
	return true, nil
}
