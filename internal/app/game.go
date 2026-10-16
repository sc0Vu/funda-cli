package app

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"github.com/mattn/go-isatty"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/sc0vu/funda-cli/internal/game"
	"github.com/sc0vu/funda-cli/internal/gameimage"
	"github.com/sc0vu/funda-cli/internal/store"
)

func runGame(ctx context.Context, s *store.Store, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: funda game guess|hunter [--mode buy|rent] [--city utrecht] [--seed N]")
	}
	name := args[0]
	if name != "guess" && name != "hunter" {
		return fmt.Errorf("unknown game %q (guess or hunter)", name)
	}
	fs := flag.NewFlagSet("game "+name, flag.ContinueOnError)
	mode := fs.String("mode", "buy", "buy or rent")
	city := fs.String("city", "", "city filter")
	source := fs.String("source", "all", "funda, mvgm or all")
	rounds := fs.Int("rounds", 5, "maximum rounds")
	seed := fs.Int64("seed", time.Now().UnixNano(), "repeatable game seed")
	budget := fs.Int64("budget", 375000, "maximum buying budget in euros")
	income := fs.Int64("income", 0, "gross monthly household income for rental simulation")
	daily := fs.Bool("daily", false, "use deterministic UTC daily challenge seed (dataset must match)")
	historyFile := fs.String("history-file", "", "override default JSON game history path")
	noSave := fs.Bool("no-save", false, "do not save completed game results or Hunter progress")
	saveFile := fs.String("save-file", "", "override House Hunter progress file")
	resume := fs.Bool("resume", false, "resume a saved House Hunter game")
	restart := fs.Bool("restart", false, "discard saved House Hunter progress and start fresh")
	imageMode := fs.String("image-mode", "auto", "auto|kitty|iterm2|link|none")
	photosFile := fs.String("photos-file", "", "optional JSON mapping source:listingID to permitted photo URLs")
	detective := fs.Bool("detective", false, "enable optional price-guessing hints (-100 points per hint)")
	household := fs.Int("household", 1, "household size for rental simulation")
	documents := fs.Bool("documents", true, "rental application documents complete")
	financing := fs.Bool("financing-certain", false, "simulated stronger financing certainty for buying")
	flexible := fs.Bool("flexible-date", false, "simulated flexible transfer date for buying")
	multiplier := fs.Float64("income-multiplier", 0, "fictional rental income requirement, e.g. 3; required for rental applications")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if *resume && *restart {
		return fmt.Errorf("--resume and --restart cannot be combined")
	}
	if *resume && name != "hunter" {
		return fmt.Errorf("--resume is only supported for hunter")
	}
	if *resume && *noSave {
		return fmt.Errorf("--resume cannot be used with --no-save")
	}
	parsedImageMode, imageErr := gameimage.Parse(*imageMode)
	if imageErr != nil {
		return imageErr
	}
	resolvedImageMode := gameimage.Resolve(parsedImageMode, gameimage.Environment(), isatty.IsTerminal(os.Stdout.Fd()))
	gallery, err := gameimage.LoadGallery(*photosFile)
	if err != nil {
		return fmt.Errorf("load photo gallery: %w", err)
	}
	if *daily {
		*seed = game.DailySeed(time.Now().UTC(), strings.ToLower(*city), name, *mode)
	}
	if *mode != "buy" && *mode != "rent" {
		return fmt.Errorf("--mode must be buy or rent")
	}
	if *rounds < 1 || *rounds > 100 {
		return fmt.Errorf("--rounds must be 1..100")
	}
	if *budget < 1 || *income < 0 || *multiplier < 0 || *household < 1 {
		return fmt.Errorf("budget, income, or multiplier invalid")
	}
	category := "buy"
	if *mode == "rent" {
		category = "rent"
	}
	ls, err := s.Search(ctx, store.SearchQuery{Source: *source, City: *city, Category: category, Fetched: true, Limit: 10000})
	if err != nil {
		return err
	}
	properties := game.Shuffle(game.FromListings(ls, *mode), *seed)
	if len(properties) == 0 {
		return game.ErrNoListings
	}
	n := *rounds
	if n > len(properties) {
		n = len(properties)
	}
	// Resume pins the exact property sequence; refuse mismatched caches rather than
	// silently resuming against a different listing or changed ordering.
	savePath := *saveFile
	if name == "hunter" && !*noSave && savePath == "" {
		savePath, err = game.DefaultHunterSavePath()
		if err != nil {
			return err
		}
	}
	var progress game.HunterSave
	startIndex := 0
	if name == "hunter" && !*noSave {
		if *restart {
			if err := os.Remove(savePath); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
		if *resume {
			progress, err = game.LoadHunter(savePath)
			if err != nil {
				return fmt.Errorf("resume hunter: %w", err)
			}
			if progress.Mode != *mode || progress.City != *city || progress.Source != *source {
				return fmt.Errorf("saved mode/city/source differ; supply the saved options")
			}
			*seed = progress.Seed
			properties = game.Shuffle(game.FromListings(ls, *mode), *seed)
			if len(properties) < progress.Rounds {
				return fmt.Errorf("cached listings have changed; cannot safely resume")
			}
			for j, id := range progress.PropertyIDs {
				if properties[j].Source+":"+properties[j].ID != id {
					return fmt.Errorf("cached property set has changed; cannot safely resume")
				}
				if len(progress.Prices) > 0 && properties[j].Price != progress.Prices[j] {
					return fmt.Errorf("cached asking prices have changed; restart the game to use new data")
				}
			}
			n = progress.Rounds
			startIndex = progress.Index
		} else {
			if _, e := os.Stat(savePath); e == nil {
				return fmt.Errorf("unfinished Hunter save exists at %s; use --resume or --restart", savePath)
			} else if !os.IsNotExist(e) {
				return e
			}
			ids := make([]string, n)
			prices := make([]int64, n)
			for j, p := range properties[:n] {
				ids[j] = p.Source + ":" + p.ID
				prices[j] = p.Price
			}
			progress = game.HunterSave{Version: 1, Mode: *mode, City: *city, Source: *source, Seed: *seed, Rounds: n, PropertyIDs: ids, Prices: prices}
			if err := game.SaveHunter(savePath, progress); err != nil {
				return err
			}
		}
	}
	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("\nFUNDA %s (%s) | %d rounds | seed %d\n", strings.ToUpper(name), *mode, n, *seed)
	fmt.Println("Offline simulation using cached listings. No real offers or applications are sent.")
	fmt.Printf("Image mode: %s (optional photo manifest; links only)\n", resolvedImageMode)
	if *daily {
		fmt.Println("Daily challenge: scores are only comparable with identical cached listing datasets.")
	}
	total := progress.Score
	const maxTurns = 20
	turns := progress.Turns
	persist := func(index int, viewed, inspected bool) error {
		if name != "hunter" || *noSave {
			return nil
		}
		progress.Index = index
		progress.Turns = turns
		progress.Score = total
		progress.Viewed = viewed
		progress.Inspected = inspected
		return game.SaveHunter(savePath, progress)
	}
	for i := startIndex; i < n; i++ {
		p := properties[i]
		fmt.Printf("\n--- Round %d/%d ---\n", i+1, n)
		v := p.Public()
		fmt.Printf("%s | %.0f m² | %d rooms | energy %s\n", v.City, v.Area, v.Rooms, v.Energy)
		if name == "guess" {
			fmt.Println("Photo links are hidden until your guess to avoid revealing the listing.")
		}
		if name == "guess" {
			// Price, address, source URL and listing ID are deliberately withheld until submission.
			hints := 0
			if *detective {
				fmt.Print("[h] Show estimated price range (-100 points) [g] Guess [q] Quit: ")
				selection, readErr := reader.ReadString('\n')
				if readErr != nil {
					if readErr == io.EOF {
						return nil
					}
					return readErr
				}
				switch strings.ToLower(strings.TrimSpace(selection)) {
				case "q":
					return nil
				case "h":
					lo, hi := game.PriceBand(p, *seed+int64(i)*7919)
					fmt.Printf("Fictional estimated range: €%d–€%d (not an actual valuation)\n", lo, hi)
					hints++
				}
			}
			guess, err := askInt(reader, "Your guess (€), or q to quit: ")
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return err
			}
			score := game.HintScore(p.Price, guess, hints)
			total += score
			fmt.Printf("Actual asking %s: €%d | Your guess: €%d | Score: %d/1000\n", map[bool]string{true: "monthly rent", false: "price"}[*mode == "rent"], p.Price, guess, score)
			fmt.Printf("Property: %s | %s\n", p.Address, p.URL)
			if ok, err := gallery.Show(os.Stdout, p.Source, p.ID); err != nil {
				return err
			} else if !ok {
				fmt.Println("No gallery available. Open the original listing for photos.")
			}
			continue
		}
		fmt.Printf("Asking %s: €%d | %s\n", map[bool]string{true: "monthly rent", false: "price"}[*mode == "rent"], p.Price, p.Address)
		viewed := i == startIndex && progress.Viewed
		inspected := i == startIndex && progress.Inspected
		for {
			fmt.Printf("Turn %d/%d | ", turns, maxTurns)
			fmt.Print("[v] View [i] Fictional inspection [e] Details [o] Open listing [a] Apply/Bid [s] Skip [q] Quit: ")
			action, err := reader.ReadString('\n')
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return err
			}
			switch strings.ToLower(strings.TrimSpace(action)) {
			case "v":
				if !viewed {
					if turns >= maxTurns {
						fmt.Println("No turns remaining.")
						continue
					}
					turns++
					viewed = true
					if err := persist(i, viewed, inspected); err != nil {
						return err
					}
				}
				fmt.Printf("Viewing: %.0f m² | %d rooms | energy %s | asking €%d\n", p.Area, p.Rooms, p.Energy, p.Price)
				if ok, err := gallery.Show(os.Stdout, p.Source, p.ID); err != nil {
					return err
				} else if !ok {
					fmt.Println("No gallery available. Press [o] to open the original listing.")
				}
			case "e":
				fmt.Printf("Cached details: %.0f m² | %d rooms | energy %s | asking €%d\n", p.Area, p.Rooms, p.Energy, p.Price)
			case "i":
				if !viewed {
					fmt.Println("Schedule a viewing [v] first.")
					continue
				}
				if !inspected {
					if turns >= maxTurns {
						fmt.Println("No turns remaining.")
						continue
					}
					turns++
					inspected = true
					if err := persist(i, viewed, inspected); err != nil {
						return err
					}
				}
				report := game.Inspect(p, *seed+int64(i)*7919)
				fmt.Println("Fictional inspection (not real listing information):", report.Summary())
			case "o":
				if err := openListing(p.URL); err != nil {
					fmt.Printf("Open manually: %s (%v)\n", p.URL, err)
				}
			case "s":
				if turns >= maxTurns {
					fmt.Println("No turns remaining.")
					continue
				}
				turns++
				if i+1 < n {
					if err := persist(i+1, false, false); err != nil {
						return err
					}
				}
				goto next
			case "q":
				return nil
			case "a":
				if turns >= maxTurns {
					fmt.Println("No turns remaining.")
					continue
				}
				if *mode == "buy" {
					amount, err := askInt(reader, "Your simulated bid (€): ")
					if err != nil {
						return err
					}
					if amount > *budget {
						fmt.Println("Bid exceeds your budget.")
						continue
					}
					turns++
					res := game.ResolveBuy(p, game.Offer{Amount: amount, FinancingCertain: *financing, FlexibleDate: *flexible}, *seed+int64(i)*7919)
					fmt.Printf("%s Rival bid: €%d. %s\n", map[bool]string{true: "Won!", false: "Lost."}[res.Won], res.RivalAmount, res.Explanation)
					if res.Won {
						total += 1000
					}
				} else {
					turns++
					res := game.ResolveRent(p, game.RentalApplication{MonthlyGrossIncome: *income, DocumentsComplete: *documents, HouseholdSize: *household, RequiredMultiplier: *multiplier}, *seed+int64(i)*7919)
					fmt.Println(res.Explanation)
					if res.Won {
						total += 1000
					}
				}
				if i+1 < n {
					if err := persist(i+1, false, false); err != nil {
						return err
					}
				}
				goto next
			default:
				fmt.Println("Unknown action")
			}
		}
	next:
		if name == "hunter" && turns >= maxTurns {
			fmt.Println("Search deadline reached.")
			break
		}
	}
	fmt.Printf("\nGame complete! Score: %d\n", total)
	if !*noSave {
		path := *historyFile
		if path == "" {
			var err error
			path, err = game.DefaultHistoryPath()
			if err != nil {
				return err
			}
		}
		if err := game.AppendResult(path, game.SessionResult{Game: name, Mode: *mode, City: *city, Seed: *seed, Rounds: n, Score: total, CompletedAt: time.Now().UTC()}); err != nil {
			return fmt.Errorf("save game history: %w", err)
		}
		fmt.Printf("History saved: %s\n", path)
		if name == "hunter" {
			if err := os.Remove(savePath); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
	}
	return nil
}
func askInt(r *bufio.Reader, prompt string) (int64, error) {
	for {
		fmt.Print(prompt)
		line, err := r.ReadString('\n')
		if err != nil {
			return 0, err
		}
		line = strings.TrimSpace(line)
		if strings.EqualFold(line, "q") {
			return 0, io.EOF
		}
		n, e := strconv.ParseInt(line, 10, 64)
		if e == nil && n >= 0 {
			return n, nil
		}
		fmt.Println("Enter a non-negative whole number.")
	}
}
func openListing(url string) error {
	if !(strings.HasPrefix(url, "https://www.funda.nl/") || strings.HasPrefix(url, "https://ikwilhuren.nu/")) {
		return fmt.Errorf("untrusted or missing listing URL")
	}
	var name string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		name = "open"
		args = []string{url}
	case "windows":
		name = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", url}
	default:
		name = "xdg-open"
		args = []string{url}
	}
	return exec.Command(name, args...).Start()
}
