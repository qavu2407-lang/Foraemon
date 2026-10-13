package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"lorisocchipinti.com/gbp-rates/explain"
)

func Test_Metrics(t *testing.T) {
	facts := []fact{{Importance: 2, Covered: true}, {Importance: 0, Covered: false}, {Importance: 1, Covered: false}}
	claims := []claim{{Label: "SUPPORTED"}, {Label: "SUPPORTED"}, {Label: "CONTRADICTED"}, {Label: "NOT_SUPPORTED"}}
	prec, crate, rec, wrec, f1 := metrics(facts, claims)
	assert.Equal(t, 0.5, prec)
	assert.Equal(t, 0.25, crate)
	assert.InDelta(t, 1.0/3, rec, 1e-9)
	assert.InDelta(t, 2.0/3, wrec, 1e-9, "weighted: the covered fact carries 2 of 3")
	assert.InDelta(t, 2*0.5*(2.0/3)/(0.5+2.0/3), f1, 1e-9)

	_, _, _, _, f1 = metrics(nil, nil)
	assert.Equal(t, 0.0, f1, "an empty answer scores 0, not NaN")
}

func Test_LegNamed(t *testing.T) {
	in := json.RawMessage(`{"legs":[{"pair":"AUD/VND","AUD_leg_pp":"-0.05","VND_leg_pp":"+0.20"},
		{"pair":"CZK/VND","CZK_leg_pp":"-0.60","VND_leg_pp":"+0.10"}]}`)
	got := legNamed(in, explain.Explanation{Macro: []explain.Claim{{Currency: "AUD"}, {Currency: "VND"}}})
	assert.Equal(t, map[string]bool{"AUD/VND": true, "CZK/VND": false}, got, "VND drove AUD/VND; CZK drove CZK/VND but has no paragraph")

	got = legNamed(in, explain.Explanation{Macro: []explain.Claim{{Currency: "USD"}}})
	assert.Equal(t, map[string]bool{"AUD/VND": true, "CZK/VND": true}, got, "a broad dollar paragraph counts for either leg")
}

func Test_Chunk(t *testing.T) {
	got, err := chunk(`{"rates":[1],"headlines":[{"id":1},{"id":2},{"id":3}]}`, 2)
	assert.NoError(t, err)
	assert.Equal(t, []string{`{"rates":[1]}`, `{"headlines":[{"id":1},{"id":2}]}`, `{"headlines":[{"id":3}]}`}, got)
}

func Test_LoadFacts(t *testing.T) {
	file := filepath.Join(t.TempDir(), "facts.json")
	assert.NoError(t, os.WriteFile(file, []byte(`[{"text":"a","relevance":5,"salience":3,"importance":1.5}]`), 0o644))
	// A zero judge would panic on any model call: a saved file must be read, not re-extracted.
	got, err := loadFacts(context.Background(), judge{}, "{}", file)
	assert.NoError(t, err)
	assert.Equal(t, []fact{{Text: "a", Relevance: 5, Salience: 3, Importance: 1.5}}, got)
}

func Test_Audit(t *testing.T) {
	evidence := `{"rates":[{"rate":25123.50}],"headlines":[{"id":3,"summary":"Tỷ giá trung tâm\ntăng 10 đồng"}]}`
	claims := []claim{
		{Quote: "Tỷ giá trung tâm tăng 10 đồng", HeadlineIDs: []int{3}},
		{Quote: "25123.50"},
		{Quote: "Fed cut rates", HeadlineIDs: []int{3, 9}},
		{},
	}
	assert.NoError(t, audit(evidence, claims))
	assert.True(t, claims[0].QuoteFound, "escapes decoded, newline matched as a space")
	assert.True(t, claims[0].IDsValid)
	assert.True(t, claims[1].QuoteFound, "a number keeps its written form")
	assert.False(t, claims[2].QuoteFound, "a quote not in the evidence")
	assert.False(t, claims[2].IDsValid, "headline 9 was never sent")
	assert.False(t, claims[3].QuoteFound, "no quote is not a found quote")
	assert.True(t, claims[3].IDsValid)
}
