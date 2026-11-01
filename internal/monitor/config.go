package monitor

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

type Config struct {
	Interval       string              `json:"interval"`
	NotifyExisting bool                `json:"notify_existing"`
	RefreshAfter   string              `json:"refresh_after"`
	EnrichLimit    int                 `json:"enrich_limit"`
	StatusRefresh  StatusRefresh       `json:"status_refresh"`
	Providers      map[string]Provider `json:"providers"`
	Watches        []Watch             `json:"watches"`
}
type StatusRefresh struct {
	Active           string `json:"active"`
	UnderOption      string `json:"under_option"`
	Unavailable      string `json:"unavailable"`
	SuspectedRemoved string `json:"suspected_removed"`
	Removed          string `json:"removed"`
	Unknown          string `json:"unknown"`
}

func (r StatusRefresh) Interval(status string) time.Duration {
	value := r.Unknown
	switch status {
	case "active":
		value = r.Active
	case "under_option":
		value = r.UnderOption
	case "unavailable":
		value = r.Unavailable
	case "suspected_removed":
		value = r.SuspectedRemoved
	case "removed":
		value = r.Removed
	}
	d, err := time.ParseDuration(value)
	if err != nil || d <= 0 {
		return time.Hour
	}
	return d
}

type Provider struct {
	Type        string   `json:"type"`
	TokenEnv    string   `json:"token_env,omitempty"`
	ChatIDEnv   string   `json:"chat_id_env,omitempty"`
	URLEnv      string   `json:"url_env,omitempty"`
	Host        string   `json:"host,omitempty"`
	Port        int      `json:"port,omitempty"`
	UsernameEnv string   `json:"username_env,omitempty"`
	PasswordEnv string   `json:"password_env,omitempty"`
	From        string   `json:"from,omitempty"`
	To          []string `json:"to,omitempty"`
}
type Watch struct {
	Name        string   `json:"name"`
	Source      string   `json:"source"`
	City        string   `json:"city"`
	Category    string   `json:"category"`
	MaxPrice    float64  `json:"max_price"`
	MinArea     float64  `json:"min_area"`
	MinBedrooms int      `json:"min_bedrooms"`
	Notify      []string `json:"notify"`
}

func Load(path string) (Config, error) {
	var c Config
	b, err := os.ReadFile(path)
	if err != nil {
		return c, err
	}
	if err = json.Unmarshal(b, &c); err != nil {
		return c, fmt.Errorf("parse config: %w", err)
	}
	if c.Interval == "" {
		c.Interval = "10m"
	}
	if c.RefreshAfter == "" {
		c.RefreshAfter = "30m"
	}
	if c.StatusRefresh.Active == "" {
		c.StatusRefresh.Active = "30m"
	}
	if c.StatusRefresh.UnderOption == "" {
		c.StatusRefresh.UnderOption = "2h"
	}
	if c.StatusRefresh.Unavailable == "" {
		c.StatusRefresh.Unavailable = "24h"
	}
	if c.StatusRefresh.SuspectedRemoved == "" {
		c.StatusRefresh.SuspectedRemoved = "30m"
	}
	if c.StatusRefresh.Removed == "" {
		c.StatusRefresh.Removed = "168h"
	}
	if c.StatusRefresh.Unknown == "" {
		c.StatusRefresh.Unknown = "1h"
	}
	for _, value := range []string{c.StatusRefresh.Active, c.StatusRefresh.UnderOption, c.StatusRefresh.Unavailable, c.StatusRefresh.SuspectedRemoved, c.StatusRefresh.Removed, c.StatusRefresh.Unknown} {
		d, e := time.ParseDuration(value)
		if e != nil || d <= 0 {
			return c, fmt.Errorf("invalid status_refresh interval %q", value)
		}
	}
	if c.EnrichLimit <= 0 {
		c.EnrichLimit = 50
	}
	if _, err = time.ParseDuration(c.Interval); err != nil {
		return c, fmt.Errorf("interval: %w", err)
	}
	if _, err = time.ParseDuration(c.RefreshAfter); err != nil {
		return c, fmt.Errorf("refresh_after: %w", err)
	}
	names := map[string]bool{}
	for _, w := range c.Watches {
		if w.Name == "" || names[w.Name] {
			return c, fmt.Errorf("watch names must be nonempty and unique")
		}
		names[w.Name] = true
		if w.Source != "funda" && w.Source != "mvgm" {
			return c, fmt.Errorf("watch %s: source must be funda or mvgm", w.Name)
		}
		if w.Category != "rent" && w.Category != "buy" && w.Category != "newbuild" {
			return c, fmt.Errorf("watch %s: category must be rent, buy or newbuild", w.Name)
		}
		if w.Source == "mvgm" && w.Category != "rent" {
			return c, fmt.Errorf("watch %s: mvgm supports rent only", w.Name)
		}
		if w.MaxPrice < 0 || w.MinArea < 0 || w.MinBedrooms < 0 {
			return c, fmt.Errorf("watch %s: negative filters", w.Name)
		}
		for _, n := range w.Notify {
			if _, ok := c.Providers[n]; !ok {
				return c, fmt.Errorf("watch %s: unknown provider %s", w.Name, n)
			}
		}
	}
	for n, p := range c.Providers {
		if p.Type != "telegram" && p.Type != "webhook" && p.Type != "smtp" {
			return c, fmt.Errorf("provider %s: unsupported type %s", n, p.Type)
		}
		if p.Type == "smtp" {
			if p.Host == "" || (p.Port != 465 && p.Port != 587) || p.From == "" || len(p.To) == 0 {
				return c, fmt.Errorf("provider %s: SMTP requires host, port (465 or 587), from and to", n)
			}
		}
	}
	return c, nil
}
