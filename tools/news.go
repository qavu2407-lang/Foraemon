package tools

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"io"
	"regexp"
	"sort"
	"strings"
	"time"
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
// first. A failing feed is reported in errs and skipped.
func FetchNews(ctx context.Context, since time.Time, perFeed int) (news []Headline, errs []error) {
	seen := map[string]bool{}
	for _, f := range Feeds {
		data, err := get(ctx, f.url)
		if err == nil {
			var items []Headline
			items, err = parseFeed(data, f.source)
			n := 0
			for _, h := range items {
				if n == perFeed || h.Published.Before(since) || seen[h.Title] {
					continue
				}
				seen[h.Title] = true
				news = append(news, h)
				n++
			}
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", f.source, err))
		}
	}
	return news, errs
}

// parseFeed reads RSS 2.0 and RSS 1.0 (RDF, the RBA's format) alike: both are a list
// of <item> elements, nested differently. Items without a parseable date are dropped,
// because their age is unknown. Sorted newest first.
func parseFeed(data []byte, source string) ([]Headline, error) {
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
		title := clean(it.Title, 200)
		if !ok || title == "" {
			undated++
			continue
		}
		// Google News links are redirects that some regions can't open, so the outlet's
		// name has to travel with the claim. Its titles end in " - Outlet": drop that.
		publisher := clean(it.Publisher, 80)
		if publisher == "" {
			publisher = source
		}
		title = strings.TrimSuffix(title, " - "+publisher)
		items = append(items, Headline{Source: source, Publisher: publisher, Author: clean(it.Author, 80),
			Title: title, Summary: clean(it.Description, 300), URL: strings.TrimSpace(it.Link), Published: published})
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

var tags = regexp.MustCompile(`<[^>]*>`)

// clean drops markup, decodes entities, collapses whitespace and truncates to max runes.
func clean(s string, max int) string {
	s = strings.Join(strings.Fields(html.UnescapeString(tags.ReplaceAllString(html.UnescapeString(s), " "))), " ")
	if r := []rune(s); len(r) > max {
		s = string(r[:max]) + "…"
	}
	return s
}
