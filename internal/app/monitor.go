package app

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"github.com/sc0vu/funda-cli/internal/client"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sc0vu/funda-cli/internal/enrich"
	"github.com/sc0vu/funda-cli/internal/model"
	"github.com/sc0vu/funda-cli/internal/monitor"
	"github.com/sc0vu/funda-cli/internal/mvgm"
	"github.com/sc0vu/funda-cli/internal/store"
)

func runNotify(ctx context.Context, args []string) error {
	if len(args) == 0 || args[0] != "test" {
		return fmt.Errorf("usage: funda notify test --config monitor.json --provider NAME")
	}
	fs := flag.NewFlagSet("notify test", flag.ContinueOnError)
	path := fs.String("config", "monitor.json", "monitor config path")
	name := fs.String("provider", "", "provider name")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	c, err := monitor.Load(*path)
	if err != nil {
		return err
	}
	p, ok := c.Providers[*name]
	if !ok {
		return fmt.Errorf("unknown provider %q", *name)
	}
	return monitor.Send(ctx, p, "funda-cli: test notification")
}
func runMonitor(ctx context.Context, s *store.Store, command string, args []string) error {
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	path := fs.String("config", "monitor.json", "monitor config path")
	noNotify := fs.Bool("no-notify", false, "fetch and enrich without sending alerts")
	if err := fs.Parse(args); err != nil {
		return err
	}
	c, err := monitor.Load(*path)
	if err != nil {
		return err
	}
	if err := s.InitNotifications(); err != nil {
		return err
	}
	run := func() error { return checkCycle(ctx, s, c, *noNotify) }
	if command == "check" {
		return run()
	}
	interval, _ := time.ParseDuration(c.Interval)
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "check:", err)
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := run(); err != nil {
				fmt.Fprintln(os.Stderr, "check:", err)
			}
		}
	}
}
func checkCycle(ctx context.Context, s *store.Store, c monitor.Config, noNotify bool) error {
	owner := strconv.FormatInt(time.Now().UnixNano(), 10) + "-" + strconv.Itoa(os.Getpid())
	locked, lockErr := s.AcquireCheckLock(ctx, owner, 2*time.Hour)
	if lockErr != nil {
		return lockErr
	}
	if !locked {
		return fmt.Errorf("another funda check is already running")
	}
	defer s.ReleaseCheckLock(context.Background(), owner)
	// Capture existing references before sync, so the initial scan can still
	// alert on genuinely newly discovered references.
	initializedAtStart := map[string]bool{}
	for _, w := range c.Watches {
		initialized, err := s.WatchInitialized(ctx, w.Name)
		if err != nil {
			return err
		}
		initializedAtStart[w.Name] = initialized
		if !initialized && !c.NotifyExisting {
			if err := s.BaselineKnownRefs(ctx, w.Name, w.Source, w.City, w.Category); err != nil {
				return err
			}
		}
	}
	refresh, _ := time.ParseDuration(c.RefreshAfter)
	cutoff := time.Now().Add(-refresh)
	// Each source/city/category is discovered once, regardless of watch count.
	discovered := map[string]bool{}
	for _, w := range c.Watches {
		if err := ctx.Err(); err != nil {
			return err
		}
		key := w.Source + "|" + strings.ToLower(w.City) + "|" + w.Category
		if discovered[key] {
			continue
		}
		discovered[key] = true
		if w.City == "" {
			return fmt.Errorf("watch %s: city is required", w.Name)
		}
		var err error
		if w.Source == "mvgm" {
			err = runSync(ctx, s, []string{"--source", "mvgm", "--city", w.City})
		} else {
			err = runSync(ctx, s, []string{"--source", "funda", "--category", w.Category})
		}
		if err != nil {
			return fmt.Errorf("sync %s: %w", key, err)
		}
	}
	// Refresh new and stale listings before evaluating any watch.
	refreshed := map[string]bool{}
	priceDrops := map[string]string{}
	for _, w := range c.Watches {
		refs, err := s.RefreshCandidates(ctx, w.Source, w.City, w.Category, cutoff, c.EnrichLimit)
		if err != nil {
			return err
		}
		for _, r := range refs {
			key := r.Source + "|" + r.ID
			if refreshed[key] {
				continue
			}
			refreshed[key] = true
			var l model.Listing
			if r.Source == "mvgm" {
				var d model.RentalDetails
				l, d, err = mvgm.Fetch(ctx, "chrome", 30*time.Second, r)
				if err == nil {
					err = s.UpsertListing(ctx, l)
				}
				if err == nil {
					err = s.UpsertRentalDetails(ctx, d)
				}
			} else {
				l, err = enrich.Fetch(ctx, "chrome", 30*time.Second, r.ID)
				if err == nil {
					l.Source = r.Source
					l.ID = r.ID
					if l.URL == "" {
						l.URL = r.URL
					}
					if l.City == "" {
						l.City = r.City
					}
					if l.TransactionType == "" {
						l.TransactionType = r.TransactionType
					}
					err = s.UpsertListing(ctx, l)
				}
			}
			if err == nil {
				err = s.RecordFetchSuccess(ctx, r.Source, r.ID, l.Status, c.StatusRefresh.Interval)
			}
			if err == nil && l.Price > 0 {
				k, pe := s.CurrentPriceEvent(ctx, r.Source, r.ID, l.Price)
				if pe != nil {
					return pe
				}
				if k != "" {
					if pe = s.SavePriceDrop(ctx, r.Source, r.ID, k); pe != nil {
						return pe
					}
					priceDrops[key] = k
				}
			}
			if err == nil {
				err = s.MarkEnrichedLastmod(ctx, r.Source, r.ID, r.LastMod)
			}
			if err != nil {
				var he *client.HTTPStatusError
				code := 0
				if errors.As(err, &he) {
					code = he.Code
				}
				retry := c.StatusRefresh.Interval("unknown")
				if code == http.StatusNotFound {
					retry = c.StatusRefresh.Interval("suspected_removed")
				}
				if code == http.StatusGone {
					retry = c.StatusRefresh.Interval("removed")
				}
				if code == http.StatusTooManyRequests {
					retry = 2 * time.Hour
				}
				if recordErr := s.RecordFetchFailure(ctx, r.Source, r.ID, code, retry); recordErr != nil {
					return recordErr
				}
				fmt.Fprintf(os.Stderr, "enrich %s/%s: %v\n", r.Source, r.ID, err)
			}
		}
	}
	for _, w := range c.Watches {
		initialized := initializedAtStart[w.Name]
		// Query all matching listings; use fetched_at to exclude stale records.
		listings, err := s.Search(ctx, store.SearchQuery{Source: w.Source, City: w.City, Category: w.Category, MaxPrice: w.MaxPrice, MinArea: w.MinArea, Fetched: true, Limit: 100000})
		if err != nil {
			return err
		}
		matches := 0
		for _, l := range listings {
			generation, active, err := s.ActiveGeneration(ctx, l.Source, l.ID)
			if err != nil {
				return err
			}
			if !active {
				continue
			}
			if l.Price <= 0 || l.LivingArea <= 0 && w.MinArea > 0 || l.Bedrooms < w.MinBedrooms {
				continue
			}
			// Suppress only references known before the first scan. New
			// references discovered during that scan remain eligible.
			baseline, err := s.WasBaselineRef(ctx, w.Name, l.Source, l.ID)
			if err != nil {
				return err
			}
			if !initialized && !c.NotifyExisting && baseline {
				continue
			}
			fetched, err := time.Parse(time.RFC3339, l.FetchedAt)
			if err != nil || fetched.Before(cutoff) {
				continue
			}
			matches++
			if noNotify {
				continue
			}
			events := [][2]string{{"new_match", strconv.Itoa(generation)}}
			if generation > 0 {
				events[0][0] = "reopened"
			}
			drop := priceDrops[l.Source+"|"+l.ID]
			if drop == "" {
				drop, err = s.LatestPriceDrop(ctx, l.Source, l.ID)
				if err != nil {
					return err
				}
			}
			if drop != "" {
				events = append(events, [2]string{"price_drop", drop})
			}
			for _, event := range events {
				eventType, eventKey := event[0], event[1]
				for _, name := range w.Notify {
					if err := s.QueueJob(ctx, w.Name, l.Source, l.ID, name, generation, eventType, eventKey); err != nil {
						return err
					}
					claimed, err := s.ClaimJob(ctx, w.Name, l.Source, l.ID, name, generation, eventType, eventKey)
					if err != nil {
						return err
					}
					if !claimed {
						continue
					}
					message := fmt.Sprintf("🏠 %s (%s): %s\n%s\n€%.0f · %.0f m² · %d bedrooms\n%s", w.Name, eventType, l.Address, l.City, l.Price, l.LivingArea, l.Bedrooms, l.URL)
					sendErr := monitor.Send(ctx, c.Providers[name], message)
					if err := s.CompleteJob(ctx, w.Name, l.Source, l.ID, name, generation, eventType, eventKey, sendErr); err != nil {
						return err
					}
					if sendErr != nil {
						fmt.Fprintf(os.Stderr, "notify %s/%s: %v\n", w.Name, name, sendErr)
					}
				}
			}
		}
		if !initialized {
			if err := s.InitializeWatch(ctx, w.Name); err != nil {
				return err
			}
		}
		fmt.Printf("watch %s: %d fresh matches\n", w.Name, matches)
	}
	return nil
}

func runWatchList(args []string) error {
	fs := flag.NewFlagSet("watch list", flag.ContinueOnError)
	path := fs.String("config", "monitor.json", "monitor config path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	c, err := monitor.Load(*path)
	if err != nil {
		return err
	}
	fmt.Printf("Monitoring preferences (%s)\n", *path)
	fmt.Printf("Interval: %s | Refresh after: %s | Notify existing: %t\n", c.Interval, c.RefreshAfter, c.NotifyExisting)
	if len(c.Watches) == 0 {
		fmt.Println("No watch rules configured")
	}
	for _, w := range c.Watches {
		fmt.Printf("\n%s\n  Source: %s | Category: %s | City: %s\n", w.Name, w.Source, w.Category, w.City)
		fmt.Printf("  Max price: %.0f | Min area: %.0f m² | Min bedrooms: %d\n", w.MaxPrice, w.MinArea, w.MinBedrooms)
		fmt.Printf("  Notify: %s\n", strings.Join(w.Notify, ", "))
	}
	fmt.Println("\nNotification destinations:")
	names := make([]string, 0, len(c.Providers))
	for name := range c.Providers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		fmt.Printf("  %s (%s)\n", name, c.Providers[name].Type)
	}
	return nil
}
