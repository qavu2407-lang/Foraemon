package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/aws/aws-lambda-go/lambda"
	"lorisocchipinti.com/gbp-rates/logger"
	"lorisocchipinti.com/gbp-rates/mailer"
)

// RateRequest is the EventBridge event. Entry is optional: per pair, the MID-MARKET
// rate on the day you converted, e.g. {"entry": {"AUD/VND": 18450}}. Not the effective
// rate you received: that has a fee taken out, which would show as a permanent gain.
type RateRequest struct {
	Entry map[string]float64 `json:"entry"`
}

// main runs as a Lambda handler on Lambda, and once otherwise (GitHub Actions, local).
// Off Lambda, the event comes from EVENT, e.g. EVENT='{"entry":{"AUD/VND":18450}}'.
func main() {
	if os.Getenv("AWS_LAMBDA_RUNTIME_API") != "" {
		lambda.Start(CheckRate)
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

func CheckRate(ctx context.Context, request RateRequest) error {
	if err := validateEntries(request.Entry); err != nil {
		return err
	}

	// History only feeds the change figures. If it can't be read, still send the rates,
	// but remember the failure: saving now would overwrite the history with one day.
	history, historyErr := loadHistory(ctx)
	if historyErr != nil {
		logger.Error(fmt.Errorf("history unavailable, sending without change: %w", historyErr))
	}

	logger.Log("Fetching rates...")
	snap, err := fetchRates(ctx)
	if err != nil {
		return err
	}

	prev := previous(history, snap)
	pairs := derivePairs(snap, prev)
	for _, p := range pairs {
		logger.Log(fmt.Sprintf("%s %s", p.Name, formatNum(p.Rate, p.Decimals)))
	}

	history = appendSnapshot(history, snap)
	if historyErr == nil {
		if err := saveHistory(ctx, history); err != nil {
			logger.Error(fmt.Errorf("saving history: %w", err))
		}
	}

	// ponytail: the count is days of stored history, so it restarts if history is lost
	// and stops at keepSnapshots (400). A real send counter would need its own storage.
	subject, body, err := render(buildView(snap, prev, pairs, request.Entry, len(history)))
	if err != nil {
		return err
	}
	logger.Log("Sending email...")
	return mailer.Send(ctx, subject, body)
}

// validateEntries rejects unknown pair names and non-positive rates, so a typo in the
// event fails loudly instead of silently dropping the line or dividing by zero.
func validateEntries(entries map[string]float64) error {
	for name, rate := range entries {
		known := false
		for _, def := range pairDefs {
			known = known || def.name == name
		}
		if !known {
			return fmt.Errorf("entry %q is not a reported pair (use AUD/VND, AUD/USD, USD/VND, CZK/USD or CZK/VND)", name)
		}
		if rate <= 0 {
			return fmt.Errorf("entry for %s must be a positive rate, got %v", name, rate)
		}
	}
	return nil
}

// redact keeps the key out of the logs: net/http wraps failures in a *url.Error
// that prints the whole URL, and the key lives in the path.
func redact(err error, apikey string) error {
	return errors.New(strings.ReplaceAll(err.Error(), apikey, "REDACTED"))
}
