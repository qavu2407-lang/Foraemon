package rates

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

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
	{"AUD/VND", 2, "AUD", func(u map[string]float64) float64 { return u["VND"] / u["AUD"] }},
	{"AUD/USD", 4, "", func(u map[string]float64) float64 { return 1 / u["AUD"] }},
	{"USD/VND", 0, "", func(u map[string]float64) float64 { return u["VND"] }},
	{"CZK/USD", 4, "", func(u map[string]float64) float64 { return 1 / u["CZK"] }},
	{"CZK/VND", 2, "CZK", func(u map[string]float64) float64 { return u["VND"] / u["CZK"] }},
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

func DerivePairs(cur Snapshot, prev *Snapshot) []Pair {
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

func Attribute(cross, base, usdvnd Pair) Attribution {
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
func (a Attribution) Hundredths() (total, base, vnd int64) {
	total, base, vnd = ToHundredths(a.Total), ToHundredths(a.BaseLeg), ToHundredths(a.VNDLeg)
	if diff := total - base - vnd; diff != 0 {
		if math.Abs(a.BaseLeg) >= math.Abs(a.VNDLeg) {
			base += diff
		} else {
			vnd += diff
		}
	}
	return total, base, vnd
}

func ToHundredths(pct float64) int64 { return int64(math.Round(pct * 100)) }

// FormatHundredths signs non-zero values only: "+0.00" would claim a direction.
func FormatHundredths(h int64) string {
	if h == 0 {
		return "0.00"
	}
	return fmt.Sprintf("%+.2f", float64(h)/100)
}

func Sign(h int64) int {
	switch {
	case h > 0:
		return 1
	case h < 0:
		return -1
	}
	return 0
}

// FormatNum renders a positive rate with thousands separators: 18502.54 -> 18,502.54.
func FormatNum(v float64, decimals int) string {
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

// ValidateEntries rejects unknown pair names and non-positive rates, so a typo in the
// event fails loudly instead of silently dropping the line or dividing by zero.
func ValidateEntries(entries map[string]float64) error {
	for name, rate := range entries {
		known := false
		for _, def := range pairDefs {
			known = known || def.name == name
		}
		if !known {
			return fmt.Errorf("entry %q is not a reported pair (use AUD/VND, AUD/USD, USD/VND, CZK/USD or CZK/VND)", name)
		}
		if rate <= 0 {
			return fmt.Errorf("entry for %s must be a positive rate, got %v", name, rate)
		}
	}
	return nil
}
