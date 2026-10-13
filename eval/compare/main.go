// Command compare pairs a baseline's and a variant's score files by date and prints, per
// metric, both means, the mean paired difference, the days the variant won, lost and tied, and
// a two-sided sign-test p-value over the days that weren't ties. Several samples of one day
// are averaged first. Metrics are compared only on days both explained.
//
//	go run ./eval/compare <baseline dir> <variant dir>
//
// It refuses to compare scores from different judges: a judge change needs a new baseline.
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
)

// metrics are the score fields compared, and whether higher is better.
var metrics = []struct {
	name   string
	higher bool
}{
	{"precision", true},
	{"contradiction_rate", false},
	{"f1", true},
	{"weighted_recall", true},
	{"recall", true},
	{"badq", false}, // quotes not found in the evidence
	{"tokens", false},
	{"explain_seconds", false},
}

// day is one date's metrics, averaged over its samples.
type day struct {
	explained bool // every sample explained
	values    map[string]float64
}

type row struct {
	metric             string
	base, variant      float64
	wins, losses, ties int
	p                  float64
}

func main() {
	if len(os.Args) != 3 {
		log.Fatal("usage: go run ./eval/compare <baseline dir> <variant dir>")
	}
	rows, unexplained, n, err := compare(os.Args[1], os.Args[2])
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%d paired days; days not explained: baseline %d, variant %d\n\n", n, unexplained[0], unexplained[1])
	fmt.Println("metric               baseline  variant   diff   W/L/T     p")
	for _, r := range rows {
		fmt.Printf("%-20s %8.3f %8.3f %+7.3f   %d/%d/%d  %5.3f\n", r.metric, r.base, r.variant,
			r.variant-r.base, r.wins, r.losses, r.ties, r.p)
	}
}

func compare(baseDir, variantDir string) (rows []row, unexplained [2]int, paired int, err error) {
	base, baseJudge, err := load(baseDir)
	if err != nil {
		return nil, unexplained, 0, err
	}
	variant, variantJudge, err := load(variantDir)
	if err != nil {
		return nil, unexplained, 0, err
	}
	if baseJudge != variantJudge {
		return nil, unexplained, 0, fmt.Errorf("different judges: baseline %q, variant %q", baseJudge, variantJudge)
	}
	var dates []string
	for d, b := range base {
		v, ok := variant[d]
		if !ok {
			continue
		}
		if !b.explained {
			unexplained[0]++
		}
		if !v.explained {
			unexplained[1]++
		}
		if b.explained && v.explained {
			dates = append(dates, d)
		}
	}
	sort.Strings(dates)
	for _, m := range metrics {
		r := row{metric: m.name}
		for _, d := range dates {
			b, v := base[d].values[m.name], variant[d].values[m.name]
			r.base += b / float64(len(dates))
			r.variant += v / float64(len(dates))
			switch diff := v - b; {
			case math.Abs(diff) < 1e-9:
				r.ties++
			case (diff > 0) == m.higher:
				r.wins++
			default:
				r.losses++
			}
		}
		r.p = signTest(r.wins, r.losses)
		rows = append(rows, r)
	}
	return rows, unexplained, len(dates), nil
}

// load reads every .score.json in dir, by date, and the judge that scored them all.
func load(dir string) (map[string]day, string, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.score.json"))
	if err != nil {
		return nil, "", err
	}
	if len(paths) == 0 {
		return nil, "", fmt.Errorf("%s: no .score.json files", dir)
	}
	var judge string
	sums := map[string]map[string]float64{}
	counts := map[string]int{}
	explained := map[string]bool{}
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, "", err
		}
		var s map[string]json.RawMessage
		if err := json.Unmarshal(data, &s); err != nil {
			return nil, "", fmt.Errorf("%s: %w", p, err)
		}
		var date, model, hash string
		var ok bool
		var claims []struct {
			Quote      string `json:"quote"`
			QuoteFound bool   `json:"quote_found"`
		}
		json.Unmarshal(s["date"], &date)
		json.Unmarshal(s["judge_model"], &model)
		json.Unmarshal(s["prompts_hash"], &hash)
		json.Unmarshal(s["explained"], &ok)
		json.Unmarshal(s["claims"], &claims)
		if j := model + " " + hash; judge == "" {
			judge = j
		} else if j != judge {
			return nil, "", fmt.Errorf("%s: judged by %q, others by %q", p, j, judge)
		}
		if sums[date] == nil {
			sums[date] = map[string]float64{}
			explained[date] = true
		}
		counts[date]++
		explained[date] = explained[date] && ok
		for _, m := range metrics {
			var v float64
			if m.name == "badq" {
				for _, c := range claims {
					if c.Quote != "" && !c.QuoteFound {
						v++
					}
				}
			} else {
				json.Unmarshal(s[m.name], &v)
			}
			sums[date][m.name] += v
		}
	}
	out := map[string]day{}
	for d, vs := range sums {
		for k := range vs {
			vs[k] /= float64(counts[d])
		}
		out[d] = day{explained: explained[d], values: vs}
	}
	return out, judge, nil
}

// signTest is the two-sided exact sign test: how likely a split at least this uneven is if
// the variant were as likely to win as to lose on each day.
func signTest(wins, losses int) float64 {
	n, k := wins+losses, min(wins, losses)
	if n == 0 {
		return 1
	}
	var tail float64
	for i := 0; i <= k; i++ {
		tail += binom(n, i)
	}
	return math.Min(1, 2*tail/math.Pow(2, float64(n)))
}

func binom(n, k int) float64 {
	r := 1.0
	for i := 1; i <= k; i++ {
		r = r * float64(n-k+i) / float64(i)
	}
	return r
}
