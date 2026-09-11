package mvgm

import (
	"context"
	"fmt"
	"html"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/sc0vu/funda-cli/internal/client"
	"github.com/sc0vu/funda-cli/internal/model"
)

const baseURL = "https://ikwilhuren.nu"

var (
	objectHrefRE  = regexp.MustCompile(`(?i)href=["'](/object/[^"'#?]+)`)
	tagRE         = regexp.MustCompile(`(?s)<[^>]+>`)
	spaceRE       = regexp.MustCompile(`\s+`)
	euroRE        = regexp.MustCompile(`€\s*([0-9][0-9.]*)(?:,([0-9]{1,2}))?`)
	areaRE        = regexp.MustCompile(`(?i)Woonoppervlakte\s*([0-9]+(?:[.,][0-9]+)?)\s*m`)
	bedroomsRE    = regexp.MustCompile(`(?i)Slaapkamers\s*([0-9]+)`)
	energyRE      = regexp.MustCompile(`(?i)Energielabel\s*([A-G][+]{0,4})`)
	statusRE      = regexp.MustCompile(`(?i)Status\s*(Te huur|Verhuurd onder voorbehoud|Verhuurd)`)
	availableRE   = regexp.MustCompile(`(?i)Beschikbaar vanaf\s*([^|]+?)(?:Complex|Plattegrond|Kenmerken|$)`)
	addressRE     = regexp.MustCompile(`(?i)(?:Te huur|Verhuurd onder voorbehoud):\s*([^,]+),\s*([a-zA-ZÀ-ÿ' -]+)`)
	rentRE        = regexp.MustCompile(`(?i)Huurprijs\s*€\s*([0-9][0-9.]*)\s*,?-?\s*/mnd`)
	serviceRE     = regexp.MustCompile(`(?i)Servicekosten\s*€\s*([0-9][0-9.]*)`)
	depositRE     = regexp.MustCompile(`(?i)Waarborg\s*Vast inkomen:\s*€\s*([0-9][0-9.]*)`)
	annual42RE    = regexp.MustCompile(`(?i)(?:bruto[- ]jaarinkomen).*?42x\s+de\s+kale\s+maandhuur`)
	monthly35RE   = regexp.MustCompile(`(?i)(?:bruto[- ]maandinkomen).*?(?:3[,.]5|3\.5)\s*(?:x|keer).*?(?:huurprijs|kale maandhuur)`)
	genericMultRE = regexp.MustCompile(`(?i)([0-9]+(?:[,.][0-9]+)?)\s*(?:x|keer)\s+(?:de\s+)?(?:kale\s+)?maandhuur`)
)

func Sync(ctx context.Context, profile string, timeout time.Duration, city string) ([]model.ListingRef, error) {
	if city == "" {
		city = "utrecht"
	}
	c, err := client.NewClient(profile, timeout)
	if err != nil {
		return nil, err
	}
	u := fmt.Sprintf("%s/aanbod/%s/?lang=nl", baseURL, strings.ToLower(city))
	body, err := c.Get(ctx, u, defaultHeaders())
	if err != nil {
		return nil, err
	}

	matches := objectHrefRE.FindAllStringSubmatch(string(body), -1)
	now := time.Now().UTC()
	seen := map[string]bool{}
	refs := make([]model.ListingRef, 0, len(matches))
	for _, m := range matches {
		path := html.UnescapeString(m[1])
		path = strings.TrimSuffix(path, "/") + "/"
		id := ListingID(path)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		refs = append(refs, model.ListingRef{
			Source: "mvgm", ID: id, URL: baseURL + path, City: city,
			TransactionType: "rent", FirstSeenAt: now, LastSeenAt: now,
		})
	}
	if len(refs) == 0 {
		return nil, fmt.Errorf("MVGM: no listing links found at %s", u)
	}
	return refs, nil
}

func Fetch(ctx context.Context, profile string, timeout time.Duration, ref model.ListingRef) (model.Listing, model.RentalDetails, error) {
	c, err := client.NewClient(profile, timeout)
	if err != nil {
		return model.Listing{}, model.RentalDetails{}, err
	}
	body, err := c.Get(ctx, ref.URL, defaultHeaders())
	if err != nil {
		return model.Listing{}, model.RentalDetails{}, err
	}
	text := normalizeText(string(body))

	l := model.Listing{Source: "mvgm", ID: ref.ID, URL: ref.URL, City: ref.City, TransactionType: "rent", FetchedAt: time.Now().UTC().Format(time.RFC3339)}
	d := model.RentalDetails{Source: "mvgm", ListingID: ref.ID, RawConditions: extractIncomeBlock(text)}

	if m := addressRE.FindStringSubmatch(text); len(m) > 0 {
		l.Address = strings.TrimSpace(m[1])
		if l.City == "" {
			l.City = strings.TrimSpace(m[2])
		}
	}
	l.Price = parseFirstNumber(rentRE, text)
	d.ServiceCost = parseFirstNumber(serviceRE, text)
	d.Deposit = parseFirstNumber(depositRE, text)
	if m := areaRE.FindStringSubmatch(text); len(m) > 0 {
		l.LivingArea = parseDutchFloat(m[1])
	}
	if m := bedroomsRE.FindStringSubmatch(text); len(m) > 0 {
		l.Bedrooms, _ = strconv.Atoi(m[1])
	}
	if m := energyRE.FindStringSubmatch(text); len(m) > 0 {
		l.EnergyLabel = m[1]
	}
	if m := statusRE.FindStringSubmatch(text); len(m) > 0 {
		l.Status = strings.ToLower(strings.TrimSpace(m[1]))
	}
	if m := availableRE.FindStringSubmatch(text); len(m) > 0 {
		d.AvailableFrom = strings.TrimSpace(m[1])
	}

	d.IncomeMultiplier, d.IncomeBasis, d.IncomeRule = parseIncomeRule(text, l.Price)
	if l.Price > 0 && d.IncomeMultiplier > 0 {
		if d.IncomeBasis == "annual" {
			d.RequiredIncome = l.Price * d.IncomeMultiplier
		} else {
			d.RequiredIncome = l.Price * d.IncomeMultiplier * 12
		}
	}
	return l, d, nil
}

func ListingID(path string) string {
	s := strings.Trim(path, "/")
	if s == "" {
		return ""
	}
	parts := strings.Split(s, "/")
	slug := parts[len(parts)-1]
	return slug
}

func parseIncomeRule(text string, rent float64) (float64, string, string) {
	if annual42RE.MatchString(text) && rent > 1600 {
		return 42, "annual", "42 × kale maandhuur (listing rule for rent > €1,600)"
	}
	if annual42RE.MatchString(text) && rent > 0 && rent <= 1600 {
		return 3.5, "monthly", "3.5–4 × monthly rent (general MVGM guidance; 42× clause on page applies above €1,600)"
	}
	if monthly35RE.MatchString(text) {
		return 3.5, "monthly", "3.5 × maandhuur"
	}
	if m := genericMultRE.FindStringSubmatch(text); len(m) > 0 {
		mult := parseDutchFloat(m[1])
		if mult >= 10 {
			return mult, "annual", fmt.Sprintf("%.1f × kale maandhuur", mult)
		}
		return mult, "monthly", fmt.Sprintf("%.1f × maandhuur", mult)
	}
	return 0, "", "unknown"
}

func extractIncomeBlock(text string) string {
	low := strings.ToLower(text)
	start := strings.Index(low, "inkomenseis")
	if start < 0 {
		start = strings.Index(low, "bruto jaarinkomen")
	}
	if start < 0 {
		return ""
	}
	end := len(text)
	for _, marker := range []string{"Interesse?", "Kenmerken", "Plattegrond"} {
		if i := strings.Index(text[start:], marker); i >= 0 && start+i < end {
			end = start + i
		}
	}
	v := strings.TrimSpace(text[start:end])
	if len(v) > 2000 {
		v = v[:2000]
	}
	return v
}

func normalizeText(raw string) string {
	s := tagRE.ReplaceAllString(raw, " ")
	s = html.UnescapeString(s)
	s = strings.ReplaceAll(s, "\u00a0", " ")
	return strings.TrimSpace(spaceRE.ReplaceAllString(s, " "))
}
func parseFirstNumber(re *regexp.Regexp, s string) float64 {
	if m := re.FindStringSubmatch(s); len(m) > 0 {
		return parseDutchFloat(m[1])
	}
	return 0
}
func parseDutchFloat(v string) float64 {
	v = strings.ReplaceAll(v, ".", "")
	v = strings.ReplaceAll(v, ",", ".")
	f, _ := strconv.ParseFloat(v, 64)
	return f
}
func defaultHeaders() http.Header {
	h := http.Header{}
	h.Set("accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	h.Set("accept-language", "nl-NL,nl;q=0.9,en;q=0.8")
	return h
}
