package tools

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"lorisocchipinti.com/gbp-rates/clean"
)

// Headline is one news item. Its text is untrusted: it reaches the model as data only.
type Headline struct {
	Source    string // the feed it came from, e.g. "Google News: AUD"
	Publisher string // who published it, e.g. "FXStreet": named in the email's text
	Author    string // "" when the feed doesn't say
	Title     string
	Summary   string
	URL       string
	Published time.Time
	// The feed's text before clean.Text, kept so a changed cleaning rule can be re-run
	// over old run records.
	RawTitle, RawSummary string
}

type feed struct{ source, url string }

// Feeds is a fixed allowlist (all checked live 2026-10-01). The CNB has no RSS feed and
// English koruna searches return converter pages, so the koruna comes from Czech-language
// FX news. EUR/USD covers the broad dollar, which the koruna largely follows via the euro.
var Feeds = []feed{
	{"VnEconomy", "https://vneconomy.vn/tai-chinh.rss"},
	{"CafeF", "https://cafef.vn/tai-chinh-ngan-hang.rss"},
	{"VnExpress", "https://vnexpress.net/rss/kinh-doanh.rss"},
	{"RBA", "https://www.rba.gov.au/rss/rss-cb-media-releases.xml"},
	{"Federal Reserve", "https://www.federalreserve.gov/feeds/press_all.xml"},
	{"Google News: AUD", "https://news.google.com/rss/search?q=%22Australian+dollar%22+when:2d&hl=en-US&gl=US&ceid=US:en"},
	{"Google News: koruna (cs)", "https://news.google.com/rss/search?q=koruna+kurz+when:2d&hl=cs&gl=CZ&ceid=CZ:cs"},
	{"Google News: EUR/USD", "https://news.google.com/rss/search?q=%22EUR%2FUSD%22+when:1d&hl=en-US&gl=US&ceid=US:en"},
	{"Google News: tỷ giá", "https://news.google.com/rss/search?q=%22t%E1%BB%B7+gi%C3%A1%22+when:2d&hl=vi&gl=VN&ceid=VN:vi"},
}

// FetchNews returns up to perFeed of each feed's items published after since, newest
// first, and what cleaning found in every item fetched. A failing feed is reported in errs
// and skipped. The same story from two feeds is kept once.
func FetchNews(ctx context.Context, since time.Time, perFeed int) (news []Headline, rep clean.Report, errs []error) {
	seen := map[string]bool{}
	for _, f := range Feeds {
		data, err := get(ctx, f.url)
		if err == nil {
			var items []Headline
			items, err = parseFeed(data, f.source, &rep)
			n := 0
			for _, h := range items {
				if n == perFeed || h.Published.Before(since) {
					continue
				}
				if seen[clean.Key(h.Title)] {
					rep.Duplicates++
					continue
				}
				seen[clean.Key(h.Title)] = true
				news = append(news, h)
				n++
			}
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", f.source, err))
		}
	}
	return news, rep, errs
}

// parseFeed reads RSS 2.0 and RSS 1.0 (RDF, the RBA's format) alike: both are a list
// of <item> elements, nested differently. Items without a parseable date are dropped,
// because their age is unknown. Sorted newest first.
func parseFeed(data []byte, source string, rep *clean.Report) ([]Headline, error) {
	var items []Headline
	undated := 0
	dec := xml.NewDecoder(bytes.NewReader(data))
	for {
		tok, err := dec.Token()
		if err != nil {
			if errors.Is(err, io.EOF) || len(items) > 0 {
				break // a truncated feed still yields the items before the break
			}
			return nil, err
		}
		start, ok := tok.(xml.StartElement)
		if !ok || start.Name.Local != "item" {
			continue
		}
		var it struct {
			Title       string `xml:"title"`
			Link        string `xml:"link"`
			Description string `xml:"description"`
			Publisher   string `xml:"source"`  // Google News: the outlet behind the link
			Author      string `xml:"creator"` // dc:creator
			PubDate     string `xml:"pubDate"`
			Date        string `xml:"date"` // dc:date in RSS 1.0
		}
		if err := dec.DecodeElement(&it, &start); err != nil {
			return nil, err
		}
		published, ok := parseDate(it.PubDate + it.Date)
		title := clean.Text(it.Title, 200, rep)
		if !ok || title == "" {
			undated++
			continue
		}
		// Google News links are redirects that some regions can't open, so the outlet's
		// name has to travel with the claim. Its titles end in " - Outlet": drop that.
		publisher := clean.Text(it.Publisher, 80, rep)
		if publisher == "" {
			publisher = source
		}
		title = strings.TrimSuffix(title, " - "+publisher)
		items = append(items, Headline{Source: source, Publisher: publisher, Author: clean.Text(it.Author, 80, rep),
			Title: title, Summary: clean.Text(it.Description, 300, rep), URL: strings.TrimSpace(it.Link), Published: published,
			RawTitle: it.Title, RawSummary: it.Description})
	}
	// A feed whose every item is skipped has changed format; say so instead of going quiet.
	if len(items) == 0 && undated > 0 {
		return nil, fmt.Errorf("none of %d items has a readable date", undated)
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].Published.After(items[j].Published) })
	return items, nil
}

func parseDate(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	// The last layout is CafeF's two-digit year: "Thu, 01 Oct 26 18:25:33 +0700".
	for _, layout := range []string{time.RFC1123Z, time.RFC1123, time.RFC3339, "Mon, 2 Jan 2006 15:04:05 -0700", "Mon, 02 Jan 06 15:04:05 -0700"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}
