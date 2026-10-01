package rates

import (
	"math"
	"time"
)

// periodStarts are the WTD, MTD and YTD boundaries for a publication: Monday, the 1st and
// 1 January at 00:00 UTC. UTC because the provider's days are UTC days: a backfilled day
// closes at 23:00 UTC, so an ICT boundary (17:00 UTC the day before) would pick the
// close of the wrong day.
func periodStarts(published int64) (week, month, year int64) {
	t := time.Unix(published, 0).UTC()
	day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	week = day.AddDate(0, 0, -(int(t.Weekday())+6)%7).Unix()
	month = time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC).Unix()
	year = time.Date(t.Year(), 1, 1, 0, 0, 0, 0, time.UTC).Unix()
	return week, month, year
}

// periodChange is the % change of pair def since the last publication before t, or
// ok=false when the history doesn't reach back that far.
func periodChange(h []Snapshot, cur Snapshot, rate func(map[string]float64) float64, t int64) (pct float64, ok bool) {
	base := Before(h, t)
	if base == nil {
		return 0, false
	}
	return (rate(cur.PerUSD)/rate(base.PerUSD) - 1) * 100, true
}

// Technical is where a pair sits in its recent ranges, computed from history only.
type Technical struct {
	Low30, High30 float64
	Low52, High52 float64
	Has52         bool // the history covers (nearly) a full year
	Streak        int  // consecutive publications moving the same way: +3 = up three times
}

// technicals ranges over the publications within 30 days and 52 weeks of cur, cur
// included. Has52 needs 360 days of history, so a short history doesn't pass for a
// 52-week range.
func technicals(h []Snapshot, cur Snapshot, rate func(map[string]float64) float64) Technical {
	now := rate(cur.PerUSD)
	t := Technical{Low30: now, High30: now, Low52: now, High52: now}
	for _, s := range h {
		age := cur.PublishedAt - s.PublishedAt
		if age < 0 || age > 365*86400 {
			continue
		}
		r := rate(s.PerUSD)
		t.Low52, t.High52 = math.Min(t.Low52, r), math.Max(t.High52, r)
		if age <= 30*86400 {
			t.Low30, t.High30 = math.Min(t.Low30, r), math.Max(t.High30, r)
		}
		t.Has52 = t.Has52 || age >= 360*86400
	}
	t.Streak = streak(h, cur, rate)
	return t
}

// streak counts back from cur while each publication moved the same way as the last.
func streak(h []Snapshot, cur Snapshot, rate func(map[string]float64) float64) int {
	n, dir, next := 0, 0, rate(cur.PerUSD)
	for i := len(h) - 1; i >= 0; i-- {
		if h[i].PublishedAt >= cur.PublishedAt {
			continue
		}
		r := rate(h[i].PerUSD)
		d := 0
		if next > r {
			d = 1
		} else if next < r {
			d = -1
		}
		if d == 0 || (dir != 0 && d != dir) {
			break
		}
		dir, n, next = d, n+1, r
	}
	return n * dir
}

// RangePosition is where v sits between low and high: 0 at the low, 100 at the high.
func RangePosition(v, low, high float64) float64 {
	if high-low < 1e-12 {
		return 50
	}
	return (v - low) / (high - low) * 100
}

// PeriodRow is one line of the FX market rates table and its technical context.
type PeriodRow struct {
	Pair          Pair
	WTD, MTD, YTD *float64 // nil when the history doesn't reach back that far
	Tech          Technical
}

// PeriodRows follows pairs, which DerivePairs builds in pairDefs order.
func PeriodRows(h []Snapshot, cur Snapshot, pairs []Pair) []PeriodRow {
	week, month, year := periodStarts(cur.PublishedAt)
	rows := make([]PeriodRow, len(pairs))
	for i, def := range pairDefs {
		rows[i] = PeriodRow{Pair: pairs[i], Tech: technicals(h, cur, def.rate)}
		for _, p := range []struct {
			dst **float64
			t   int64
		}{{&rows[i].WTD, week}, {&rows[i].MTD, month}, {&rows[i].YTD, year}} {
			if pct, ok := periodChange(h, cur, def.rate, p.t); ok {
				*p.dst = &pct
			}
		}
	}
	return rows
}
