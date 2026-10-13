package clean

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_Text(t *testing.T) {
	var r Report
	assert.Equal(t, "Tỷ giá & lãi suất", Text("&lt;p&gt;Tỷ giá &amp;amp; <b>lãi</b>   suất&lt;/p&gt;", 100, &r),
		"markup and double-escaped entities, then whitespace")

	// An attack headline: a zero-width space splitting a word, a right-to-left override,
	// a Cyrillic е in "Fed" and a full-width "A".
	got := Text("The Fеd cut​ rates‮ Ａgain", 100, &r)
	assert.Equal(t, "The Fеd cut rates Again", got, "invisible characters dropped, full-width folded, Cyrillic kept")
	assert.Equal(t, 2, r.Invisible)
	assert.Equal(t, []string{"Fеd"}, r.MixedScript, "flagged, not rewritten")

	// "giá" typed as g + i + a + combining acute matches the precomposed one.
	assert.Equal(t, Text("Tỷ giá", 100, nil), Text("Tỷ giá", 100, nil))

	assert.Equal(t, "abc…", Text("abcdef", 3, &r))
	assert.Equal(t, 1, r.Truncated)
	assert.Equal(t, "Line one two", Text("Line\none\ttwo", 100, nil), "newlines and tabs are spaces, not invisible")
}

func Test_Key(t *testing.T) {
	assert.Equal(t, Key("Giá USD trong nước tăng"), Key("  giá USD trong nước tăng!"))
	assert.Equal(t, "eur usd trades 1 1260", Key("EUR/USD trades 1.1260"))
	assert.NotEqual(t, Key("Fed holds rates"), Key("Fed cuts rates"))
}
