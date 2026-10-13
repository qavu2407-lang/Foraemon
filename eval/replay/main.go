// Command replay re-runs the generator on saved run records' inputs and writes one new record
// per sample into -out: the same date, input and source errors, the new output, usage and
// timing. eval/run.sh runs it inside a variant's worktree, so the variant's code generates.
//
//	model=... OPENAI_API_KEY=... go run ./eval/replay -out dir [-n 1] <record.json>...
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"lorisocchipinti.com/gbp-rates/explain"
)

func main() {
	out := flag.String("out", "", "folder for the replayed records")
	n := flag.Int("n", 1, "samples per record; sample k > 1 is written as <date>.k.json")
	flag.Parse()
	if *out == "" || *n < 1 || flag.NArg() < 1 || os.Getenv("model") == "" {
		log.Fatal("usage: model=... go run ./eval/replay -out dir [-n 1] <record.json>...")
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		log.Fatal(err)
	}
	for _, path := range flag.Args() {
		if strings.HasSuffix(path, ".score.json") {
			continue
		}
		if err := replay(context.Background(), path, *out, *n); err != nil {
			log.Fatalf("%s: %v", path, err)
		}
	}
}

func replay(ctx context.Context, path, out string, n int) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var r struct {
		Date   string          `json:"date"`
		Errors json.RawMessage `json:"errors"`
		Input  json.RawMessage `json:"input"`
		Case   json.RawMessage `json:"case"` // eval/cases: what was changed, for its check
	}
	if err := json.Unmarshal(data, &r); err != nil {
		return err
	}
	if len(r.Input) == 0 || string(r.Input) == "null" {
		log.Printf("%s: no input saved (the live run had no headlines), skipped", r.Date)
		return nil
	}
	for k := 1; k <= n; k++ {
		start := time.Now()
		e, u, err := explain.Replay(ctx, r.Input)
		rec := map[string]any{
			"date":            r.Date,
			"replayed":        start.UTC().Format(time.RFC3339),
			"source":          path,
			"model":           os.Getenv("model"),
			"explain_seconds": time.Since(start).Seconds(),
			"usage":           u,
			"errors":          r.Errors, // the live run's source errors: the evidence is the same
			"input":           r.Input,
		}
		if len(r.Case) > 0 {
			rec["case"] = r.Case
		}
		// A failed call is the variant's failure, scored as a day with no explanation.
		if err != nil {
			rec["no_explain"] = err.Error()
		} else {
			rec["output"] = e
		}
		name := r.Date + ".json"
		if k > 1 {
			name = fmt.Sprintf("%s.%d.json", r.Date, k)
		}
		data, err := json.MarshalIndent(rec, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(out, name), data, 0o644); err != nil {
			return err
		}
		log.Printf("%s sample %d: %d input + %d output tokens", r.Date, k, u.InputTokens, u.OutputTokens)
	}
	return nil
}
