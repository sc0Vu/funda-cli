package app

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/sc0vu/funda-cli/internal/enrich"
	"github.com/sc0vu/funda-cli/internal/sitemap"
	"github.com/sc0vu/funda-cli/internal/store"
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
	case "help", "-h", "--help":
		usage()
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runSync(ctx context.Context, s *store.Store, args []string) error {
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	category := fs.String("category", "rent", "rent, buy, newbuild, or all")
	url := fs.String("sitemap", defaultIndex, "sitemap or sitemap-index URL")
	profile := fs.String("profile", "chrome", "Chrome, ChromeAndroid, Firefox, Safari, Edge, IOS")
	timeout := fs.Duration("timeout", 30*time.Second, "HTTP request timeout")
	if err := fs.Parse(args); err != nil {
		return err
	}
	refs, err := sitemap.Fetch(ctx, *profile, *timeout, *url, normalizeCategory(*category))
	if err != nil {
		return err
	}
	if err := s.UpsertRefs(ctx, refs); err != nil {
		return err
	}
	fmt.Printf("synced %d listing references\n", len(refs))
	fmt.Printf("use search command to view the data\n")
	return nil
}

func runEnrich(ctx context.Context, s *store.Store, args []string) error {
	fs := flag.NewFlagSet("enrich", flag.ContinueOnError)
	city := fs.String("city", "", "city slug/name, e.g. utrecht")
	category := fs.String("category", "rent", "rent, buy, newbuild, or all")
	limit := fs.Int("limit", 25, "maximum listing details to fetch")
	profile := fs.String("profile", "chrome", "Chrome, ChromeAndroid, Firefox, Safari, Edge, IOS")
	timeout := fs.Duration("timeout", 30*time.Second, "HTTP request timeout")
	if err := fs.Parse(args); err != nil {
		return err
	}
	refs, err := s.MissingDetails(ctx, *city, normalizeCategory(*category), *limit)
	if err != nil {
		return err
	}
	ok := 0
	for _, r := range refs {
		l, err := enrich.Fetch(ctx, *profile, *timeout, r.ID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[%s] %v\n", r.ID, err)
			continue
		}
		// r.ID mistmatch the id extracted from URL
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
		ok++
		fmt.Printf("enriched %s %s\n", r.ID, r.City)
	}
	fmt.Printf("enriched %d/%d listings\n", ok, len(refs))
	return nil
}

func runSearch(ctx context.Context, s *store.Store, args []string) error {
	fs := flag.NewFlagSet("search", flag.ContinueOnError)
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

	fmt.Printf("%-12s %-8s %-16s %-10s %-8s %-5s %-8s %-10s\n",
		"ID", "TYPE", "CITY", "PRICE", "AREA", "BED", "ENERGY", "DETAIL")
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

		fmt.Printf("%-12s %-8s %-16s %-10s %-8s %-5s %-8s %-10s\n",
			l.ID, l.TransactionType, truncate(l.City, 16), price, area, bedrooms, energy, detail)
		if *showURL {
			fmt.Printf("  %s\n", l.URL)
		}
	}
	fmt.Printf("%d result(s)\n", len(items))
	return nil
}

func runView(ctx context.Context, s *store.Store, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: funda view <listing-id>")
	}
	items, err := s.Search(ctx, store.SearchQuery{Limit: 10000})
	if err != nil {
		return err
	}
	for _, l := range items {
		if l.ID == args[0] {
			fmt.Printf("ID: %s\nAddress: %s\nCity: %s\nType: %s\nPrice: %.0f\nArea: %.0f m²\nBedrooms: %d\nRooms: %d\nEnergy: %s\nStatus: %s\nURL: %s\nFetched: %s\n",
				l.ID, l.Address, l.City, l.TransactionType, l.Price, l.LivingArea,
				l.Bedrooms, l.Rooms, l.EnergyLabel, l.Status, l.URL, l.FetchedAt)
			return nil
		}
	}
	return fmt.Errorf("listing %s not found", args[0])
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
  funda sync    --category rent|buy|newbuild|all [--sitemap URL]
  funda enrich  --city utrecht --category rent --limit 25
  funda search  [--city utrecht] [--category rent] [--unfetched|--fetched] [--max-price 2000] [--min-area 50]
  funda view    <listing-id>

Environment:
  FUNDA_DB          SQLite path (default: ./funda.db)
`)
}
