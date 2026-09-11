package sitemap

import (
	"context"
	"encoding/xml"
	"fmt"

	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/sc0vu/funda-cli/internal/client"
	"github.com/sc0vu/funda-cli/internal/model"
)

var idPattern = regexp.MustCompile(`/([0-9]{6,12})/?$`)

type urlSet struct {
	URLs []struct {
		Loc     string `xml:"loc"`
		LastMod string `xml:"lastmod"`
	} `xml:"url"`
}

type indexSet struct {
	Sitemaps []struct {
		Loc string `xml:"loc"`
	} `xml:"sitemap"`
}

func Fetch(ctx context.Context, profile string, timeout time.Duration, sitemapURL, category string) ([]model.ListingRef, error) {
	c, err := client.NewClient(profile, timeout)
	if err != nil {
		return nil, err
	}

	header := http.Header{
		"Accept": {
			"application/xml,text/xml;q=0.9,*/*;q=0.8",
		},
	}

	body, err := c.Get(ctx, sitemapURL, header)
	if err != nil {
		return nil, err
	}

	var set urlSet
	if err := xml.Unmarshal(body, &set); err == nil && len(set.URLs) > 0 {
		return parseURLSet(set, category), nil
	}

	var idx indexSet
	if err := xml.Unmarshal(body, &idx); err != nil {
		return nil, fmt.Errorf("parse sitemap: %w", err)
	}

	var out []model.ListingRef
	for _, child := range idx.Sitemaps {
		if !matchesCategory(child.Loc, category) {
			continue
		}
		refs, err := Fetch(ctx, profile, timeout, child.Loc, category)
		if err != nil {
			return nil, err
		}
		out = append(out, refs...)
	}
	return out, nil
}

func parseURLSet(set urlSet, category string) []model.ListingRef {
	now := time.Now().UTC()
	out := make([]model.ListingRef, 0, len(set.URLs))
	for _, item := range set.URLs {
		if category != "" && !matchesCategory(item.Loc, category) {
			continue
		}
		id := ListingID(item.Loc)
		if id == "" {
			continue
		}
		out = append(out, model.ListingRef{
			Source:          "funda",
			ID:              id,
			URL:             item.Loc,
			City:            cityFromURL(item.Loc),
			TransactionType: categoryFromURL(item.Loc),
			LastMod:         item.LastMod,
			FirstSeenAt:     now,
			LastSeenAt:      now,
		})
	}
	return out
}

func matchesCategory(raw, category string) bool {
	if category == "" || category == "all" {
		return true
	}
	l := strings.ToLower(raw)
	switch category {
	case "rent", "huur":
		return strings.Contains(l, "huur")
	case "buy", "koop":
		return strings.Contains(l, "koop")
	case "newbuild", "nieuwbouw":
		return strings.Contains(l, "nieuwbouw")
	default:
		return true
	}
}

func categoryFromURL(raw string) string {
	l := strings.ToLower(raw)
	switch {
	case strings.Contains(l, "/huur/"):
		return "rent"
	case strings.Contains(l, "/koop/"):
		return "buy"
	case strings.Contains(l, "nieuwbouw"):
		return "newbuild"
	default:
		return "unknown"
	}
}

func ListingID(raw string) string {
	m := idPattern.FindStringSubmatch(raw)
	if len(m) == 2 {
		return m[1]
	}
	return ""
}

func cityFromURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	parts := strings.Split(strings.Trim(path.Clean(u.Path), "/"), "/")
	// Common shape: detail/{huur|koop}/{city}/{property-slug}/{id}
	if len(parts) >= 3 && parts[0] == "detail" {
		return strings.ReplaceAll(parts[2], "-", " ")
	}
	return ""
}
