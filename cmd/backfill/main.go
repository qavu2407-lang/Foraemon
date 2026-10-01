package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"lorisocchipinti.com/gbp-rates/logger"
	"lorisocchipinti.com/gbp-rates/rates"
)

// One-off, run by hand while a paid exchangerate-api plan is active:
//
//	set -a; source .env; set +a
//	go run ./cmd/backfill 2025-10-01
//
// It wrote data/history-seed.json for the Pro trial (Oct 2026). The daily run never
// calls it; delete this folder once the seed is no longer worth refreshing.
func main() {
	if len(os.Args) != 2 {
		log.Fatal("usage: go run ./cmd/backfill 2025-10-01")
	}
	if err := backfill(context.Background(), os.Args[1]); err != nil {
		log.Fatal(err)
	}
}

// backfill fetches one historical snapshot per UTC day from `from` up to yesterday and
// writes them to rates.SeedFile. Needs a paid exchangerate-api plan: the free tier answers
// "plan-upgrade-required". Any failed day stops the run before anything is written,
// so a seed with gaps is never saved.
func backfill(ctx context.Context, from string) error {
	start, err := time.Parse("2006-01-02", from)
	if err != nil {
		return fmt.Errorf("backfill wants a start date like 2025-10-01: %w", err)
	}
	end := time.Now().UTC().Truncate(24 * time.Hour) // today 00:00 UTC: today isn't closed yet

	var seed []rates.Snapshot
	for d := start; d.Before(end); d = d.AddDate(0, 0, 1) {
		s, err := rates.FetchSnapshot(ctx, fmt.Sprintf("history/USD/%d/%d/%d", d.Year(), d.Month(), d.Day()))
		if err != nil {
			return fmt.Errorf("backfill %s: %w", d.Format("2006-01-02"), err)
		}
		seed = append(seed, s)
		if len(seed)%30 == 0 {
			logger.Log(fmt.Sprintf("backfilled up to %s", d.Format("2006-01-02")))
		}
	}

	data, err := json.Marshal(seed)
	if err != nil {
		return err
	}
	logger.Log(fmt.Sprintf("writing %d days to %s", len(seed), rates.SeedFile))
	return os.WriteFile(rates.SeedFile, data, 0o644)
}
