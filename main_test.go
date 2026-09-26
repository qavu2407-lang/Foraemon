package main

import (
	"errors"
	"math"
	"testing"

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
	// The provider sends USD/AUD = 1.5, i.e. 1.5 AUD per dollar. AUD/USD is its reciprocal.
	pairs := derivePairs(snap(1, 1.5, 26000, 20), nil)

	byName := map[string]float64{}
	for _, p := range pairs {
		byName[p.Name] = p.Rate
		assert.False(t, p.HasPrev())
	}
	assert.Equal(t, "AUD/VND", pairs[0].Name, "headline pair first")
	assert.InDelta(t, 0.6667, byName["AUD/USD"], 0.0001)
	assert.InDelta(t, 26000, byName["USD/VND"], 0)
	assert.InDelta(t, 1300, byName["CZK/VND"], 0.0001)
	assert.InDelta(t, 17333.33, byName["AUD/VND"], 0.01)
	assert.InDelta(t, byName["AUD/USD"]*byName["USD/VND"], byName["AUD/VND"], 1e-9)
}

func Test_Attribute(t *testing.T) {
	cases := []struct {
		name           string
		prev, cur      Snapshot
		audDir, vndDir float64 // expected sign of each leg
	}{
		// USD/AUD down = AUD stronger; USD/VND up = VND weaker. Both push AUD/VND up.
		{"both legs push up", snap(1, 1.50, 26000, 20), snap(2, 1.49, 26100, 20), 1, 1},
		// AUD weaker pulls down, VND weaker pushes up.
		{"legs oppose", snap(1, 1.50, 26000, 20), snap(2, 1.52, 26100, 20), -1, 1},
		{"only VND moved", snap(1, 1.50, 26000, 20), snap(2, 1.50, 26100, 20), 0, 1},
	}
	for _, c := range cases {
		prev := c.prev
		p := derivePairs(c.cur, &prev)
		a := attribute(p[0], p[1], p[2])

		assert.False(t, a.Flat, c.name)
		assert.InDelta(t, a.Total, a.AUDLeg+a.VNDLeg, 1e-9, "%s: legs must sum to the total", c.name)
		assert.Equal(t, c.audDir, sgn(a.AUDLeg), c.name)
		assert.Equal(t, c.vndDir, sgn(a.VNDLeg), c.name)
	}
}

func Test_Attribute_NearZeroIsFlat(t *testing.T) {
	prev := snap(1, 1.5, 26000, 20)
	p := derivePairs(snap(2, 1.5, 26000.5, 20), &prev) // +0.002%: prints as 0.00%
	assert.True(t, attribute(p[0], p[1], p[2]).Flat)
}

func Test_Hundredths_LegsAddUpAfterRounding(t *testing.T) {
	// Rounded independently: 0.13 + 0.13 = 0.26, but the total shows 0.25.
	total, aud, vnd := Attribution{Total: 0.252, AUDLeg: 0.126, VNDLeg: 0.126}.hundredths()
	assert.Equal(t, int64(25), total)
	assert.Equal(t, total, aud+vnd)
}

func Test_History_DedupesCapsAndFindsPrevious(t *testing.T) {
	h := appendSnapshot(nil, snap(100, 1.5, 26000, 20))
	h = appendSnapshot(h, snap(100, 1.5, 26000, 20))
	assert.Len(t, h, 1, "the same publication is stored once")

	h = appendSnapshot(h, snap(200, 1.5, 26100, 20))
	assert.Equal(t, int64(100), previous(h, snap(200, 0, 0, 0)).PublishedAt,
		"a re-run on the latest publication compares against the one before")
	assert.Equal(t, int64(200), previous(h, snap(300, 0, 0, 0)).PublishedAt)
	assert.Nil(t, previous(h, snap(50, 0, 0, 0)))

	for i := int64(0); i < keepSnapshots+10; i++ {
		h = appendSnapshot(h, snap(1000+i, 1.5, 26000, 20))
	}
	assert.Len(t, h, keepSnapshots)
	assert.Equal(t, int64(1000+keepSnapshots+9), h[len(h)-1].PublishedAt, "keeps the newest")
}

func Test_ValidateEntries(t *testing.T) {
	assert.Nil(t, validateEntries(nil))
	assert.Nil(t, validateEntries(map[string]float64{"AUD/VND": 18450}))
	assert.ErrorContains(t, validateEntries(map[string]float64{"AUDVND": 18450}), "not a reported pair")
	assert.ErrorContains(t, validateEntries(map[string]float64{"AUD/VND": 0}), "positive")
}

func Test_FormatNum(t *testing.T) {
	assert.Equal(t, "18,502.54", formatNum(18502.54, 2))
	assert.Equal(t, "26,300", formatNum(26300, 0))
	assert.Equal(t, "0.6536", formatNum(0.65359, 4))
	assert.Equal(t, "1,223.4", formatNum(1223.44, 1))
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

// The email is the product, so pin it whole: a wording change that makes the VND sign
// ambiguous again, or a column that stops aligning, fails here.
func Test_Render_Golden(t *testing.T) {
	prev := snap(1790294400, 1.4210, 26280, 21.40)
	cur := snap(1790380800, 1.4400, 26263, 21.35)
	pairs := derivePairs(cur, &prev)

	subject, body, err := render(buildView(cur, &prev, pairs, map[string]float64{"AUD/VND": 18450}))
	assert.Nil(t, err)
	assert.Equal(t, "🔴 AUD/VND 18,238.19 (-1.38%)", subject)
	assert.Equal(t, `Good morning, bee ready to make money today! 🐝 💸 🐝

Rates published 26 Sep 2026, change since 25 Sep 2026.

AUD/VND   18,238.19   -1.38%
  AUD leg  -1.32pp  AUD weaker vs USD
  VND leg  -0.06pp  VND stronger vs USD
         -1.15% vs your entry 18,450.00

AUD/USD      0.6944   -1.32%
USD/VND      26,263   -0.06%
CZK/VND     1,230.1   +0.17%

Rates are mid-market. What you receive on a transfer is lower by the provider's fee.
`, body)
}

func Test_Render_FirstRun(t *testing.T) {
	cur := snap(1790380800, 1.4400, 26263, 21.35)
	subject, body, err := render(buildView(cur, nil, derivePairs(cur, nil), nil))
	assert.Nil(t, err)
	assert.Equal(t, "AUD/VND 18,238.19", subject, "no change and no colour without history")
	assert.Contains(t, body, "First run")
	assert.NotContains(t, body, "leg")
}
