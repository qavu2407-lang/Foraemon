package email

import (
	"html"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"lorisocchipinti.com/gbp-rates/explain"
	"lorisocchipinti.com/gbp-rates/rates"
	"lorisocchipinti.com/gbp-rates/tools"
)

// snap builds a snapshot quoted per 1 USD, as the provider sends it.
func snap(at int64, aud, vnd, czk float64) rates.Snapshot {
	return rates.Snapshot{PublishedAt: at, PerUSD: map[string]float64{"AUD": aud, "VND": vnd, "CZK": czk}}
}

// visibleText is what a reader sees: tags dropped, entities decoded, whitespace collapsed.
// The golden test compares this rather than the HTML, so restyling the email doesn't
// break it but changing a word, number or sign does.
func visibleText(body string) string {
	text := html.UnescapeString(regexp.MustCompile(`<[^>]*>`).ReplaceAllString(body, " "))
	return strings.Join(strings.Fields(text), " ")
}

// The email is the product, so pin its content whole: a wording change that makes the VND
// sign ambiguous again, or a number in the wrong place, fails here.
func Test_Render_Golden(t *testing.T) {
	prev := snap(1790294400, 1.4210, 26280, 21.40)
	cur := snap(1790380800, 1.4400, 26263, 21.35)
	pairs := rates.DerivePairs(cur, &prev)
	// Closes of 31 Dec 2025, 31 Aug 2026 and Sunday 20 Sep 2026: the YTD, MTD and WTD bases.
	h := []rates.Snapshot{snap(1767222000, 1.50, 26000, 20.0), snap(1788217200, 1.46, 26200, 21.0), snap(1789945200, 1.43, 26270, 21.3), prev, cur}
	b := explain.Brief{
		News: []tools.Headline{
			{Publisher: "RBA", Title: "RBA holds", URL: "https://example.com/rba", Published: time.Date(2026, 9, 25, 4, 30, 0, 0, time.UTC)},
			{Publisher: "FXStreet", Title: "AUD slips", URL: "https://example.com/fx", Published: time.Date(2026, 9, 25, 6, 0, 0, 0, time.UTC)},
			{Publisher: "Uncited", Title: "Not in the list", URL: "https://example.com/x"},
		},
		Explanation: &explain.Explanation{
			Headlines: []explain.Claim{{Currency: "AUD", Text: "RBA holds rates\r\nBcc: x@example.com", Cites: []int{1}}},
			Macro: []explain.Claim{
				{Currency: "AUD", Text: "According to FXStreet, the AUD fell after the RBA held.", Cites: []int{2}},
				{Currency: "VND", Text: "The dong likely tracked the dollar."},
			},
			Scenarios: []explain.Scenario{{If: "US payrolls beat", Then: "the dollar may firm"}},
		},
		SBV:      &tools.CentralRate{Date: "2026-09-26", USDVND: 25624},
		Calendar: []tools.Event{{Time: time.Date(2026, 9, 26, 12, 30, 0, 0, time.UTC), Country: "USD", Title: "Non-Farm Employment Change", Impact: "High", Forecast: "89K", Previous: "162K"}},
	}

	subject, body, err := Render(Build(cur, &prev, pairs, map[string]float64{"AUD/VND": 18450}, rates.PeriodRows(h, cur, pairs), b))
	assert.Nil(t, err)
	assert.Equal(t, "AUD/VND 18,238.19 (-1.38%) · CZK/VND 1,230.12 (+0.17%)", subject,
		"the subject is the two cross pairs only: no model text reaches the mail header")
	assert.Equal(t, strings.Join([]string{
		"Good morning. Here is your FX update for 26 Sep 2026.",
		"1. Market snapshot In today's update: RBA holds rates Bcc: x@example.com [1]",
		"AUD/VND 18,238.19 -1.38% -1.15% vs your entry 18,450.00",
		"CZK/VND 1,230.12 +0.17% Rates published 26 Sep 2026, 07:00 ICT, change since 25 Sep 2026, 07:00 ICT. Why they moved is in section 3.",
		"2. FX market rates Today Last WTD MTD YTD",
		"AUD/VND 18,238.19 18,494.02 -0.72% +1.63% +5.22%",
		"AUD/USD 0.6944 0.7037 -0.69% +1.39% +4.17%",
		"USD/VND 26,263 26,280 -0.03% +0.24% +1.01%",
		"CZK/USD 0.0468 0.0467 -0.23% -1.64% -6.32%",
		"CZK/VND 1,230.12 1,228.04 -0.26% -1.40% -5.38%",
		"All rates are mid-market from ExchangeRate-API. What you get on a transfer is lower by the provider's fee.",
		"Today is the latest publication (26 Sep 2026, 07:00 ICT). Last is the one before it.",
		"WTD, MTD and YTD compare today's rate with the last publication before Monday, the 1st of the month, and 1 January, each at 00:00 UTC, the provider's day.",
		"History before 1 Oct 2026 is backfilled from ExchangeRate-API's historical data, one blended mid-rate per day.",
		"— means the history doesn't go back far enough.",
		"3. Why it moved",
		"Which leg moved AUD/VND 18,238.19 -1.38% AUD leg -1.32pp AUD weaker vs USD VND leg -0.06pp VND stronger vs USD AUD/USD 0.6944 -1.32% USD/VND 26,263 -0.06%",
		"CZK/VND 1,230.12 +0.17% CZK leg +0.23pp CZK stronger vs USD VND leg -0.06pp VND stronger vs USD CZK/USD 0.0468 +0.23% USD/VND 26,263 -0.06%",
		"Drivers AUD: According to FXStreet, the AUD fell after the RBA held. [2] VND: The dong likely tracked the dollar. (no source)",
		"The SBV central rate is 25,624 (2026-09-26). Market USD/VND at 26,263 is 2.49% above it, so the dong is trading weaker than the central rate.",
		"Technical context 30-day range 52-week range Streak",
		"AUD/VND 17,945.21–18,494.02 (at 53%) — down 1 in a row",
		"AUD/USD 0.6849–0.7037 (at 51%) — down 1 in a row",
		"USD/VND 26,200–26,280 (at 79%) — down 1 in a row",
		"CZK/USD 0.0467–0.0476 (at 12%) — up 1 in a row",
		"CZK/VND 1,228.04–1,247.62 (at 11%) — up 1 in a row",
		`The drivers are written by a language model from the cited news. "Likely" or "consistent with" marks its own inference, not a reported fact.`,
		"Some sources, such as FXStreet and Investing.com, may need a VPN to open from Vietnam. The source list at the end gives each one's headline, so a claim can be checked without opening the link.",
		`"At 0%" is the bottom of the range and "at 100%" the top. A streak counts publications moving the same way.`,
		"4. What's next Calendar, next 48 hours (ICT) When Event Impact Forecast Previous",
		"Sat 26 Sep 19:30 USD Non-Farm Employment Change High 89K 162K",
		"AUD, USD and EUR only: the calendar has no CZK or VND, and EUR stands in for the koruna.",
		"Scenarios If US payrolls beat, then the dollar may firm",
		`For information only, not financial advice. The headlines, drivers and scenarios are written by a language model from the linked news; numbers in brackets link to the headline each claim rests on. Every rate and percentage is computed, not written by the model.`,
		"Sources: rates from ExchangeRate-API · calendar from ForexFactory · SBV central rate via AllRatesToday .",
		"Sources cited RBA, 25 Sep: RBA holds FXStreet, 25 Sep: AUD slips",
	}, " "), visibleText(body))
	assert.Contains(t, body, `href="https://example.com/rba"`, "citations link to the headline")
	assert.NotContains(t, body, "Not in the list", "only cited headlines are listed")
}

func Test_Render_FirstRunWithoutBrief(t *testing.T) {
	cur := snap(1790380800, 1.4400, 26263, 21.35)
	pairs := rates.DerivePairs(cur, nil)
	subject, body, err := Render(Build(cur, nil, pairs, nil, rates.PeriodRows(nil, cur, pairs), explain.Brief{NoExplain: "no headlines could be fetched"}))
	assert.Nil(t, err)
	assert.Equal(t, "AUD/VND 18,238.19 · CZK/VND 1,230.12", subject, "no change without history, no headline without a model")
	assert.Contains(t, body, "First run")
	assert.Contains(t, body, "No explanation today: no headlines could be fetched")
	assert.NotContains(t, body, "leg")
}
