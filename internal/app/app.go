package app

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/sc0vu/funda-cli/internal/enrich"
	"github.com/sc0vu/funda-cli/internal/model"
	"github.com/sc0vu/funda-cli/internal/mvgm"
	"github.com/sc0vu/funda-cli/internal/sitemap"
	"github.com/sc0vu/funda-cli/internal/store"
	"github.com/sc0vu/funda-cli/internal/termmap"
)

const defaultIndex = "https://www.funda.nl/sitemap_index.xml"

type App struct {
	DBPath string
}

func (a App) Run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		usage()
		return nil
	}

	s, err := store.Open(a.DBPath)
	if err != nil {
		return err
	}
	defer s.Close()

	switch args[0] {
	case "sync":
		return runSync(ctx, s, args[1:])
	case "enrich":
		return runEnrich(ctx, s, args[1:])
	case "search":
		return runSearch(ctx, s, args[1:])
	case "view":
		return runView(ctx, s, args[1:])
	case "map":
		return runMap(ctx, s, args[1:])
	case "fetch":
		return runFetch(ctx, s, args[1:])
	case "help", "-h", "--help":
		usage()
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runSync(ctx context.Context, s *store.Store, args []string) error {
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	source := fs.String("source", "funda", "funda or mvgm")
	city := fs.String("city", "utrecht", "city for source-specific sync, e.g. utrecht")
	category := fs.String("category", "rent", "rent, buy, newbuild, or all")
	url := fs.String("sitemap", defaultIndex, "Funda sitemap or sitemap-index URL")
	profile := fs.String("profile", "chrome", "Chrome, ChromeAndroid, Firefox, Safari, Edge, IOS")
	timeout := fs.Duration("timeout", 30*time.Second, "HTTP request timeout")
	if err := fs.Parse(args); err != nil {
		return err
	}

	var refs []model.ListingRef
	var err error
	switch strings.ToLower(*source) {
	case "funda":
		refs, err = sitemap.Fetch(ctx, *profile, *timeout, *url, normalizeCategory(*category))
	case "mvgm":
		if normalizeCategory(*category) != "rent" && normalizeCategory(*category) != "all" {
			return fmt.Errorf("MVGM only supports rent listings")
		}
		refs, err = mvgm.Sync(ctx, *profile, *timeout, *city)
	default:
		return fmt.Errorf("unknown source %q (use funda or mvgm)", *source)
	}
	if err != nil {
		return err
	}
	if err := s.UpsertRefs(ctx, refs); err != nil {
		return err
	}
	fmt.Printf("synced %d %s listing references\n", len(refs), strings.ToLower(*source))
	fmt.Printf("use enrich then search to view the data\n")
	return nil
}

func runEnrich(ctx context.Context, s *store.Store, args []string) error {
	fs := flag.NewFlagSet("enrich", flag.ContinueOnError)
	source := fs.String("source", "funda", "funda or mvgm")
	city := fs.String("city", "", "city slug/name, e.g. utrecht")
	category := fs.String("category", "rent", "rent, buy, newbuild, or all")
	limit := fs.Int("limit", 25, "maximum listing details to fetch")
	profile := fs.String("profile", "chrome", "Chrome, ChromeAndroid, Firefox, Safari, Edge, IOS")
	timeout := fs.Duration("timeout", 30*time.Second, "HTTP request timeout")
	if err := fs.Parse(args); err != nil {
		return err
	}
	refs, err := s.MissingDetails(ctx, strings.ToLower(*source), *city, normalizeCategory(*category), *limit)
	if err != nil {
		return err
	}
	ok := 0
	for _, r := range refs {
		if strings.ToLower(*source) == "mvgm" && r.Source != "mvgm" {
			continue
		}
		if strings.ToLower(*source) == "funda" && r.Source != "funda" {
			continue
		}

		switch strings.ToLower(*source) {
		case "funda":
			l, err := enrich.Fetch(ctx, *profile, *timeout, r.ID)
			if err != nil {
				fmt.Fprintf(os.Stderr, "[%s] %v\n", r.ID, err)
				continue
			}
			l.Source = "funda"
			l.ID = sitemap.ListingID(r.URL)
			if l.ID == "" {
				l.ID = r.ID
			}
			if l.URL == "" {
				l.URL = r.URL
			}
			if l.City == "" {
				l.City = r.City
			}
			if l.TransactionType == "" {
				l.TransactionType = r.TransactionType
			}
			if err := s.UpsertListing(ctx, l); err != nil {
				fmt.Fprintf(os.Stderr, "[%s] store: %v\n", r.ID, err)
				continue
			}
		case "mvgm":
			l, d, err := mvgm.Fetch(ctx, *profile, *timeout, r)
			if err != nil {
				fmt.Fprintf(os.Stderr, "[%s] %v\n", r.ID, err)
				continue
			}
			if err := s.UpsertListing(ctx, l); err != nil {
				fmt.Fprintf(os.Stderr, "[%s] listing store: %v\n", r.ID, err)
				continue
			}
			if err := s.UpsertRentalDetails(ctx, d); err != nil {
				fmt.Fprintf(os.Stderr, "[%s] rental detail store: %v\n", r.ID, err)
				continue
			}
		default:
			return fmt.Errorf("unknown source %q (use funda or mvgm)", *source)
		}
		ok++
		fmt.Printf("enriched %s %s\n", r.ID, r.City)
	}
	fmt.Printf("enriched %d listing(s) from %s\n", ok, strings.ToLower(*source))
	return nil
}

func runSearch(ctx context.Context, s *store.Store, args []string) error {
	fs := flag.NewFlagSet("search", flag.ContinueOnError)
	source := fs.String("source", "all", "funda, mvgm, or all")
	income := fs.Float64("income", 0, "gross annual household income for MVGM eligibility")
	savings := fs.Float64("savings", 0, "own assets; MVGM allows 10% to be added to gross annual income")
	city := fs.String("city", "", "city, e.g. utrecht")
	category := fs.String("category", "all", "rent, buy, newbuild, or all")
	maxPrice := fs.Float64("max-price", 0, "maximum price (requires fetched details)")
	minArea := fs.Float64("min-area", 0, "minimum living area in m² (requires fetched details)")
	limit := fs.Int("limit", 50, "maximum results")
	fetched := fs.Bool("fetched", false, "only show references with fetched detail data")
	unfetched := fs.Bool("unfetched", false, "only show references without fetched detail data")
	detailsOnly := fs.Bool("details-only", false, "alias for --fetched")
	showURL := fs.Bool("url", false, "show the full listing URL")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *fetched && *unfetched {
		return fmt.Errorf("--fetched and --unfetched cannot be used together")
	}
	if *detailsOnly && *unfetched {
		return fmt.Errorf("--details-only and --unfetched cannot be used together")
	}

	items, err := s.Search(ctx, store.SearchQuery{
		Source:    strings.ToLower(*source),
		City:      *city,
		Category:  normalizeCategory(*category),
		MaxPrice:  *maxPrice,
		MinArea:   *minArea,
		Limit:     *limit,
		Fetched:   *fetched || *detailsOnly,
		Unfetched: *unfetched,
	})
	if err != nil {
		return err
	}
	if len(items) == 0 {
		fmt.Println("no listings found")
		return nil
	}

	showEligibility := *income > 0 || *savings > 0
	if showEligibility {
		fmt.Printf("%-7s %-24s %-16s %-9s %-7s %-7s %-12s %-11s\n",
			"SOURCE", "ID", "CITY", "PRICE", "AREA", "BED", "REQ.INCOME", "ELIGIBILITY")
	} else {
		fmt.Printf("%-7s %-24s %-8s %-16s %-10s %-8s %-5s %-8s %-10s\n",
			"SOURCE", "ID", "TYPE", "CITY", "PRICE", "AREA", "BED", "ENERGY", "DETAIL")
	}
	qualifyingIncome := *income + (*savings * 0.10)
	for _, l := range items {
		price := "-"
		area := "-"
		bedrooms := "-"
		energy := "-"
		detail := "-"
		if l.HasDetails {
			detail = "yes"
			if l.Price > 0 {
				price = fmt.Sprintf("€%.0f", l.Price)
			}
			if l.LivingArea > 0 {
				area = fmt.Sprintf("%.0fm²", l.LivingArea)
			}
			if l.Bedrooms > 0 {
				bedrooms = fmt.Sprintf("%d", l.Bedrooms)
			}
			if l.EnergyLabel != "" {
				energy = l.EnergyLabel
			}
		}

		if showEligibility {
			required := "-"
			eligibility := "n/a"
			if l.Source == "mvgm" && l.HasDetails {
				if d, err := s.RentalDetails(ctx, l.Source, l.ID); err == nil && d.RequiredIncome > 0 {
					required = fmt.Sprintf("€%.0f", d.RequiredIncome)
					ratio := qualifyingIncome / d.RequiredIncome
					switch {
					case ratio >= 1.0:
						eligibility = "eligible*"
					case ratio >= 0.95:
						eligibility = "borderline"
					default:
						eligibility = "below"
					}
				}
			}
			fmt.Printf("%-7s %-24s %-16s %-9s %-7s %-7s %-12s %-11s\n",
				l.Source, truncate(l.ID, 24), truncate(l.City, 16), price, area, bedrooms, required, eligibility)
		} else {
			fmt.Printf("%-7s %-24s %-8s %-16s %-10s %-8s %-5s %-8s %-10s\n",
				l.Source, truncate(l.ID, 24), l.TransactionType, truncate(l.City, 16), price, area, bedrooms, energy, detail)
		}
		if *showURL {
			fmt.Printf("  %s\n", l.URL)
		}
	}
	if showEligibility {
		fmt.Printf("qualifying income used: €%.0f (= income €%.0f + 10%% of savings €%.0f)\n", qualifyingIncome, *income, *savings)
		fmt.Println("* eligibility is an estimate; listing-specific requirements and MVGM verification still apply")
	}
	fmt.Printf("%d result(s)\n", len(items))
	return nil
}

func runView(ctx context.Context, s *store.Store, args []string) error {
	fs := flag.NewFlagSet("view", flag.ContinueOnError)
	source := fs.String("source", "all", "funda, mvgm, or all")
	showMap := fs.Bool("map", false, "show an OpenStreetMap terminal map")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: funda view [--source funda|mvgm] [--map] <listing-id>")
	}
	id := fs.Arg(0)
	items, err := s.Search(ctx, store.SearchQuery{ID: id, Source: strings.ToLower(*source), Limit: 2})
	if err != nil {
		return err
	}
	var matches []model.Listing
	for _, l := range items {
		if l.ID == id {
			matches = append(matches, l)
		}
	}
	if len(matches) == 0 {
		return fmt.Errorf("listing %s not found", id)
	}
	if len(matches) > 1 {
		var sources []string
		for _, l := range matches {
			sources = append(sources, l.Source)
		}
		return fmt.Errorf("listing ID %s exists in multiple sources (%s); use --source", id, strings.Join(sources, ", "))
	}
	l := matches[0]
	fmt.Printf("Source: %s\nID: %s\nAddress: %s\nCity: %s\nType: %s\nPrice: %.0f\nArea: %.0f m²\nBedrooms: %d\nRooms: %d\nEnergy: %s\nStatus: %s\nURL: %s\nFetched: %s\n",
		l.Source, l.ID, l.Address, l.City, l.TransactionType, l.Price, l.LivingArea,
		l.Bedrooms, l.Rooms, l.EnergyLabel, l.Status, l.URL, l.FetchedAt)
	if l.Source == "mvgm" {
		if d, err := s.RentalDetails(ctx, l.Source, l.ID); err == nil {
			fmt.Printf("Service cost: €%.0f\nDeposit: €%.0f\nAvailable: %s\nIncome rule: %s\nRequired annual income: €%.0f\n", d.ServiceCost, d.Deposit, d.AvailableFrom, d.IncomeRule, d.RequiredIncome)
		}
	}
	if *showMap {
		fmt.Println("\nMap (OpenStreetMap):")
		m, err := termmap.Render(ctx, termmap.Options{Lat: l.Latitude, Lon: l.Longitude, Width: 90, Height: 26, RadiusMeters: 450, Color: true})
		if err != nil {
			return err
		}
		fmt.Print(m)
	}
	return nil
}

func runMap(ctx context.Context, s *store.Store, args []string) error {
	fs := flag.NewFlagSet("map", flag.ContinueOnError)
	source := fs.String("source", "all", "funda, mvgm, or all")
	width := fs.Int("width", 90, "map width in terminal columns")
	height := fs.Int("height", 26, "map height in terminal rows")
	radius := fs.Float64("radius", 450, "map radius around the property in meters")
	buildings := fs.Bool("buildings", false, "include building outlines (denser and slower)")
	noColor := fs.Bool("no-color", false, "disable ANSI map colors")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: funda map [--source funda|mvgm] [--width 90] [--height 26] [--radius 450] [--buildings] [--no-color] <listing-id>")
	}
	id := fs.Arg(0)
	items, err := s.Search(ctx, store.SearchQuery{ID: id, Source: strings.ToLower(*source), Limit: 2})
	if err != nil {
		return err
	}
	var matches []model.Listing
	for _, l := range items {
		if l.ID == id && l.HasDetails {
			matches = append(matches, l)
		}
	}
	if len(matches) == 0 {
		return fmt.Errorf("listing %s has no fetched details/coordinates; run funda fetch first", id)
	}
	if len(matches) > 1 {
		return fmt.Errorf("listing ID %s exists in multiple sources; use --source", id)
	}
	l := matches[0]
	fmt.Printf("%s, %s  (%.5f, %.5f)\n", l.Address, l.City, l.Latitude, l.Longitude)
	fmt.Println("OpenStreetMap:")
	m, err := termmap.Render(ctx, termmap.Options{Lat: l.Latitude, Lon: l.Longitude, Width: *width, Height: *height, RadiusMeters: *radius, Buildings: *buildings, Color: !*noColor})
	if err != nil {
		return err
	}
	fmt.Print(m)

	return nil
}

func runFetch(ctx context.Context, s *store.Store, args []string) error {
	fs := flag.NewFlagSet("fetch", flag.ContinueOnError)
	source := fs.String("source", "all", "funda, mvgm, or all")
	profile := fs.String("profile", "chrome", "Chrome, ChromeAndroid, Firefox, Safari, Edge, IOS")
	timeout := fs.Duration("timeout", 30*time.Second, "HTTP request timeout")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: funda fetch [--source funda|mvgm] <listing-id>")
	}
	id := fs.Arg(0)
	r, err := s.GetRef(ctx, strings.ToLower(*source), id)
	if err != nil {
		return err
	}
	switch strings.ToLower(*source) {
	case "funda":
		l, err := enrich.Fetch(ctx, *profile, *timeout, r.ID)
		if err != nil {
			return err
		}
		l.Source = "funda"
		l.ID = sitemap.ListingID(r.URL)
		if l.ID == "" {
			l.ID = r.ID
		}
		if l.URL == "" {
			l.URL = r.URL
		}
		if l.City == "" {
			l.City = r.City
		}
		if l.TransactionType == "" {
			l.TransactionType = r.TransactionType
		}
		if err := s.UpsertListing(ctx, l); err != nil {
			return err
		}
	case "mvgm":
		l, d, err := mvgm.Fetch(ctx, *profile, *timeout, r)
		if err != nil {
			return err
		}
		if err := s.UpsertListing(ctx, l); err != nil {
			return err
		}
		if err := s.UpsertRentalDetails(ctx, d); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown source %q (use funda or mvgm)", *source)
	}
	fmt.Printf("fetched %s from %s\n", id, strings.ToLower(*source))
	return nil
}

func normalizeCategory(v string) string {
	switch strings.ToLower(v) {
	case "huur", "rent":
		return "rent"
	case "koop", "buy":
		return "buy"
	case "nieuwbouw", "newbuild":
		return "newbuild"
	case "all":
		return "all"
	default:
		return strings.ToLower(v)
	}
}

func truncate(v string, max int) string {
	if len(v) <= max {
		return v
	}
	if max <= 1 {
		return v[:max]
	}
	return v[:max-1] + "…"
}

func getenv(k, fallback string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return fallback
}

func usage() {
	fmt.Print(`funda - lightweight local housing search CLI

Commands:
  funda sync    --source funda --category rent|buy|newbuild|all [--sitemap URL]
  funda sync    --source mvgm --city utrecht
  funda enrich  --source funda --city utrecht --category rent --limit 25
  funda enrich  --source mvgm --city utrecht --limit 25
  funda search  --source all|funda|mvgm  [--city utrecht] [--category rent] [--unfetched|--fetched] [--max-price 2000] [--min-area 50]
  funda view    [--source funda|mvgm] [--map] <listing-id>
  funda map     [--source funda|mvgm] [--width 90] [--height 26] [--radius 450] [--buildings] [--no-color] <listing-id>
  funda fetch    [--source funda|mvgm] <listing-id> fetch a single listing for debuging purpose

Environment:
  FUNDA_DB          SQLite path (default: ./funda.db)
`)
}
