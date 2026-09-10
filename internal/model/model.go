package model

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

type ListingRef struct {
	ID              string
	URL             string
	City            string
	TransactionType string
	LastMod         string
	FirstSeenAt     time.Time
	LastSeenAt      time.Time
}

type Listing struct {
	ID              string  `json:"id"`
	URL             string  `json:"url"`
	TransactionType string  `json:"transaction_type"`
	City            string  `json:"city"`
	Address         string  `json:"address"`
	Price           float64 `json:"price"`
	LivingArea      float64 `json:"living_area"`
	Bedrooms        int     `json:"bedrooms"`
	Rooms           int     `json:"rooms"`
	EnergyLabel     string  `json:"energy_label"`
	Latitude        float64 `json:"latitude"`
	Longitude       float64 `json:"longitude"`
	Status          string  `json:"status"`
	FetchedAt       string  `json:"fetched_at"`

	// Search metadata. These fields are populated when listings are joined
	// with listing_refs and are not part of the detail-provider JSON payload.
	HasDetails bool      `json:"-"`
	LastSeenAt time.Time `json:"-"`
}

type FundaListingResponse struct {
	Identifiers        Identifiers        `json:"Identifiers"`
	Price              Price              `json:"Price"`
	KenmerkSections    []KenmerkSection   `json:"KenmerkSections"`
	URLs               URLs               `json:"Urls"`
	ListingDescription ListingDescription `json:"ListingDescription"`
	AddressDetails     AddressDetails     `json:"AddressDetails"`
	Coordinates        Coordinates        `json:"Coordinates"`
	Advertising        Advertising        `json:"Advertising"`
	FastView           FastView           `json:"FastView"`

	IsSoldOrRented   bool   `json:"IsSoldOrRented"`
	ObjectType       string `json:"ObjectType"`
	OfferingType     string `json:"OfferingType"`
	ConstructionType string `json:"ConstructionType"`
	PublicationDate  string `json:"PublicationDate"`
}

type Identifiers struct {
	GlobalID int64  `json:"GlobalId"`
	TinyID   string `json:"TinyId"`
}

type Price struct {
	SellingPrice        string `json:"SellingPrice"`
	NumericSellingPrice int    `json:"NumericSellingPrice"`

	RentalPrice        string `json:"RentalPrice"`
	NumericRentalPrice int    `json:"NumericRentalPrice"`

	IsAuction bool `json:"IsAuction"`
}

type KenmerkSection struct {
	Index         int       `json:"Index"`
	ID            string    `json:"Id"`
	Title         string    `json:"Title"`
	KenmerkenList []Kenmerk `json:"KenmerkenList"`
}

type Kenmerk struct {
	Index         int         `json:"Index"`
	ID            string      `json:"Id"`
	Label         string      `json:"Label"`
	Value         string      `json:"Value"`
	LabelStyle    string      `json:"LabelStyle"`
	EnergyData    *EnergyData `json:"EnergyData,omitempty"`
	EnergyLabel   string      `json:"EnergyLabel,omitempty"`
	KenmerkenList []Kenmerk   `json:"KenmerkenList,omitempty"`
}

type EnergyData struct {
	EnergyLabel string `json:"EnergyLabel"`
}

type URLs struct {
	FriendlyURL FriendlyURL `json:"FriendlyUrl"`
}

type FriendlyURL struct {
	FullURL     string `json:"FullUrl"`
	RelativeURL string `json:"RelativeUrl"`
}

type ListingDescription struct {
	Title       string `json:"Title"`
	Description string `json:"Description"`
}

type AddressDetails struct {
	Title                  string `json:"Title"`
	SubTitle               string `json:"SubTitle"`
	IsInternational        bool   `json:"IsInternational"`
	NeighborhoodName       string `json:"NeighborhoodName"`
	NeighborhoodIdentifier string `json:"NeighborhoodIdentifier"`
	City                   string `json:"City"`
	Province               string `json:"Province"`
	Country                string `json:"Country"`
	HouseNumber            string `json:"HouseNumber"`
	HouseNumberExtension   string `json:"HouseNumberExtension"`
	PostCode               string `json:"PostCode"`
}

type Coordinates struct {
	Latitude  float64 `json:"Latitude"`
	Longitude float64 `json:"Longitude"`
}

type Advertising struct {
	OfferType        string           `json:"OfferType"`
	ContentURL       string           `json:"ContentUrl"`
	TargetingOptions TargetingOptions `json:"TargetingOptions"`
}

type TargetingOptions struct {
	OfferingType string `json:"soortaanbod"`
	GlobalID     string `json:"globalid"`
	TinyID       string `json:"tinyid"`

	Province     string `json:"provincie"`
	City         string `json:"plaats"`
	Municipality string `json:"gemeente"`
	Neighborhood string `json:"buurt"`
	PostCode     string `json:"postcode"`
	HouseNumber  string `json:"huisnummer"`

	SellingPrice string `json:"koopprijs"`
	RentalPrice  string `json:"huurprijs"`
	LivingArea   string `json:"woonoppervlakte"`
	PlotArea     string `json:"perceeloppervlakte"`
	Rooms        string `json:"aantalkamers"`
	ObjectType   string `json:"soortobject"`
	HouseType    string `json:"soortwoning"`
	EnergyClass  string `json:"energieklasse"`
	BuildType    string `json:"bouwvorm"`
	BuildYear    string `json:"bouwjaar"`
	Status       string `json:"status"`
}

type FastView struct {
	LivingArea       string `json:"LivingArea"`
	PlotArea         string `json:"PlotArea"`
	NumberOfBedrooms string `json:"NumberOfBedrooms"`
	EnergyLabel      string `json:"EnergyLabel"`
}

// ToListing converts the raw Funda detail payload into the application's
// normalized Listing model.
//
// Fallback order follows the pyfunda detail parser where practical:
//   - price: NumericSellingPrice -> NumericRentalPrice
//   - living area: advertising -> kenmerken -> FastView
//   - rooms: advertising -> kenmerken
//   - energy label: FastView -> advertising
//   - status: kenmerken -> advertising
func (r *FundaListingResponse) ToListing() Listing {
	return Listing{
		URL:             r.listingURL(),
		TransactionType: normalizeTransactionType(r.OfferingType),
		City:            r.AddressDetails.City,
		Address:         r.AddressDetails.Title,
		Price:           r.price(),
		LivingArea:      r.livingArea(),
		Bedrooms:        parseInt(r.FastView.NumberOfBedrooms),
		Rooms:           r.rooms(),
		EnergyLabel:     r.energyLabel(),
		Latitude:        r.Coordinates.Latitude,
		Longitude:       r.Coordinates.Longitude,
		Status:          normalizeStatus(r.status(), r.OfferingType),
		FetchedAt:       time.Now().UTC().Format(time.RFC3339),
	}
}

func (r *FundaListingResponse) listingURL() string {
	if r.URLs.FriendlyURL.FullURL != "" {
		return r.URLs.FriendlyURL.FullURL
	}

	id := r.Identifiers.TinyID
	if id == "" && r.Identifiers.GlobalID != 0 {
		id = strconv.FormatInt(r.Identifiers.GlobalID, 10)
	}
	if id == "" {
		id = r.Advertising.TargetingOptions.TinyID
	}
	if id == "" {
		return ""
	}

	offer := normalizeTransactionType(r.OfferingType)
	switch offer {
	case "sale":
		offer = "koop"
	case "rental":
		offer = "huur"
	default:
		return fmt.Sprintf("https://www.funda.nl/detail/%s", id)
	}

	if r.AddressDetails.City == "" {
		return fmt.Sprintf("https://www.funda.nl/detail/%s/%s", offer, id)
	}

	return fmt.Sprintf(
		"https://www.funda.nl/detail/%s/%s/%s",
		offer,
		strings.ToLower(r.AddressDetails.City),
		id,
	)
}

func (r *FundaListingResponse) price() float64 {
	if r.Price.NumericSellingPrice > 0 {
		return float64(r.Price.NumericSellingPrice)
	}
	if r.Price.NumericRentalPrice > 0 {
		return float64(r.Price.NumericRentalPrice)
	}

	ads := r.Advertising.TargetingOptions
	if n := parseFloat(ads.SellingPrice); n > 0 {
		return n
	}
	return parseFloat(ads.RentalPrice)
}

func (r *FundaListingResponse) livingArea() float64 {
	if n := parseFloat(r.Advertising.TargetingOptions.LivingArea); n > 0 {
		return n
	}

	if value, ok := r.KenmerkByID("afmetingen-gebruiksoppervlakten-wonen"); ok {
		if n := parseFloat(value); n > 0 {
			return n
		}
	}

	if value, ok := r.KenmerkByLabel("Wonen"); ok {
		if n := parseFloat(value); n > 0 {
			return n
		}
	}

	return parseFloat(r.FastView.LivingArea)
}

func (r *FundaListingResponse) rooms() int {
	if n := parseInt(r.Advertising.TargetingOptions.Rooms); n > 0 {
		return n
	}

	if value, ok := r.KenmerkByID("indeling-totalrooms"); ok {
		if n := parseInt(value); n > 0 {
			return n
		}
	}

	if value, ok := r.KenmerkByLabel("Aantal kamers"); ok {
		return parseInt(value)
	}

	return 0
}

func (r *FundaListingResponse) energyLabel() string {
	if r.FastView.EnergyLabel != "" {
		return r.FastView.EnergyLabel
	}

	return strings.ToUpper(r.Advertising.TargetingOptions.EnergyClass)
}

func (r *FundaListingResponse) status() string {
	if value, ok := r.KenmerkByID("overdracht-status"); ok && value != "" {
		return value
	}

	if value, ok := r.KenmerkByLabel("Status"); ok && value != "" {
		return value
	}

	return r.Advertising.TargetingOptions.Status
}

// KenmerkByID recursively looks up a characteristic by its stable Funda ID.
func (r *FundaListingResponse) KenmerkByID(id string) (string, bool) {
	for _, section := range r.KenmerkSections {
		if value, ok := walkKenmerken(section.KenmerkenList, func(k Kenmerk) bool {
			return k.ID == id
		}); ok {
			return value, true
		}
	}

	return "", false
}

// KenmerkByLabel recursively looks up a characteristic by display label.
// This is useful as a fallback for characteristics without stable IDs.
func (r *FundaListingResponse) KenmerkByLabel(label string) (string, bool) {
	for _, section := range r.KenmerkSections {
		if value, ok := walkKenmerken(section.KenmerkenList, func(k Kenmerk) bool {
			return k.Label == label
		}); ok {
			return value, true
		}
	}

	return "", false
}

func walkKenmerken(items []Kenmerk, match func(Kenmerk) bool) (string, bool) {
	for _, item := range items {
		if match(item) {
			return item.Value, true
		}

		if value, ok := walkKenmerken(item.KenmerkenList, match); ok {
			return value, true
		}
	}

	return "", false
}

func normalizeTransactionType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "rental", "huur":
		return "rental"
	case "sale", "koop":
		return "sale"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func normalizeStatus(value, offeringType string) string {
	value = strings.ToLower(strings.TrimSpace(value))

	switch value {
	case "beschikbaar", "available":
		return "available"
	case "verkocht", "sold":
		return "sold"
	case "verhuurd", "rented":
		return "rented"
	case "onder bod", "under offer":
		return "under_offer"
	case "verkocht onder voorbehoud":
		return "sold_subject_to_conditions"
	case "verhuurd onder voorbehoud":
		return "rented_subject_to_conditions"
	}

	// If the source only exposes a generic sold/rented flag elsewhere,
	// callers can refine this based on OfferingType. Preserve unknown
	// values rather than guessing.
	_ = offeringType
	return value
}

// parseFloat extracts the first numeric value from Funda display strings such
// as "43 m²", "1029", or "€ 1.029".
func parseFloat(value string) float64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}

	var b strings.Builder
	started := false

	for _, r := range value {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
			started = true
		case (r == ',' || r == '.') && started:
			b.WriteRune(r)
		case started:
			goto done
		}
	}

done:
	token := b.String()
	if token == "" {
		return 0
	}

	// Funda uses Dutch formatted prices such as 1.029, while areas can
	// contain decimal commas. Treat a single separator followed by three
	// digits as a thousands separator.
	if strings.Count(token, ".") == 1 && !strings.Contains(token, ",") {
		parts := strings.Split(token, ".")
		if len(parts) == 2 && len(parts[1]) == 3 {
			token = parts[0] + parts[1]
		}
	}

	token = strings.ReplaceAll(token, ".", "")
	token = strings.ReplaceAll(token, ",", ".")

	n, err := strconv.ParseFloat(token, 64)
	if err != nil {
		return 0
	}

	return n
}

func parseInt(value string) int {
	n := parseFloat(value)
	if n <= 0 {
		return 0
	}
	return int(n)
}
