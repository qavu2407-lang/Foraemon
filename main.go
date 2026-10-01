package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"lorisocchipinti.com/gbp-rates/email"
	"lorisocchipinti.com/gbp-rates/explain"
	"lorisocchipinti.com/gbp-rates/logger"
	"lorisocchipinti.com/gbp-rates/mailer"
	"lorisocchipinti.com/gbp-rates/rates"
)

// RateRequest is the optional EVENT input. Entry is optional: per pair, the MID-MARKET
// rate on the day you converted, e.g. {"entry": {"AUD/VND": 18450}}. Not the effective
// rate you received: that has a fee taken out, which would show as a permanent gain.
type RateRequest struct {
	Entry map[string]float64 `json:"entry"`
}

// main runs once and exits (GitHub Actions, local). The optional event comes from
// EVENT, e.g. EVENT='{"entry":{"AUD/VND":18450}}'. MODE=record only stores today's rates,
// for a second schedule that records a fixed time of day without sending an email.
func main() {
	if os.Getenv("MODE") == "record" {
		if _, _, err := record(context.Background()); err != nil {
			fail(err)
		}
		return
	}

	var request RateRequest
	if event := os.Getenv("EVENT"); event != "" {
		if err := json.Unmarshal([]byte(event), &request); err != nil {
			fail(fmt.Errorf("EVENT is not valid JSON: %w", err))
		}
	}
	if err := CheckRate(context.Background(), request); err != nil {
		fail(err)
	}
}

// fail exits with err. On GitHub Actions it also prints an ::error:: line, which puts the
// message on the run's summary page instead of only in the step log.
func fail(err error) {
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		fmt.Printf("::error::%s\n", strings.ReplaceAll(err.Error(), "\n", "%0A"))
	}
	log.Fatal(err)
}

// CheckRate is the daily run: rates, then the evidence and its explanation, then the email.
func CheckRate(ctx context.Context, request RateRequest) error {
	if err := rates.ValidateEntries(request.Entry); err != nil {
		return err
	}
	history, snap, err := record(ctx)
	if err != nil && len(history) == 0 {
		return err // no rates at all
	}
	if err != nil {
		logger.Error(err) // rates fetched, history not saved: send anyway
	}

	prev := rates.Previous(history, snap) // strictly older, so not today's own snapshot
	pairs := rates.DerivePairs(snap, prev)
	rows := rates.PeriodRows(history, snap, pairs)
	b := explain.Gather(ctx, snap, rows, time.Now())

	subject, body, err := email.Render(email.Build(snap, prev, pairs, request.Entry, rows, b))
	if err != nil {
		return err
	}
	// PREVIEW_FILE writes the email to a file instead of sending it, plus what the model
	// was sent and wrote beside it as .json, to check the design and the explanation.
	if f := os.Getenv("PREVIEW_FILE"); f != "" {
		logger.Log(fmt.Sprintf("Subject: %s\nWriting preview to %s", subject, f))
		model, _ := json.MarshalIndent(map[string]any{"input": b.Input, "output": b.Explanation}, "", "  ")
		if err := os.WriteFile(strings.TrimSuffix(f, ".html")+".json", model, 0o644); err != nil {
			return err
		}
		return os.WriteFile(f, []byte(body), 0o644)
	}
	logger.Log("Sending email...")
	return mailer.Send(ctx, subject, body)
}

// record fetches the latest rates and appends them to the history. It returns the history
// with today's snapshot last. A failed fetch returns no history; a history that can't be
// read or saved returns the snapshot alone plus the error, and the file is left untouched
// rather than overwritten with a single day.
func record(ctx context.Context) ([]rates.Snapshot, rates.Snapshot, error) {
	history, historyErr := rates.LoadHistory()
	// A missing seed isn't an error, so say how much history there is: 1 means it's gone.
	logger.Log(fmt.Sprintf("History: %d publications", len(history)))

	logger.Log("Fetching rates...")
	snap, err := rates.FetchLatest(ctx)
	if err != nil {
		return nil, rates.Snapshot{}, err
	}
	logger.Log("Published " + rates.PublishedTime(snap.PublishedAt))
	for _, p := range rates.DerivePairs(snap, nil) {
		logger.Log(fmt.Sprintf("%s %s", p.Name, rates.FormatNum(p.Rate, p.Decimals)))
	}

	if historyErr != nil {
		return []rates.Snapshot{snap}, snap, fmt.Errorf("history unavailable, not saved: %w", historyErr)
	}
	history = rates.AppendSnapshot(history, snap)
	if err := rates.SaveHistory(history); err != nil {
		return history, snap, fmt.Errorf("saving history: %w", err)
	}
	return history, snap, nil
}
