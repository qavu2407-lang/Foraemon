package rates

import (
	"time"
)

// ICT is Vietnam's time (UTC+7, no daylight saving), for times shown to the reader.
var ICT = time.FixedZone("ICT", 7*3600)

func FormatPct(pct float64) string { return FormatHundredths(ToHundredths(pct)) + "%" }

func PctOrDash(p *float64) string {
	if p == nil {
		return "—"
	}
	return FormatPct(*p)
}

func FormatRange(low, high float64, decimals int) string {
	return FormatNum(low, decimals) + "–" + FormatNum(high, decimals)
}

func PublishedDate(unix int64) string { return time.Unix(unix, 0).UTC().Format("2 Jan 2006") }

// PublishedTime shows which publication the rates are: the trial key publishes hourly,
// so two runs on the same day can report different rates.
func PublishedTime(unix int64) string { return time.Unix(unix, 0).In(ICT).Format("2 Jan 2006, 15:04 ICT") }
