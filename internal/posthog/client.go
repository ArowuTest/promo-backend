package posthog

import (
	"fmt"
	"time"

	"github.com/ArowuTest/promo-backend/internal/config"
	"github.com/ArowuTest/promo-backend/internal/models"
)

// Client is a placeholder around your PostHog integration.
// For now, FetchEligibleEntries always returns an empty slice.
// Later you can replace this stub with real PostHog calls.
type Client struct {
	apiKey   string
	endpoint string
}

// NewClient constructs a “client” using AppConfig.  It does *not* fail if keys are missing.
func NewClient(cfg *config.AppConfig) (*Client, error) {
	if cfg.PosthogAPIKey == "" || cfg.PosthogEndpoint == "" {
		// Missing values, but we’ll still return a Client stub.
		return &Client{apiKey: cfg.PosthogAPIKey, endpoint: cfg.PosthogEndpoint}, nil
	}
	return &Client{apiKey: cfg.PosthogAPIKey, endpoint: cfg.PosthogEndpoint}, nil
}

// Close is a no-op for now.
func (c *Client) Close() {
	// no longer holding any connections
}

// FetchEligibleEntries should call PostHog, fetch all “Recharge” events (or whatever event name),
// and return distinct MSISDNs + total points.  For now it returns an empty slice to keep the build green.
func (c *Client) FetchEligibleEntries(since, until time.Time) ([]models.EligibleEntry, error) {
	// ==== REPLACE THIS STUB with real PostHog query logic ====
	fmt.Println("posthog integration is not yet implemented; returning zero entries")
	return []models.EligibleEntry{}, nil
}
