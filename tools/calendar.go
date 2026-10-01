package tools

import (
	"context"
	"encoding/json"
	"sort"
	"time"
)

// Event is one scheduled release from the ForexFactory calendar.
type Event struct {
	Time     time.Time
	Country  string // the currency it moves: AUD, USD, EUR...
	Title    string
	Impact   string // High or Medium
	Forecast string
	Previous string
}

const calendarURL = "https://nfs.faireconomy.media/ff_calendar_thisweek.json"

// FetchCalendar returns the High and Medium impact events for the given currencies in
// [from, to), oldest first. The feed has no CZK or VND: the koruna mostly follows the
// ECB, so callers pass EUR for it.
//
// ponytail: this-week feed only, so a Friday or Saturday run sees nothing for next
// Monday. Add ff_calendar_nextweek.json if weekend emails need it.
func FetchCalendar(ctx context.Context, from, to time.Time, currencies []string) ([]Event, error) {
	data, err := get(ctx, calendarURL)
	if err != nil {
		return nil, err
	}
	return parseCalendar(data, from, to, currencies)
}

func parseCalendar(data []byte, from, to time.Time, currencies []string) ([]Event, error) {
	var raw []struct {
		Title, Country, Date, Impact, Forecast, Previous string
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	want := map[string]bool{}
	for _, c := range currencies {
		want[c] = true
	}
	var events []Event
	for _, r := range raw {
		t, err := time.Parse(time.RFC3339, r.Date)
		if err != nil || !want[r.Country] || (r.Impact != "High" && r.Impact != "Medium") ||
			t.Before(from) || !t.Before(to) {
			continue
		}
		events = append(events, Event{Time: t, Country: r.Country, Title: r.Title,
			Impact: r.Impact, Forecast: r.Forecast, Previous: r.Previous})
	}
	sort.SliceStable(events, func(i, j int) bool { return events[i].Time.Before(events[j].Time) })
	return events, nil
}
