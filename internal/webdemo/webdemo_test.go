package webdemo

import (
	"strings"
	"sync"
	"testing"
)

func TestHelpAbout(t *testing.T) {
	h := Exec("HELP")
	for _, c := range []string{"help", "about", "search", "show", "guess"} {
		if !strings.Contains(h, c) {
			t.Errorf("help missing %s", c)
		}
	}
	a := Exec("about")
	if !strings.Contains(a, "fictional") || !strings.Contains(a, "no network access") || strings.Contains(a, "\n") {
		t.Errorf("about = %q", a)
	}
}

func TestEmptyAndUnknown(t *testing.T) {
	if Exec("") != "" || Exec("  \t ") != "" {
		t.Error("expected empty")
	}
	if got := Exec("foo bar"); got != "unknown command: foo. Type 'help' for available commands." {
		t.Errorf("got %q", got)
	}
}

func TestSearch(t *testing.T) {
	all := strings.Split(Exec("search"), "\n")
	if len(all) != 9 {
		t.Fatalf("lines = %d", len(all))
	}
	u := Exec("search --city utrecht")
	if n := strings.Count(u, "Utrecht"); n != 3 {
		t.Errorf("utrecht rows = %d", n)
	}
	if strings.Contains(u, "Amsterdam") {
		t.Error("leak")
	}
	a := Exec("search --category rent --city AMSTERDAM")
	b := Exec("search --city amsterdam --category RENT")
	if a != b || strings.Count(a, "\n") != 2 || strings.Contains(a, "buy") {
		t.Errorf("a=%q b=%q", a, b)
	}
	if !(strings.Index(all[1], "1001") >= 0 && strings.Index(all[8], "1008") >= 0) {
		t.Error("not sorted")
	}
	if Exec("search --city Nowhere") != "No listings found." {
		t.Error("no match")
	}
	for _, in := range []string{"search --category lease", "search --bogus x", "search --city", "search --category", "search city"} {
		got := Exec(in)
		if !strings.HasPrefix(got, "error: ") || !strings.HasSuffix(got, "usage: search [--city NAME] [--category rent|buy]") {
			t.Errorf("%s -> %q", in, got)
		}
	}
}

func TestShow(t *testing.T) {
	got := Exec("show 1001")
	for _, w := range []string{"Oudegracht 12", "Utrecht", "buy", "525000", "85", "4", "B"} {
		if !strings.Contains(got, w) {
			t.Errorf("missing %q in %q", w, got)
		}
	}
	if Exec("show") != "usage: show ID" || Exec("show 1 2") != "usage: show ID" {
		t.Error("usage")
	}
	if Exec("show 9999") != `error: listing "9999" not found` {
		t.Error("not found")
	}
}

func TestGuess(t *testing.T) {
	got := Exec("guess 1001 525000")
	for _, w := range []string{"525000", "Score: 1000/1000"} {
		if !strings.Contains(got, w) {
			t.Errorf("missing %q in %q", w, got)
		}
	}
	if !strings.Contains(Exec("guess 1001 0"), "Score: 0/1000") {
		t.Error("zero guess")
	}
	for _, in := range []string{"guess", "guess 1001", "guess 1001 x", "guess 1001 -5", "guess 1001 1.5", "guess 1001 1 2"} {
		if Exec(in) != "guess ID PRICE" && Exec(in) != "usage: guess ID PRICE" {
			t.Errorf("%s -> %q", in, Exec(in))
		}
	}
	if Exec("guess 9999 100") != `error: listing "9999" not found` {
		t.Error("not found")
	}
}

func TestPlainAndConcurrent(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for _, c := range []string{"help", "search", "show 1002", "guess 1002 1600"} {
				o := Exec(c)
				if strings.Contains(o, "\x1b") || strings.HasSuffix(o, "\n") || o != Exec(c) {
					t.Errorf("bad output for %s", c)
				}
			}
		}()
	}
	wg.Wait()
}
