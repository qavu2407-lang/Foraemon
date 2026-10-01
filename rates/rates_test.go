package rates

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func snap(at int64, aud, vnd, czk float64) Snapshot {
	return Snapshot{PublishedAt: at, PerUSD: map[string]float64{"AUD": aud, "VND": vnd, "CZK": czk}}
}

func Test_ParseRates(t *testing.T) {
	ok := `{"result":"success","time_last_update_unix":1790380801,
		"conversion_rates":{"USD":1,"AUD":1.53,"VND":26300,"CZK":21.5,"EUR":0.92}}`
	s, err := parseRates([]byte(ok))
	assert.Nil(t, err)
	assert.Equal(t, snap(1790380801, 1.53, 26300, 21.5), s, "keeps only the needed currencies")

	_, err = parseRates([]byte(`{"result":"error","error-type":"invalid-key"}`))
	assert.ErrorContains(t, err, "invalid-key")

	_, err = parseRates([]byte(`{"result":"success","time_last_update_unix":1,"conversion_rates":{"AUD":1.53,"VND":26300}}`))
	assert.ErrorContains(t, err, "CZK", "a missing currency must fail, not divide by zero later")
}

func Test_DerivePairs_SignConvention(t *testing.T) {
	// The provider sends USD/AUD = 1.5, i.e. 1.5 AUD per dollar. AUD/USD is its reciprocal,
	// and CZK/USD likewise.
	pairs := DerivePairs(snap(1, 1.5, 26000, 20), nil)

	byName := map[string]float64{}
	for _, p := range pairs {
		byName[p.Name] = p.Rate
		assert.False(t, p.HasPrev())
	}
	assert.Equal(t, "AUD/VND", pairs[0].Name, "headline pair first")
	assert.InDelta(t, 0.6667, byName["AUD/USD"], 0.0001)
	assert.InDelta(t, 26000, byName["USD/VND"], 0)
	assert.InDelta(t, 0.05, byName["CZK/USD"], 1e-12)
	assert.InDelta(t, 1300, byName["CZK/VND"], 0.0001)
	assert.InDelta(t, 17333.33, byName["AUD/VND"], 0.01)
	assert.InDelta(t, byName["AUD/USD"]*byName["USD/VND"], byName["AUD/VND"], 1e-9)
	assert.InDelta(t, byName["CZK/USD"]*byName["USD/VND"], byName["CZK/VND"], 1e-9)
}

// pairsByName derives the pairs and indexes them, so tests can pick legs by name.
func pairsByName(cur Snapshot, prev *Snapshot) map[string]Pair {
	m := map[string]Pair{}
	for _, p := range DerivePairs(cur, prev) {
		m[p.Name] = p
	}
	return m
}

func Test_Attribute(t *testing.T) {
	cases := []struct {
		name            string
		base            string // AUD or CZK: which cross pair to split
		prev, cur       Snapshot
		baseDir, vndDir float64 // expected sign of each leg
	}{
		// USD/AUD down = AUD stronger; USD/VND up = VND weaker. Both push AUD/VND up.
		{"AUD: both legs push up", "AUD", snap(1, 1.50, 26000, 20), snap(2, 1.49, 26100, 20), 1, 1},
		// AUD weaker pulls down, VND weaker pushes up.
		{"AUD: legs oppose", "AUD", snap(1, 1.50, 26000, 20), snap(2, 1.52, 26100, 20), -1, 1},
		{"AUD: only VND moved", "AUD", snap(1, 1.50, 26000, 20), snap(2, 1.50, 26100, 20), 0, 1},
		// USD/CZK down = CZK stronger.
		{"CZK: both legs push up", "CZK", snap(1, 1.50, 26000, 20), snap(2, 1.50, 26100, 19.8), 1, 1},
		{"CZK: legs oppose", "CZK", snap(1, 1.50, 26000, 20), snap(2, 1.50, 26100, 20.2), -1, 1},
		{"CZK: only CZK moved", "CZK", snap(1, 1.50, 26000, 20), snap(2, 1.50, 26000, 19.8), 1, 0},
	}
	for _, c := range cases {
		prev := c.prev
		p := pairsByName(c.cur, &prev)
		a := Attribute(p[c.base+"/VND"], p[c.base+"/USD"], p["USD/VND"])

		assert.False(t, a.Flat, c.name)
		assert.InDelta(t, a.Total, a.BaseLeg+a.VNDLeg, 1e-9, "%s: legs must sum to the total", c.name)
		assert.Equal(t, c.baseDir, sgn(a.BaseLeg), c.name)
		assert.Equal(t, c.vndDir, sgn(a.VNDLeg), c.name)
	}
}

func Test_Attribute_NearZeroIsFlat(t *testing.T) {
	prev := snap(1, 1.5, 26000, 20)
	p := DerivePairs(snap(2, 1.5, 26000.5, 20), &prev) // +0.002%: prints as 0.00%
	assert.True(t, Attribute(p[0], p[1], p[2]).Flat)
}

func Test_Hundredths_LegsAddUpAfterRounding(t *testing.T) {
	// Rounded independently: 0.13 + 0.13 = 0.26, but the total shows 0.25.
	total, base, vnd := Attribution{Total: 0.252, BaseLeg: 0.126, VNDLeg: 0.126}.Hundredths()
	assert.Equal(t, int64(25), total)
	assert.Equal(t, total, base+vnd)
}

func Test_History_DedupesCapsAndFindsPrevious(t *testing.T) {
	h := AppendSnapshot(nil, snap(100, 1.5, 26000, 20))
	h = AppendSnapshot(h, snap(100, 1.5, 26000, 20))
	assert.Len(t, h, 1, "the same publication is stored once")

	h = AppendSnapshot(h, snap(200, 1.5, 26100, 20))
	assert.Equal(t, int64(100), Previous(h, snap(200, 0, 0, 0)).PublishedAt,
		"a re-run on the latest publication compares against the one before")
	assert.Equal(t, int64(200), Previous(h, snap(300, 0, 0, 0)).PublishedAt)
	assert.Nil(t, Previous(h, snap(50, 0, 0, 0)))

	for i := int64(0); i < keepSnapshots+10; i++ {
		h = AppendSnapshot(h, snap(1000+i, 1.5, 26000, 20))
	}
	assert.Len(t, h, keepSnapshots)
	assert.Equal(t, int64(1000+keepSnapshots+9), h[len(h)-1].PublishedAt, "keeps the newest")
}

func Test_ValidateEntries(t *testing.T) {
	assert.Nil(t, ValidateEntries(nil))
	assert.Nil(t, ValidateEntries(map[string]float64{"AUD/VND": 18450}))
	assert.ErrorContains(t, ValidateEntries(map[string]float64{"AUDVND": 18450}), "not a reported pair")
	assert.ErrorContains(t, ValidateEntries(map[string]float64{"AUD/VND": 0}), "positive")
}

func Test_FormatNum(t *testing.T) {
	assert.Equal(t, "18,502.54", FormatNum(18502.54, 2))
	assert.Equal(t, "26,300", FormatNum(26300, 0))
	assert.Equal(t, "0.6536", FormatNum(0.65359, 4))
	assert.Equal(t, "1,223.4", FormatNum(1223.44, 1))
}

func Test_Redact_HidesTheKey(t *testing.T) {
	err := redact(errors.New(`Get "https://v6.exchangerate-api.com/v6/s3cr3t/latest/USD": timeout`), "s3cr3t")
	assert.NotContains(t, err.Error(), "s3cr3t")
	assert.Contains(t, err.Error(), "REDACTED")
}

func sgn(x float64) float64 {
	if math.Abs(x) < 1e-12 {
		return 0
	}
	return math.Copysign(1, x)
}

// visibleText is what a reader sees: tags dropped, entities decoded, whitespace collapsed.
// The golden test compares this rather than the HTML, so restyling the email doesn't

func Test_ParseRates_History(t *testing.T) {
	s, err := parseRates([]byte(`{"result":"success","year":2026,"month":9,"day":30,
		"conversion_rates":{"AUD":1.43,"VND":25922,"CZK":21.5}}`))
	assert.Nil(t, err)
	assert.Equal(t, time.Date(2026, 9, 30, 23, 0, 0, 0, time.UTC).Unix(), s.PublishedAt, "a history day is stamped at its close")
}

func Test_MergeDaily_LiveWinsTheDay(t *testing.T) {
	day := int64(86400)
	seed := []Snapshot{snap(1*day+82800, 1, 1, 1), snap(2*day+82800, 2, 2, 2)}
	live := []Snapshot{snap(2*day+79200, 9, 9, 9), snap(3*day+79200, 3, 3, 3)}
	h := mergeDaily(seed, live)
	assert.Len(t, h, 3)
	assert.Equal(t, 1.0, h[0].PerUSD["AUD"])
	assert.Equal(t, 9.0, h[1].PerUSD["AUD"], "the daily run's snapshot replaces the seed's on the same UTC day")
	assert.Equal(t, 3.0, h[2].PerUSD["AUD"])
}

func Test_PeriodStarts(t *testing.T) {
	wed := time.Date(2026, 9, 30, 22, 0, 0, 0, time.UTC).Unix()
	week, month, year := periodStarts(wed)
	assert.Equal(t, time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC).Unix(), week)
	assert.Equal(t, time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC).Unix(), month)
	assert.Equal(t, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Unix(), year)

	sun := time.Date(2026, 10, 4, 1, 0, 0, 0, time.UTC).Unix()
	week, _, _ = periodStarts(sun)
	assert.Equal(t, time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC).Unix(), week, "Sunday belongs to the week that began Monday")

	vnd := func(u map[string]float64) float64 { return u["VND"] }
	_, ok := periodChange([]Snapshot{snap(wed, 1, 26000, 1)}, snap(wed+86400, 1, 26100, 1), vnd, wed)
	assert.False(t, ok, "no publication before the boundary means no figure, not 0%")
}

func Test_Technicals(t *testing.T) {
	vnd := func(u map[string]float64) float64 { return u["VND"] }
	d := int64(86400)
	h := []Snapshot{snap(0, 1, 26500, 1), snap(320*d, 1, 26000, 1), snap(363*d, 1, 26100, 1), snap(364*d, 1, 26050, 1), snap(365*d, 1, 26200, 1)}
	tech := technicals(h, h[4], vnd)
	assert.Equal(t, 26050.0, tech.Low30)
	assert.Equal(t, 26200.0, tech.High30)
	assert.Equal(t, 26000.0, tech.Low52)
	assert.Equal(t, 26500.0, tech.High52)
	assert.True(t, tech.Has52)
	assert.Equal(t, 1, tech.Streak, "up today after a fall")
	assert.Equal(t, -1, streak(h, h[3], vnd))
	assert.Equal(t, 0.0, RangePosition(1, 1, 2))
	assert.Equal(t, 50.0, RangePosition(1, 1, 1), "a flat range doesn't divide by zero")
}
