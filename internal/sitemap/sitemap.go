package sitemap

import (
	"context"
	"compress/gzip"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/North-web-dev/impersonate-http"
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

type Client struct {
	HTTP *http.Client
}

func NewClient(profile string, timeout time.Duration) (*Client, error) {
	if profile == "" {
		profile = "chrome"
	}

	httpClient, ok := impersonate.NewByName(
		strings.ToLower(profile),
		impersonate.WithTimeout(timeout),
	)
	if !ok {
		return nil, fmt.Errorf("unknown browser profile %q", profile)
	}

	return &Client{HTTP: httpClient}, nil
}

func (c *Client) Fetch(ctx context.Context, sitemapURL, category string) ([]model.ListingRef, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sitemapURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set(
		"Accept",
		"application/xml,text/xml;q=0.9,*/*;q=0.8",
	)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sitemap request failed: %s", resp.Status)
	}

	var responseBody io.ReadCloser
	contentEncoding := resp.Header.Get("Content-Encoding")
	if !resp.Uncompressed {
		switch contentEncoding {
		case "":
			responseBody = resp.Body
			break
		case "gzip":
			gzReader, err := gzip.NewReader(resp.Body)
			if err != nil {
				return nil, fmt.Errorf("create gzip reader: %w", err)
			}
			defer gzReader.Close()
			responseBody = gzReader
			break
		default:
			return nil, fmt.Errorf("unknown content encoding: %s", contentEncoding)
		}
	}

	body, err := io.ReadAll(responseBody)
	if err != nil {
		return nil, err
	}

	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "text/html") {
		return nil, fmt.Errorf("expected sitemap XML, received HTML")
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
		refs, err := c.Fetch(ctx, child.Loc, category)
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
