// Command score rates daily run records for factual precision, importance-weighted recall
// and their F1 (Jafari, Allan & Iyyer 2026, arXiv 2604.03141), judged only against the
// evidence that run's model was given. Not part of the daily run.
//
//	git fetch origin eval-data && git worktree add ../eval-data eval-data
//	JUDGE_MODEL=... OPENAI_API_KEY=... go run ./eval/score ../eval-data/tests/results/*.json
//
// Each record gets a .score.json beside it with every fact, claim and label, so the judge
// can be audited, and prints one summary line.
//
// -facts dir freezes the reference facts: the first run extracts and saves them in
// dir/<judge model>-<prompts hash>/<date>.json, later runs reuse them. Use it for one record
// scored repeatedly, or for several outputs judged on the same evidence (experiment arms), so
// differences come from the outputs, not from extraction. A new judge model or prompt gets a
// new folder, never another judge's facts.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"lorisocchipinti.com/gbp-rates/explain"
)

// query is q in the paper: what the explanation is supposed to answer.
const query = "Why did AUD/VND and CZK/VND move today, and what does it mean for someone converting AUD or CZK into VND?"

type record struct {
	Date           string               `json:"date"`
	Model          string               `json:"model"`
	ExplainSeconds float64              `json:"explain_seconds"`
	Usage          explain.Usage        `json:"usage"`
	Errors         []string             `json:"errors"`
	NoExplain      string               `json:"no_explain"`
	Input          json.RawMessage      `json:"input"`
	Output         *explain.Explanation `json:"output"`
}

type fact struct {
	Text       string  `json:"text"`
	Relevance  int     `json:"relevance"`
	Salience   int     `json:"salience"`
	Importance float64 `json:"importance"`
	Covered    bool    `json:"covered"`
	CoveredBy  []int   `json:"covered_by"` // ids of the claims the judge said cover it
}

// claim is one answer claim with the judge's verdict on it. The judge fills label, the
// evidence it used, quote and reason; the code fills batch and the two checks.
type claim struct {
	Text        string   `json:"text"`
	Label       string   `json:"label"` // SUPPORTED, CONTRADICTED or NOT_SUPPORTED
	HeadlineIDs []int    `json:"headline_ids"`
	Sections    []string `json:"sections"`
	Quote       string   `json:"quote"`
	Reason      string   `json:"reason"`
	Batch       int      `json:"batch"`       // which verification call judged it, from 1
	QuoteFound  bool     `json:"quote_found"` // the quote appears in the evidence
	IDsValid    bool     `json:"ids_valid"`   // every cited headline id exists
}

type score struct {
	Date      string  `json:"date"`
	Model     string  `json:"model"`
	Explained bool    `json:"explained"`
	F1        float64 `json:"f1"` // precision with importance-weighted recall: the gate metric
	Precision float64 `json:"precision"`
	WRecall   float64 `json:"weighted_recall"`
	Recall    float64 `json:"recall"`
	CRate     float64 `json:"contradiction_rate"`
	// Deterministic, no judge.
	Tokens         int             `json:"tokens"`
	ExplainSeconds float64         `json:"explain_seconds"`
	SourceErrors   int             `json:"source_errors"`
	LegNamed       map[string]bool `json:"leg_named"`
	JudgeModel     string          `json:"judge_model"`
	PromptsHash    string          `json:"prompts_hash"`
	ScoredAt       time.Time       `json:"scored_at"`
	JudgeUsage     explain.Usage   `json:"judge_usage"`
	Facts          []fact          `json:"facts"`
	Claims         []claim         `json:"claims"`
}

func main() {
	factsDir := flag.String("facts", "", "reuse each day's reference facts from this folder, or save them there")
	flag.Parse()
	judgeModel := os.Getenv("JUDGE_MODEL")
	if judgeModel == "" || flag.NArg() < 1 {
		log.Fatal("usage: JUDGE_MODEL=... go run ./eval/score [-facts dir] <record.json>...")
	}
	fmt.Println("date        f1    prec  wrec  rec   contra claims badq facts tokens secs errs legs")
	for _, path := range flag.Args() {
		if strings.HasSuffix(path, ".score.json") {
			continue
		}
		s, err := scoreFile(context.Background(), path, judgeModel, *factsDir)
		if err != nil {
			log.Fatalf("%s: %v", path, err)
		}
		badq := 0 // quotes the judge gave that aren't in the evidence
		for _, c := range s.Claims {
			if c.Quote != "" && !c.QuoteFound {
				badq++
			}
		}
		fmt.Printf("%s  %.2f  %.2f  %.2f  %.2f  %.2f   %6d %4d %5d %6d %4.0f %4d %v\n", s.Date, s.F1, s.Precision,
			s.WRecall, s.Recall, s.CRate, len(s.Claims), badq, len(s.Facts), s.Tokens, s.ExplainSeconds, s.SourceErrors, s.LegNamed)
	}
}

func scoreFile(ctx context.Context, path, judgeModel, factsDir string) (score, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return score{}, err
	}
	var r record
	if err := json.Unmarshal(data, &r); err != nil {
		return score{}, err
	}
	s := score{Date: r.Date, Model: r.Model, Explained: r.Output != nil, Tokens: r.Usage.InputTokens + r.Usage.OutputTokens,
		ExplainSeconds: r.ExplainSeconds, SourceErrors: len(r.Errors), JudgeModel: judgeModel,
		PromptsHash: promptsHash, ScoredAt: time.Now().UTC()}
	if r.Output != nil {
		s.LegNamed = legNamed(r.Input, *r.Output)
		j := judge{model: judgeModel, usage: &s.JudgeUsage}
		var factsFile string
		if factsDir != "" {
			factsFile = filepath.Join(factsDir, judgeModel+"-"+promptsHash, r.Date+".json")
		}
		facts, err := loadFacts(ctx, j, string(r.Input), factsFile)
		if err != nil {
			return score{}, err
		}
		if s.Facts, s.Claims, err = j.claims(ctx, string(r.Input), *r.Output, facts); err != nil {
			return score{}, err
		}
		s.Precision, s.CRate, s.Recall, s.WRecall, s.F1 = metrics(s.Facts, s.Claims)
	}
	out, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return score{}, err
	}
	return s, os.WriteFile(strings.TrimSuffix(path, ".json")+".score.json", out, 0o644)
}

// loadFacts reads the frozen facts from file, or extracts them and, when file is set, saves
// them there for the next run.
func loadFacts(ctx context.Context, j judge, evidence, file string) ([]fact, error) {
	if file != "" {
		data, err := os.ReadFile(file)
		if err == nil {
			var facts []fact
			return facts, json.Unmarshal(data, &facts)
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
	}
	facts, err := j.facts(ctx, evidence)
	if err != nil || file == "" {
		return facts, err
	}
	data, err := json.MarshalIndent(facts, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return nil, err
	}
	return facts, os.WriteFile(file, data, 0o644)
}

// metrics is eq. 2-5 of the paper with α = β = 1, over every reference fact (K* = |F|).
func metrics(facts []fact, claims []claim) (prec, crate, rec, wrec, f1 float64) {
	for _, c := range claims {
		prec += b2f(c.Label == "SUPPORTED")
		crate += b2f(c.Label == "CONTRADICTED")
	}
	var w float64
	for _, f := range facts {
		rec += b2f(f.Covered)
		wrec += f.Importance * b2f(f.Covered)
		w += f.Importance
	}
	prec, crate = ratio(prec, len(claims)), ratio(crate, len(claims))
	rec = ratio(rec, len(facts))
	if w > 0 {
		wrec /= w
	} else {
		wrec = rec // every fact rated least important: weights say nothing
	}
	if prec+wrec > 0 {
		f1 = 2 * prec * wrec / (prec + wrec)
	}
	return
}

// legNamed checks, per cross pair, that the explanation has a macro paragraph for the leg
// that moved it more: the base currency (AUD, CZK) or VND. A USD paragraph counts for either,
// since the prompt files a broad dollar move under USD.
// ponytail: names the right currency, not that the reasoning is right; the judge covers that.
func legNamed(input json.RawMessage, e explain.Explanation) map[string]bool {
	var in struct {
		Legs []map[string]string `json:"legs"`
	}
	if json.Unmarshal(input, &in) != nil {
		return nil
	}
	have := map[string]bool{}
	for _, c := range e.Macro {
		have[c.Currency] = true
	}
	out := map[string]bool{}
	for _, l := range in.Legs {
		pair := l["pair"]
		base := strings.SplitN(pair, "/", 2)[0]
		b, _ := strconv.ParseFloat(l[base+"_leg_pp"], 64)
		v, _ := strconv.ParseFloat(l["VND_leg_pp"], 64)
		driver := base
		if math.Abs(v) > math.Abs(b) {
			driver = "VND"
		}
		out[pair] = have[driver] || have["USD"]
	}
	return out
}

func b2f(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

func ratio(n float64, d int) float64 {
	if d == 0 {
		return 0
	}
	return n / float64(d)
}
