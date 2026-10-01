package tools

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func Test_ParseFeed_RSS2AndRDF(t *testing.T) {
	rss2 := `<rss><channel><item><title>Tỷ giá &amp; lãi suất</title><link>https://a/1</link>
		<description>&lt;p&gt;Dong &lt;b&gt;steady&lt;/b&gt;&lt;/p&gt;</description><pubDate>Thu, 01 Oct 26 18:25:33 +0700</pubDate></item>
		<item><title>Older</title><link>https://a/0</link><pubDate>Wed, 30 Sep 2026 08:00:00 +0000</pubDate></item>
		<item><title>No date</title></item></channel></rss>`
	items, err := parseFeed([]byte(rss2), "CafeF")
	assert.Nil(t, err)
	assert.Len(t, items, 2, "the undated item is dropped")
	assert.Equal(t, "Tỷ giá & lãi suất", items[0].Title, "newest first, entities decoded")
	assert.Equal(t, "Dong steady", items[0].Summary, "markup stripped")
	assert.Equal(t, 2026, items[0].Published.Year(), "two-digit years are read")

	rdf := `<rdf:RDF xmlns:rdf="r" xmlns:dc="d" xmlns="http://purl.org/rss/1.0/"><channel/>
		<item><title>RBA release</title><link>https://rba/1</link><dc:date>2026-10-01T11:30:00+10:00</dc:date></item></rdf:RDF>`
	items, err = parseFeed([]byte(rdf), "RBA")
	assert.Nil(t, err)
	assert.Equal(t, []Headline{{Source: "RBA", Title: "RBA release", URL: "https://rba/1",
		Published: time.Date(2026, 10, 1, 1, 30, 0, 0, time.UTC)}}, []Headline{{Source: items[0].Source,
		Title: items[0].Title, URL: items[0].URL, Published: items[0].Published.UTC()}})

	_, err = parseFeed([]byte(`<rss><item><title>x</title><pubDate>someday</pubDate></item></rss>`), "X")
	assert.ErrorContains(t, err, "readable date", "a feed that changed its date format must not go quiet")
}

func Test_ParseFeed_Publisher(t *testing.T) {
	gn := `<rss><item><title>EUR/USD stays under pressure - FXStreet</title><link>https://news.google.com/x</link>
		<pubDate>Thu, 01 Oct 2026 08:00:00 GMT</pubDate><source url="https://www.fxstreet.com">FXStreet</source></item></rss>`
	items, err := parseFeed([]byte(gn), "Google News: EUR/USD")
	assert.Nil(t, err)
	assert.Equal(t, "FXStreet", items[0].Publisher, "the outlet, not the aggregator")
	assert.Equal(t, "EUR/USD stays under pressure", items[0].Title, "the outlet suffix is dropped")

	direct := `<rss><item><title>Fed holds</title><pubDate>Thu, 01 Oct 2026 08:00:00 GMT</pubDate><dc:creator>J. Smith</dc:creator></item></rss>`
	items, err = parseFeed([]byte(direct), "Federal Reserve")
	assert.Nil(t, err)
	assert.Equal(t, "Federal Reserve", items[0].Publisher, "a direct feed is its own publisher")
	assert.Equal(t, "J. Smith", items[0].Author)
}

func Test_ParseCalendar_Filters(t *testing.T) {
	data := `[{"title":"NFP","country":"USD","date":"2026-10-02T08:30:00-04:00","impact":"High","forecast":"89K","previous":"162K"},
		{"title":"Speech","country":"USD","date":"2026-10-01T09:00:00-04:00","impact":"Low"},
		{"title":"BoJ","country":"JPY","date":"2026-10-01T09:00:00-04:00","impact":"High"},
		{"title":"CPI","country":"EUR","date":"2026-10-01T05:00:00-04:00","impact":"Medium"},
		{"title":"Late","country":"AUD","date":"2026-10-05T05:00:00-04:00","impact":"High"}]`
	from := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	events, err := parseCalendar([]byte(data), from, from.Add(48*time.Hour), []string{"AUD", "USD", "EUR"})
	assert.Nil(t, err)
	assert.Len(t, events, 2, "low impact, other currencies and events past the window are dropped")
	assert.Equal(t, "CPI", events[0].Title, "oldest first")
	assert.Equal(t, "NFP", events[1].Title)
}

func Test_ParseCentralRate(t *testing.T) {
	r, err := parseCentralRate([]byte(`{"rate_date":"2026-10-01","stale":false,"rates":[
		{"base":"USD","quote":"VND","type":"buy","value":24393},
		{"base":"USD","quote":"VND","type":"reference","value":25624}]}`))
	assert.Nil(t, err)
	assert.Equal(t, CentralRate{Date: "2026-10-01", USDVND: 25624}, r, "the reference rate, not a dealing rate")

	_, err = parseCentralRate([]byte(`{"rates":[]}`))
	assert.Error(t, err)
}
