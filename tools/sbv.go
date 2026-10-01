package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

// CentralRate is the State Bank of Vietnam's daily USD/VND central rate (tỷ giá trung
// tâm). The dong trades in a band around it, so the gap between the market rate and this
// one shows where in the band the dong sits.
type CentralRate struct {
	Date   string // as published, e.g. 2026-10-01
	USDVND float64
	Stale  bool
}

// SBVAttribution must be shown as a link wherever the rate is used: the mirror's terms.
const SBVAttribution = "https://allratestoday.com/central-bank-rates-api/sbv/"

// ponytail: a third-party mirror (the SBV has no machine-readable feed). It can vanish,
// which is why this is a soft dependency.
const sbvURL = "https://allratestoday.com/api/open/central-bank/sbv"

func FetchCentralRate(ctx context.Context) (CentralRate, error) {
	data, err := get(ctx, sbvURL)
	if err != nil {
		return CentralRate{}, err
	}
	return parseCentralRate(data)
}

// parseCentralRate reads the USD "reference" rate: the central rate. The "buy" and
// "sell" entries are the SBV's own dealing rates at the band edges.
func parseCentralRate(data []byte) (CentralRate, error) {
	var resp struct {
		RateDate string `json:"rate_date"`
		Stale    bool   `json:"stale"`
		Rates    []struct {
			Base, Quote, Type string
			Value             float64
		} `json:"rates"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return CentralRate{}, err
	}
	for _, r := range resp.Rates {
		if r.Base == "USD" && r.Quote == "VND" && r.Type == "reference" && r.Value > 0 {
			return CentralRate{Date: resp.RateDate, USDVND: r.Value, Stale: resp.Stale}, nil
		}
	}
	return CentralRate{}, fmt.Errorf("no USD/VND reference rate in the SBV response")
}
