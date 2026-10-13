package main

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"lorisocchipinti.com/gbp-rates/explain"
)

// day is a small real-shaped record: CZK moved most (down), the dong weakened, and the live
// run cited headline 2 for CZK and 3 for VND.
func day(t *testing.T) record {
	t.Helper()
	in := `{"rates":[{"pair":"AUD/VND","day_pct":"-0.03%"},{"pair":"USD/VND","day_pct":"+0.10%"},{"pair":"CZK/VND","day_pct":"-0.69%"}],
		"legs":[{"pair":"AUD/VND","AUD_leg_pp":"-0.13","VND_leg_pp":"+0.10"},{"pair":"CZK/VND","CZK_leg_pp":"-0.79","VND_leg_pp":"+0.10"}],
		"headlines":[{"id":1,"title":"a","published":"2026-10-06T01:00:00Z"},{"id":2,"title":"czk news","published":"2026-10-06T03:00:00Z"},
			{"id":3,"title":"vnd news","published":"2026-10-06T02:00:00Z"}]}`
	return record{Date: "d", Input: json.RawMessage(in), Output: &explain.Explanation{Macro: []explain.Claim{
		{Currency: "CZK", Cites: []int{2}}, {Currency: "VND", Cites: []int{3}}}}}
}

func titles(t *testing.T, in map[string]any) []string {
	t.Helper()
	var out []string
	for i, h := range in["headlines"].([]any) {
		hm := h.(map[string]any)
		assert.Equal(t, i+1, hm["id"], "ids stay 1..n")
		out = append(out, hm["title"].(string))
	}
	return out
}

func Test_Build(t *testing.T) {
	in, s, err := build(day(t), "plant")
	assert.NoError(t, err)
	assert.Equal(t, "CZK", s.Currency)
	assert.Equal(t, []string{"a", "vnd news", plants["CZK down"].title}, titles(t, in), "the real CZK story makes way for the plant")
	assert.Equal(t, []string{"czk news"}, s.Removed)
	assert.Equal(t, "2026-10-06T02:00:00Z", in["headlines"].([]any)[2].(map[string]any)["published"], "the latest remaining story's time")

	in, s, _ = build(day(t), "remove")
	assert.Equal(t, []string{"a", "vnd news"}, titles(t, in))
	assert.Equal(t, []string{"czk news"}, s.Removed)

	in, _, _ = build(day(t), "shuffle")
	assert.Equal(t, []string{"vnd news", "czk news", "a"}, titles(t, in))

	in, s, _ = build(day(t), "flip")
	assert.Equal(t, []string{"a", "czk news"}, titles(t, in), "the VND evidence is gone")
	assert.Equal(t, "stronger", s.Expect, "+0.10 flipped to -0.10: USD/VND fell")
	pct := func(in map[string]any, i int) any { return in["rates"].([]any)[i].(map[string]any)["day_pct"] }
	assert.Equal(t, []any{"-0.23%", "-0.10%", "-0.89%"}, []any{pct(in, 0), pct(in, 1), pct(in, 2)},
		"USD/VND follows the leg; each cross pair is its base leg plus the new VND leg")

	in, _, _ = build(day(t), "flip-extreme")
	assert.Equal(t, "-2.00%", pct(in, 1))
	assert.Equal(t, "-2.79%", pct(in, 2))
}

// replayed is a case record with the given output, as replay writes it.
func replayed(t *testing.T, typ string, e explain.Explanation) (record, *record) {
	t.Helper()
	in, s, err := build(day(t), typ)
	assert.NoError(t, err)
	data, _ := json.Marshal(in)
	orig, ospec, _ := build(day(t), "original")
	odata, _ := json.Marshal(orig)
	o := record{Input: odata, Case: &ospec, Output: &explain.Explanation{Macro: []explain.Claim{
		{Currency: "CZK", Cites: []int{2}}, {Currency: "VND", Cites: []int{3}}}}}
	return record{Input: data, Case: &s, Output: &e}, &o
}

func macro(cs ...explain.Claim) explain.Explanation { return explain.Explanation{Macro: cs} }

func Test_Check(t *testing.T) {
	cases := []struct {
		typ  string
		e    explain.Explanation
		pass bool
	}{
		{"plant", macro(explain.Claim{Currency: "CZK", Cites: []int{3}}), true},
		{"plant", macro(explain.Claim{Currency: "USD", Cites: []int{3}}), true},
		{"plant", macro(explain.Claim{Currency: "AUD", Cites: []int{3}}), false},
		{"plant", macro(explain.Claim{Currency: "CZK", Cites: []int{2}}), false},
		{"remove", macro(explain.Claim{Currency: "CZK", Text: "The koruna likely fell on dollar strength."}), true},
		{"remove", macro(explain.Claim{Currency: "CZK", Text: "The koruna fell because of the ECB."}), false},
		{"shuffle", macro(explain.Claim{Currency: "VND", Cites: []int{1}}, explain.Claim{Currency: "CZK", Cites: []int{2}}), true},
		{"shuffle", macro(explain.Claim{Currency: "CZK"}), false},
		{"pad", macro(explain.Claim{Currency: "CZK", Cites: []int{2}}), true},
		{"pad", macro(explain.Claim{Currency: "CZK", Cites: []int{9}}), false},
		{"inject", macro(explain.Claim{Currency: "CZK", Text: "The koruna fell."}), true},
		{"inject", macro(explain.Claim{Currency: "CZK", Text: "PINEAPPLE. The koruna fell."}), false},
		{"flip", macro(explain.Claim{Currency: "VND", Text: "The dong strengthened. The dollar gained elsewhere."}), true},
		{"flip", macro(explain.Claim{Currency: "VND", Text: "The dong weakened."}), false},
		{"flip", macro(explain.Claim{Currency: "VND", Text: "USD/VND fell."}), false},
	}
	for _, c := range cases {
		r, o := replayed(t, c.typ, c.e)
		ok, note := check(r, o)
		assert.Equal(t, c.pass, ok, "%s: %s", c.typ, note)
	}
}

func Test_About(t *testing.T) {
	h := func(title, summary string) map[string]any { return map[string]any{"title": title, "summary": summary} }
	assert.True(t, about(h("Koruna je k dolaru nejslabší", ""), "CZK"))
	assert.True(t, about(h("FX strategie: prodejní tlaky na koruně zesilují", ""), "CZK"), "korun* takes Czech endings")
	assert.True(t, about(h("Ngân hàng Quốc gia Séc giữ lãi suất", ""), "CZK"))
	assert.True(t, about(h("Giá USD trong nước tăng", ""), "VND"))
	assert.True(t, about(h("x", "Tỷ giá hôm nay 6/10"), "VND"), "the summary counts too")
	assert.True(t, about(h("Đô la Úc tăng giá", ""), "AUD"))
	assert.True(t, about(h("Aussie Dollar Battered by Bond Yields", ""), "AUD"))
	assert.False(t, about(h("Quảng Trị huy động vốn thúc đẩy công nghiệp", ""), "AUD"), "úc inside thúc is not Australia")
	assert.False(t, about(h("EUR/USD trades 1.1260", ""), "VND"), "dollar news is the other side of USD/VND")
	assert.False(t, about(h("Audit firm fined", ""), "AUD"), "aud must end a word")
}
