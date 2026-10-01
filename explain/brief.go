package explain

import (
	"context"
	"fmt"
	"time"

	"lorisocchipinti.com/gbp-rates/logger"
	"lorisocchipinti.com/gbp-rates/rates"
	"lorisocchipinti.com/gbp-rates/tools"
)

// Brief is everything beyond the rates. Each part is optional: a failed source is
// logged and left out, and the email still goes out.
type Brief struct {
	News        []tools.Headline
	Calendar    []tools.Event
	SBV         *tools.CentralRate
	Explanation *Explanation
	NoExplain   string         // why there is no explanation, "" when there is one
	Input       map[string]any // what the model was sent, kept for previews
}

func Gather(ctx context.Context, cur rates.Snapshot, rows []rates.PeriodRow, now time.Time) Brief {
	var b Brief
	news, errs := tools.FetchNews(ctx, now.Add(-48*time.Hour), 8)
	for _, err := range errs {
		logger.Error(fmt.Errorf("news: %w", err))
	}
	b.News = news

	// EUR stands in for the koruna, which mostly follows the ECB; the feed has no CZK.
	if cal, err := tools.FetchCalendar(ctx, now, now.Add(48*time.Hour), []string{"AUD", "USD", "EUR"}); err != nil {
		logger.Error(fmt.Errorf("calendar: %w", err))
	} else {
		b.Calendar = cal
	}
	if sbv, err := tools.FetchCentralRate(ctx); err != nil {
		logger.Error(fmt.Errorf("SBV central rate: %w", err))
	} else {
		b.SBV = &sbv
	}

	if len(news) == 0 {
		b.NoExplain = "no headlines could be fetched"
		return b
	}
	logger.Log(fmt.Sprintf("Explaining with %d headlines...", len(news)))
	b.Input = input(cur, rows, b)
	e, err := ask(ctx, b.Input, len(news))
	if err != nil {
		logger.Error(fmt.Errorf("explain: %w", err))
		b.NoExplain = err.Error()
		return b
	}
	b.Explanation = &e
	return b
}

// input is what the model sees: the numbers as the email prints them, so it can't
// restate one with different rounding, then the numbered headlines.
func input(cur rates.Snapshot, rows []rates.PeriodRow, b Brief) map[string]any {
	byName := map[string]rates.Pair{}
	for _, r := range rows {
		byName[r.Pair.Name] = r.Pair
	}
	var rateRows, legs, techs []map[string]any
	for _, r := range rows {
		p := r.Pair
		row := map[string]any{"pair": p.Name, "today": rates.FormatNum(p.Rate, p.Decimals),
			"wtd_pct": rates.PctOrDash(r.WTD), "mtd_pct": rates.PctOrDash(r.MTD), "ytd_pct": rates.PctOrDash(r.YTD)}
		if p.HasPrev() {
			row["last"] = rates.FormatNum(p.Prev, p.Decimals)
			row["day_pct"] = rates.FormatPct(p.Pct())
			if p.Split != "" {
				if a := rates.Attribute(p, byName[p.Split+"/USD"], byName["USD/VND"]); !a.Flat {
					_, base, vnd := a.Hundredths()
					legs = append(legs, map[string]any{"pair": p.Name,
						p.Split + "_leg_pp": rates.FormatHundredths(base), "VND_leg_pp": rates.FormatHundredths(vnd)})
				}
			}
		}
		rateRows = append(rateRows, row)
		t := r.Tech
		tech := map[string]any{"pair": p.Name, "range_30d": rates.FormatRange(t.Low30, t.High30, p.Decimals),
			"pos_30d": fmt.Sprintf("%.0f%%", rates.RangePosition(p.Rate, t.Low30, t.High30)), "streak_publications": t.Streak}
		if t.Has52 {
			tech["range_52w"] = rates.FormatRange(t.Low52, t.High52, p.Decimals)
			tech["pos_52w"] = fmt.Sprintf("%.0f%%", rates.RangePosition(p.Rate, t.Low52, t.High52))
		}
		techs = append(techs, tech)
	}

	in := map[string]any{"published": rates.PublishedTime(cur.PublishedAt), "rates": rateRows, "legs": legs, "technicals": techs}
	if b.SBV != nil {
		in["sbv_central_rate"] = map[string]any{"date": b.SBV.Date, "usd_vnd": rates.FormatNum(b.SBV.USDVND, 0),
			"market_vs_central_pct": rates.FormatPct((cur.PerUSD["VND"]/b.SBV.USDVND - 1) * 100)}
	}
	var cal []map[string]string
	for _, e := range b.Calendar {
		cal = append(cal, map[string]string{"time_ict": e.Time.In(rates.ICT).Format("Mon 2 Jan 15:04"),
			"currency": e.Country, "event": e.Title, "impact": e.Impact, "forecast": e.Forecast, "previous": e.Previous})
	}
	in["calendar"] = cal
	var news []map[string]any
	for i, h := range b.News {
		news = append(news, map[string]any{"id": i + 1, "publisher": h.Publisher, "author": h.Author,
			"published": h.Published.UTC().Format(time.RFC3339), "title": h.Title, "summary": h.Summary})
	}
	in["headlines"] = news
	return in
}
