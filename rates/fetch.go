package rates

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// needed are the currencies the five pairs are built from.
var needed = []string{"AUD", "VND", "CZK"}

// Snapshot is one published set of rates, quoted per 1 USD as the provider sends them.
type Snapshot struct {
	PublishedAt int64              `json:"published_at"`
	PerUSD      map[string]float64 `json:"per_usd"`
}

// rateResponse is both /latest (time_last_update_unix) and /history (year, month, day).
type rateResponse struct {
	Result          string             `json:"result"`
	ErrorType       string             `json:"error-type"`
	TimeLastUpdate  int64              `json:"time_last_update_unix"`
	Year            int                `json:"year"`
	Month           int                `json:"month"`
	Day             int                `json:"day"`
	ConversionRates map[string]float64 `json:"conversion_rates"`
}

// FetchLatest makes one call for every currency, so all five pairs come from the same
// publication. Separate calls could straddle an update and attribute a move that never
// happened at any single moment.
func FetchLatest(ctx context.Context) (Snapshot, error) {
	return FetchSnapshot(ctx, "latest/USD")
}

// FetchSnapshot GETs one rate table, e.g. "latest/USD" or "history/USD/2026/9/30".
func FetchSnapshot(ctx context.Context, path string) (Snapshot, error) {
	apikey := os.Getenv("EXCHANGERATE_API_KEY")
	if apikey == "" {
		return Snapshot{}, fmt.Errorf("EXCHANGERATE_API_KEY is not set")
	}

	// The key travels in the path, so this URL is a secret: never log it.
	url := fmt.Sprintf("https://v6.exchangerate-api.com/v6/%s/%s", apikey, path)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return Snapshot{}, redact(err, apikey)
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return Snapshot{}, redact(err, apikey)
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return Snapshot{}, err
	}

	snap, err := parseRates(body)
	if err != nil {
		return Snapshot{}, fmt.Errorf("rate api returned %s: %w", res.Status, err)
	}
	return snap, nil
}

func parseRates(data []byte) (Snapshot, error) {
	var resp rateResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return Snapshot{}, err
	}
	if resp.Result != "success" {
		return Snapshot{}, fmt.Errorf("lookup failed: %s", resp.ErrorType)
	}
	published := resp.TimeLastUpdate
	if published == 0 && resp.Year > 0 {
		// A history day carries that day's last hourly publication (checked 2026-10-01:
		// today's history equals /latest), so stamp it at 23:00 UTC, the day's close.
		published = time.Date(resp.Year, time.Month(resp.Month), resp.Day, 23, 0, 0, 0, time.UTC).Unix()
	}
	if published == 0 {
		return Snapshot{}, fmt.Errorf("response has no publication time")
	}

	// Keep only what the pairs need: the stored history stays small, and a currency
	// the provider stops sending fails loudly here instead of as a division by zero.
	perUSD := make(map[string]float64, len(needed))
	for _, code := range needed {
		rate := resp.ConversionRates[code]
		if rate <= 0 {
			return Snapshot{}, fmt.Errorf("response has no usable %s rate", code)
		}
		perUSD[code] = rate
	}
	return Snapshot{PublishedAt: published, PerUSD: perUSD}, nil
}

// redact keeps the key out of the logs: net/http wraps failures in a *url.Error
// that prints the whole URL, and the key lives in the path.
func redact(err error, apikey string) error {
	return errors.New(strings.ReplaceAll(err.Error(), apikey, "REDACTED"))
}
