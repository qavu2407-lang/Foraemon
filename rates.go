package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
)

// needed are the currencies the five pairs are built from.
var needed = []string{"AUD", "VND", "CZK"}

// Snapshot is one published set of rates, quoted per 1 USD as the provider sends them.
type Snapshot struct {
	PublishedAt int64              `json:"published_at"`
	PerUSD      map[string]float64 `json:"per_usd"`
}

type latestResponse struct {
	Result          string             `json:"result"`
	ErrorType       string             `json:"error-type"`
	TimeLastUpdate  int64              `json:"time_last_update_unix"`
	ConversionRates map[string]float64 `json:"conversion_rates"`
}

// fetchRates makes one call for every currency, so all five pairs come from the same
// publication. Separate calls could straddle an update and attribute a move that never
// happened at any single moment.
func fetchRates(ctx context.Context) (Snapshot, error) {
	apikey := os.Getenv("EXCHANGERATE_API_KEY")
	if apikey == "" {
		return Snapshot{}, fmt.Errorf("EXCHANGERATE_API_KEY is not set")
	}

	// The key travels in the path, so this URL is a secret: never log it.
	url := fmt.Sprintf("https://v6.exchangerate-api.com/v6/%s/latest/USD", apikey)
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
	var resp latestResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return Snapshot{}, err
	}
	if resp.Result != "success" {
		return Snapshot{}, fmt.Errorf("lookup failed: %s", resp.ErrorType)
	}
	if resp.TimeLastUpdate == 0 {
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
	return Snapshot{PublishedAt: resp.TimeLastUpdate, PerUSD: perUSD}, nil
}

// Every rate is "units of QUOTE per 1 BASE" and named BASE/QUOTE. The provider quotes
// everything per 1 USD, i.e. as USD/xxx, so the reciprocals are taken here and nowhere
// else. The order is the email's order: the headline pair first.
//
// split names the base currency of a cross pair: X/VND = X/USD × USD/VND, and the email
// splits its move into those two legs. Both legs must be pairs listed here.
var pairDefs = []struct {
	name     string
	decimals int
	split    string
	rate     func(perUSD map[string]float64) float64
}{
	{"AUD/VND", 4, "AUD", func(u map[string]float64) float64 { return u["VND"] / u["AUD"] }},
	{"AUD/USD", 4, "", func(u map[string]float64) float64 { return 1 / u["AUD"] }},
	{"USD/VND", 0, "", func(u map[string]float64) float64 { return u["VND"] }},
	{"CZK/USD", 4, "", func(u map[string]float64) float64 { return 1 / u["CZK"] }},
	{"CZK/VND", 4, "CZK", func(u map[string]float64) float64 { return u["VND"] / u["CZK"] }},
}

type Pair struct {
	Name     string
	Rate     float64
	Prev     float64 // 0 when there is no earlier publication to compare against
	Decimals int
	Split    string // base currency of a cross pair to split, "" for none
}

func (p Pair) HasPrev() bool { return p.Prev > 0 }

// Pct is the point-in-time change since Prev, in percent: the convention rate
// platforms use for "% change".
func (p Pair) Pct() float64 { return (p.Rate/p.Prev - 1) * 100 }

func derivePairs(cur Snapshot, prev *Snapshot) []Pair {
	pairs := make([]Pair, len(pairDefs))
	for i, def := range pairDefs {
		pairs[i] = Pair{Name: def.name, Rate: def.rate(cur.PerUSD), Decimals: def.decimals, Split: def.split}
		if prev != nil {
			pairs[i].Prev = def.rate(prev.PerUSD)
		}
	}
	return pairs
}

// Attribution splits a cross pair's move (AUD/VND, CZK/VND) into its base leg (AUD/USD,
// CZK/USD) and the USD/VND leg. All three are in percent; the legs are percentage points
// of Total.
type Attribution struct {
	Total   float64
	BaseLeg float64
	VNDLeg  float64
	Flat    bool // the move would print as 0.00%, so there is nothing to split
}

// flatBelow is half the smallest displayed step: anything smaller rounds to 0.00%.
const flatBelow = 0.005

func attribute(cross, base, usdvnd Pair) Attribution {
	total := cross.Pct()
	if math.Abs(total) < flatBelow {
		return Attribution{Total: total, Flat: true}
	}
	// Percentages of a product don't add, but logs do: ln of the cross move is exactly
	// lb + lu. Scaling each leg's share by the plain % keeps the legs summing to the
	// pair's figure with no leftover cross term.
	lb := math.Log(base.Rate / base.Prev)
	lu := math.Log(usdvnd.Rate / usdvnd.Prev)
	return Attribution{Total: total, BaseLeg: total * lb / (lb + lu), VNDLeg: total * lu / (lb + lu)}
}

// hundredths rounds to the displayed precision (0.01%) so the legs add up to the total
// exactly: the larger leg absorbs any rounding difference. A column that doesn't add up
// undermines every other number in the email.
func (a Attribution) hundredths() (total, base, vnd int64) {
	total, base, vnd = toHundredths(a.Total), toHundredths(a.BaseLeg), toHundredths(a.VNDLeg)
	if diff := total - base - vnd; diff != 0 {
		if math.Abs(a.BaseLeg) >= math.Abs(a.VNDLeg) {
			base += diff
		} else {
			vnd += diff
		}
	}
	return total, base, vnd
}

func toHundredths(pct float64) int64 { return int64(math.Round(pct * 100)) }

// formatHundredths signs non-zero values only: "+0.00" would claim a direction.
func formatHundredths(h int64) string {
	if h == 0 {
		return "0.00"
	}
	return fmt.Sprintf("%+.2f", float64(h)/100)
}

func sign(h int64) int {
	switch {
	case h > 0:
		return 1
	case h < 0:
		return -1
	}
	return 0
}

// formatNum renders a positive rate with thousands separators: 18502.54 -> 18,502.54.
func formatNum(v float64, decimals int) string {
	whole, frac, _ := strings.Cut(strconv.FormatFloat(v, 'f', decimals, 64), ".")
	var b strings.Builder
	for i, r := range whole {
		if i > 0 && (len(whole)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	if frac != "" {
		b.WriteString("." + frac)
	}
	return b.String()
}
