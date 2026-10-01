package email

import (
	_ "embed"
	"fmt"
	"html"
	"html/template"
	"math"
	"strings"

	"lorisocchipinti.com/gbp-rates/explain"
	"lorisocchipinti.com/gbp-rates/rates"
	"lorisocchipinti.com/gbp-rates/tools"
)

// Every word of the email lives in the template, an HTML email; Go supplies only formatted
// numbers and directions. html/template escapes every value it inserts. Parsed at
// start-up, so a broken template fails before anything is sent.
//
//go:embed email.tmpl
var emailTemplate string

var tmpl = template.Must(template.New("email").Parse(emailTemplate))

type pairView struct {
	Name   string
	Rate   string
	Change string    // "" when there is no earlier publication
	Legs   *legsView // nil unless this is a cross pair and there is history
	Entry  string    // "" unless an entry rate was supplied for this pair
}

type legsView struct {
	Flat   bool
	Base   string // "AUD", "CZK"
	Leg    string
	VND    string
	LegDir int // +1 BASE/USD up (BASE stronger)
	VNDDir int // +1 USD/VND up (VND WEAKER): the template must word this, not the sign
}

type View struct {
	Date     string // the publication's date, for the greeting
	AsOf     string // the publication's date and time
	Since    string // the previous publication's date and time, "" on the first run
	Sections []sectionView

	Headlines []claimView
	Macro     []claimView
	Scenarios []explain.Scenario
	NoExplain string // why there is no explanation, "" when there is one

	Citations []citationView // the headlines cited anywhere, by id

	Rates      []rateView
	Technicals []techView
	SBV        *sbvView
	Calendar   []eventView
	SBVLink    string
}

type claimView struct {
	Currency string
	Text     string
	Cites    []citeView
}

type citeView struct {
	N   int
	URL string
}

type citationView struct {
	N                           int
	Publisher, Date, Title, URL string
}

// rateView is one row of the FX market rates table: rates and signed % strings, or "—".
type rateView struct{ Name, Today, Last, WTD, MTD, YTD string }

type techView struct {
	Name      string
	Range30   string
	Pos30     string // where today sits in the 30-day range, "0%" at the low
	Range52   string // "" when the history is shorter than a year
	Pos52     string
	Streak    int // publications in a row
	StreakDir int // +1 up, -1 down, 0 flat
}

type sbvView struct {
	Date, Central, Market, Gap string
	Above                      bool
	Stale                      bool
}

type eventView struct{ When, Currency, Title, Impact, Forecast, Previous string }

// sectionView is one cross pair and its two legs: AUD/VND, AUD/USD, USD/VND.
// USD/VND is a leg of every cross, so it appears in each section.
type sectionView struct {
	Name  string
	Pairs []pairView // the cross pair first
}

func Build(cur rates.Snapshot, prev *rates.Snapshot, pairs []rates.Pair, entries map[string]float64, rows []rates.PeriodRow, b explain.Brief) View {
	byName := make(map[string]rates.Pair, len(pairs))
	for _, p := range pairs {
		byName[p.Name] = p
	}

	views := make(map[string]pairView, len(pairs))
	for _, p := range pairs {
		v := pairView{Name: p.Name, Rate: rates.FormatNum(p.Rate, p.Decimals)}
		if p.HasPrev() {
			v.Change = rates.FormatHundredths(rates.ToHundredths(p.Pct())) + "%"
			if p.Split != "" {
				v.Legs = buildLegs(p.Split, rates.Attribute(p, byName[p.Split+"/USD"], byName["USD/VND"]))
			}
		}
		if entry, ok := entries[p.Name]; ok {
			v.Entry = fmt.Sprintf("%s%% vs your entry %s",
				rates.FormatHundredths(rates.ToHundredths((p.Rate/entry-1)*100)), rates.FormatNum(entry, p.Decimals))
		}
		views[p.Name] = v
	}

	view := View{Date: rates.PublishedDate(cur.PublishedAt), AsOf: rates.PublishedTime(cur.PublishedAt), SBVLink: tools.SBVAttribution}
	for _, p := range pairs {
		if p.Split != "" {
			view.Sections = append(view.Sections, sectionView{Name: p.Name, Pairs: []pairView{
				views[p.Name], views[p.Split+"/USD"], views["USD/VND"],
			}})
		}
	}
	if prev != nil {
		view.Since = rates.PublishedTime(prev.PublishedAt)
	}
	addRows(&view, rows)
	addBrief(&view, cur, b)
	return view
}

func addRows(view *View, rows []rates.PeriodRow) {
	for _, r := range rows {
		p, t := r.Pair, r.Tech
		rv := rateView{Name: p.Name, Today: rates.FormatNum(p.Rate, p.Decimals), Last: "—",
			WTD: rates.PctOrDash(r.WTD), MTD: rates.PctOrDash(r.MTD), YTD: rates.PctOrDash(r.YTD)}
		if p.HasPrev() {
			rv.Last = rates.FormatNum(p.Prev, p.Decimals)
		}
		view.Rates = append(view.Rates, rv)

		tv := techView{Name: p.Name, Range30: rates.FormatRange(t.Low30, t.High30, p.Decimals),
			Pos30: fmt.Sprintf("%.0f%%", rates.RangePosition(p.Rate, t.Low30, t.High30)), Streak: t.Streak, StreakDir: 1}
		if t.Streak < 0 {
			tv.Streak, tv.StreakDir = -t.Streak, -1
		} else if t.Streak == 0 {
			tv.StreakDir = 0
		}
		if t.Has52 {
			tv.Range52 = rates.FormatRange(t.Low52, t.High52, p.Decimals)
			tv.Pos52 = fmt.Sprintf("%.0f%%", rates.RangePosition(p.Rate, t.Low52, t.High52))
		}
		view.Technicals = append(view.Technicals, tv)
	}
}

func addBrief(view *View, cur rates.Snapshot, b explain.Brief) {
	view.NoExplain = b.NoExplain
	if e := b.Explanation; e != nil {
		claims := func(cs []explain.Claim) []claimView {
			var out []claimView
			for _, c := range cs {
				cv := claimView{Currency: c.Currency, Text: c.Text}
				for _, id := range c.Cites { // checked: 1 <= id <= len(b.News)
					cv.Cites = append(cv.Cites, citeView{N: id, URL: b.News[id-1].URL})
				}
				out = append(out, cv)
			}
			return out
		}
		view.Headlines, view.Macro = claims(e.Headlines), claims(e.Macro)
		cited := map[int]bool{}
		for _, c := range append(append([]explain.Claim(nil), e.Headlines...), e.Macro...) {
			for _, id := range c.Cites {
				cited[id] = true
			}
		}
		for id, h := range b.News {
			if cited[id+1] {
				view.Citations = append(view.Citations, citationView{N: id + 1, Publisher: h.Publisher,
					Date: h.Published.In(rates.ICT).Format("2 Jan"), Title: h.Title, URL: h.URL})
			}
		}
		view.Scenarios = e.Scenarios
	}
	if s := b.SBV; s != nil {
		gap := (cur.PerUSD["VND"]/s.USDVND - 1) * 100
		view.SBV = &sbvView{Date: s.Date, Central: rates.FormatNum(s.USDVND, 0), Market: rates.FormatNum(cur.PerUSD["VND"], 0),
			Gap: fmt.Sprintf("%.2f%%", math.Abs(gap)), Above: gap > 0, Stale: s.Stale}
	}
	for _, e := range b.Calendar {
		view.Calendar = append(view.Calendar, eventView{When: e.Time.In(rates.ICT).Format("Mon 2 Jan 15:04"),
			Currency: e.Country, Title: e.Title, Impact: e.Impact, Forecast: e.Forecast, Previous: e.Previous})
	}
}

func buildLegs(base string, attr rates.Attribution) *legsView {
	legs := legsView{Flat: attr.Flat, Base: base}
	if !attr.Flat {
		_, leg, vnd := attr.Hundredths()
		legs.Leg, legs.LegDir = rates.FormatHundredths(leg), rates.Sign(leg)
		legs.VND, legs.VNDDir = rates.FormatHundredths(vnd), rates.Sign(vnd)
	}
	return &legs
}

func Render(v View) (subject string, body string, err error) {
	// The subject block goes through the same HTML escaping as the body, but a mail
	// header is plain text, so undo it: "&#43;0.17%" must arrive as "+0.17%".
	var s, b strings.Builder
	if err := tmpl.ExecuteTemplate(&s, "subject", v); err != nil {
		return "", "", err
	}
	if err := tmpl.ExecuteTemplate(&b, "body", v); err != nil {
		return "", "", err
	}
	return html.UnescapeString(strings.TrimSpace(s.String())), b.String(), nil
}
