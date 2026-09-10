package enrich

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/sc0vu/funda-cli/internal/client"
	"github.com/sc0vu/funda-cli/internal/model"
)

const API_BASE = "https://listing-detail-page.funda.io/api/v4/listing/object/nl/tinyId"

func Fetch(ctx context.Context, profile string, timeout time.Duration, objectID string) (model.Listing, error) {
	var out model.Listing
	c, err := client.NewClient(profile, timeout)
	if err != nil {
		return out, err
	}

	header := http.Header{
		"Accept": {
			"application/json",
		},
	}

	apiURL := fmt.Sprintf("%s/%s", API_BASE, objectID)
	body, err := c.Get(ctx, apiURL, header)
	if err != nil {
		return out, err
	}
	var response model.FundaListingResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return out, fmt.Errorf("decode provider JSON: %w", err)
	}
	out = response.ToListing()
	return out, err
}
