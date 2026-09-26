package main

import (
	_ "embed"
	"fmt"
	"strings"
	"text/template"
	"time"
)

// Every word of the email lives in the template; Go supplies only formatted numbers and
// directions. Parsed at start-up, so a broken template fails before anything is sent.
//
//go:embed templates/email.tmpl
var emailTemplate string

var tmpl = template.Must(template.New("email").Parse(emailTemplate))

type pairView struct {
	Name   string
	Rate   string
	Change string // "" when there is no earlier publication
	Dir    int    // sign of the displayed change
	Entry  string // "" unless an entry rate was supplied for this pair
}

type legsView struct {
	Flat   bool
	AUD    string
	VND    string
	AUDDir int // +1 AUD/USD up (AUD stronger)
	VNDDir int // +1 USD/VND up (VND WEAKER): the template must word this, not the sign
}

type emailView struct {
	AsOf     string
	Since    string // "" on the first run
	Headline pairView
	Legs     *legsView // nil on the first run
	Others   []pairView
}

func buildView(cur Snapshot, prev *Snapshot, pairs []Pair, entries map[string]float64) emailView {
	views := make([]pairView, len(pairs))
	for i, p := range pairs {
		v := pairView{Name: p.Name, Rate: formatNum(p.Rate, p.Decimals)}
		if p.HasPrev() {
			h := toHundredths(p.Pct())
			v.Change, v.Dir = formatHundredths(h)+"%", sign(h)
		}
		if entry, ok := entries[p.Name]; ok {
			v.Entry = fmt.Sprintf("%s%% vs your entry %s",
				formatHundredths(toHundredths((p.Rate/entry-1)*100)), formatNum(entry, p.Decimals))
		}
		views[i] = v
	}

	view := emailView{AsOf: publishedDate(cur.PublishedAt), Headline: views[0], Others: views[1:]}
	if prev != nil {
		view.Since = publishedDate(prev.PublishedAt)
		attr := attribute(pairs[0], pairs[1], pairs[2])
		legs := legsView{Flat: attr.Flat}
		if !attr.Flat {
			_, aud, vnd := attr.hundredths()
			legs.AUD, legs.AUDDir = formatHundredths(aud), sign(aud)
			legs.VND, legs.VNDDir = formatHundredths(vnd), sign(vnd)
		}
		view.Legs = &legs
	}
	return view
}

func render(v emailView) (subject string, body string, err error) {
	var s, b strings.Builder
	if err := tmpl.ExecuteTemplate(&s, "subject", v); err != nil {
		return "", "", err
	}
	if err := tmpl.ExecuteTemplate(&b, "body", v); err != nil {
		return "", "", err
	}
	return strings.TrimSpace(s.String()), b.String(), nil
}

func publishedDate(unix int64) string { return time.Unix(unix, 0).UTC().Format("2 Jan 2006") }
