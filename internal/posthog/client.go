// internal/posthog/client.go

package posthog

import (
	"fmt"
	"time"

	"github.com/ArowuTest/promo-backend/internal/config"
	"github.com/ArowuTest/promo-backend/internal/models"
	"github.com/posthog/posthog-go"
)

// Client wraps a PostHog SDK client.
type Client struct {
	ph posthog.Client
}

// NewClient constructs a PostHog client from AppConfig.
// In your .env or Render env, set POSTHOG_API_KEY and POSTHOG_INSTANCE_ADDRESS.
func NewClient(cfg *config.AppConfig) (*Client, error) {
	// This uses the posthog-go v1.x constructor.  You might need to adjust
	// if PostHog changes their API.  For now, we assume:
	ph, err := posthog.NewWithConfig(cfg.PosthogAPIKey, posthog.Config{
		Endpoint: cfg.PosthogEndpoint,
	})
	if err != nil {
		return nil, fmt.Errorf("posthog: failed to create client: %w", err)
	}
	return &Client{ph: ph}, nil
}

// Close cleans up the PostHog client.
func (c *Client) Close() {
	c.ph.Close()
}

// FetchEligibleEntries pulls all “Recharge” events between since & until,
// and aggregates points per MSISDN.  In production, replace this stub
// with the real PostHog QueryEvents logic.
func (c *Client) FetchEligibleEntries(since, until time.Time) ([]models.EligibleEntry, error) {
	// ────────────────────────────────────────────────────────────────────────────
	// TODO: Replace this stub with actual PostHog event‐querying logic.
	// For now, return an empty slice so that the backend compiles and runs.
	// ────────────────────────────────────────────────────────────────────────────
	return []models.EligibleEntry{}, nil
}
