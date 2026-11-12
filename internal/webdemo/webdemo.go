// Package webdemo is a pure-Go, offline command interpreter over a built-in
// sample dataset. It is used by the WebAssembly demo and never touches the
// network or the filesystem.
package webdemo

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/sc0vu/funda-cli/internal/game"
	"github.com/sc0vu/funda-cli/internal/model"
)

const (
	searchUsage = "usage: search [--city NAME] [--category rent|buy]"
	showUsage   = "usage: show ID"
	guessUsage  = "usage: guess ID PRICE"
)

var properties = loadProperties()

func loadProperties() []game.Property {
	ls := []model.Listing{
		{ID: "1001", TransactionType: "sale", City: "Utrecht", Address: "Oudegracht 12", Price: 525000, LivingArea: 85, Rooms: 4, EnergyLabel: "B"},
		{ID: "1002", TransactionType: "rental", City: "Utrecht", Address: "Lange Nieuwstraat 5", Price: 1650, LivingArea: 62, Rooms: 3, EnergyLabel: "C"},
		{ID: "1003", TransactionType: "sale", City: "Amsterdam", Address: "Prinsengracht 220", Price: 875000, LivingArea: 110, Rooms: 5, EnergyLabel: "A"},
		{ID: "1004", TransactionType: "rental", City: "Amsterdam", Address: "Jordaanstraat 8", Price: 2400, LivingArea: 70, Rooms: 3, EnergyLabel: "B"},
		{ID: "1005", TransactionType: "sale", City: "Rotterdam", Address: "Witte de Withstraat 41", Price: 395000, LivingArea: 92, Rooms: 4, EnergyLabel: "C"},
		{ID: "1006", TransactionType: "rental", City: "Rotterdam", Address: "Kop van Zuid 3", Price: 1900, LivingArea: 78, Rooms: 3, EnergyLabel: "A"},
		{ID: "1007", TransactionType: "sale", City: "Utrecht", Address: "Maliebaan 77", Price: 640000, LivingArea: 135, Rooms: 6, EnergyLabel: "D"},
		{ID: "1008", TransactionType: "rental", City: "Amsterdam", Address: "Bilderdijkstraat 19", Price: 1250, LivingArea: 38, Rooms: 2, EnergyLabel: "E"},
	}
	for i := range ls {
		ls[i].HasDetails = true
		ls[i].Source = "demo"
	}
	return game.FromListings(ls, "all")
}

func find(id string) (game.Property, bool) {
	for _, p := range properties {
		if p.ID == id {
			return p, true
		}
	}
	return game.Property{}, false
}

// Exec runs one command line and returns plain-text output.
func Exec(line string) string {
	f := strings.Fields(line)
	if len(f) == 0 {
		return ""
	}
	args := f[1:]
	switch strings.ToLower(f[0]) {
	case "help":
		return helpText
	case "about":
		return aboutText
	case "search":
		return search(args)
	case "show":
		return show(args)
	case "guess":
		return guess(args)
	}
	return fmt.Sprintf("unknown command: %s. Type 'help' for available commands.", f[0])
}

const helpText = `Available commands:
  help                                      show this help
  about                                     about this demo
  search [--city NAME] [--category rent|buy]  list sample listings
  show ID                                   show listing details
  guess ID PRICE                            guess the price (euros) and get a score
  clear                                     clear the screen`

const aboutText = "This is an offline demo of funda-cli. All listings are fictional sample data, and it runs entirely in your browser: no network access occurs."

func search(args []string) string {
	var city, cat string
	for i := 0; i < len(args); i += 2 {
		if i+1 >= len(args) {
			return "error: missing value for " + args[i] + "\n" + searchUsage
		}
		switch args[i] {
		case "--city":
			city = args[i+1]
		case "--category":
			cat = strings.ToLower(args[i+1])
			if cat != "rent" && cat != "buy" {
				return fmt.Sprintf("error: invalid category %q\n%s", args[i+1], searchUsage)
			}
		default:
			return "error: unknown flag " + args[i] + "\n" + searchUsage
		}
	}
	var rows []game.Property
	for _, p := range properties {
		if city != "" && !strings.EqualFold(p.City, city) {
			continue
		}
		if cat != "" && p.Kind != cat {
			continue
		}
		rows = append(rows, p)
	}
	if len(rows) == 0 {
		return "No listings found."
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	lines := make([]string, 0, len(rows)+1)
	lines = append(lines, fmt.Sprintf("%-6s %-10s %-5s %-12s %s", "ID", "City", "Kind", "Price", "Area"))
	for _, p := range rows {
		lines = append(lines, fmt.Sprintf("%-6s %-10s %-5s %-12s %.0f m2", p.ID, p.City, p.Kind, euro(p), p.Area))
	}
	return strings.Join(lines, "\n")
}

func euro(p game.Property) string {
	s := "EUR " + strconv.FormatInt(p.Price, 10)
	if p.Kind == "rent" {
		s += "/mo"
	}
	return s
}

func show(args []string) string {
	if len(args) != 1 {
		return showUsage
	}
	p, ok := find(args[0])
	if !ok {
		return fmt.Sprintf("error: listing %q not found", args[0])
	}
	return strings.Join([]string{
		"Listing " + p.ID,
		"Address:      " + p.Address,
		"City:         " + p.City,
		"Kind:         " + p.Kind,
		"Price:        " + euro(p),
		fmt.Sprintf("Area:         %.0f m2", p.Area),
		fmt.Sprintf("Rooms:        %d", p.Rooms),
		"Energy label: " + p.Energy,
	}, "\n")
}

func guess(args []string) string {
	if len(args) != 2 {
		return guessUsage
	}
	g, err := strconv.ParseInt(args[1], 10, 64)
	if err != nil || g < 0 {
		return guessUsage
	}
	p, ok := find(args[0])
	if !ok {
		return fmt.Sprintf("error: listing %q not found", args[0])
	}
	return strings.Join([]string{
		fmt.Sprintf("Actual price: EUR %d", p.Price),
		fmt.Sprintf("Your guess:   EUR %d", g),
		fmt.Sprintf("Score: %d/1000", game.Score(p.Price, g)),
	}, "\n")
}
