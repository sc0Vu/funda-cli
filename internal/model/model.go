package model

import "time"

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
