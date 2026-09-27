package main

import (
	_ "embed"
	"fmt"
	"html"
	"html/template"
	"strings"
	"time"
)

// Every word of the email lives in the template, an HTML email; Go supplies only formatted
// numbers and directions. html/template escapes every value it inserts. Parsed at
// start-up, so a broken template fails before anything is sent.
//
//go:embed templates/email.tmpl
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

type emailView struct {
	Count    int // days of stored history, including today
	AsOf     string
	Since    string // "" on the first run
	Sections []sectionView
}

// sectionView is one cross pair and its two legs: AUD/VND, AUD/USD, USD/VND.
// USD/VND is a leg of every cross, so it appears in each section.
type sectionView struct {
	Name  string
	Pairs []pairView // the cross pair first
}

func buildView(cur Snapshot, prev *Snapshot, pairs []Pair, entries map[string]float64, count int) emailView {
	byName := make(map[string]Pair, len(pairs))
	for _, p := range pairs {
		byName[p.Name] = p
	}

	views := make(map[string]pairView, len(pairs))
	for _, p := range pairs {
		v := pairView{Name: p.Name, Rate: formatNum(p.Rate, p.Decimals)}
		if p.HasPrev() {
			v.Change = formatHundredths(toHundredths(p.Pct())) + "%"
			if p.Split != "" {
				v.Legs = buildLegs(p.Split, attribute(p, byName[p.Split+"/USD"], byName["USD/VND"]))
			}
		}
		if entry, ok := entries[p.Name]; ok {
			v.Entry = fmt.Sprintf("%s%% vs your entry %s",
				formatHundredths(toHundredths((p.Rate/entry-1)*100)), formatNum(entry, p.Decimals))
		}
		views[p.Name] = v
	}

	view := emailView{Count: count, AsOf: publishedDate(cur.PublishedAt)}
	for _, p := range pairs {
		if p.Split != "" {
			view.Sections = append(view.Sections, sectionView{Name: p.Name, Pairs: []pairView{
				views[p.Name], views[p.Split+"/USD"], views["USD/VND"],
			}})
		}
	}
	if prev != nil {
		view.Since = publishedDate(prev.PublishedAt)
	}
	return view
}

func buildLegs(base string, attr Attribution) *legsView {
	legs := legsView{Flat: attr.Flat, Base: base}
	if !attr.Flat {
		_, leg, vnd := attr.hundredths()
		legs.Leg, legs.LegDir = formatHundredths(leg), sign(leg)
		legs.VND, legs.VNDDir = formatHundredths(vnd), sign(vnd)
	}
	return &legs
}

func render(v emailView) (subject string, body string, err error) {
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

func publishedDate(unix int64) string { return time.Unix(unix, 0).UTC().Format("2 Jan 2006") }
