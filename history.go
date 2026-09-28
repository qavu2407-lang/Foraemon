package main

import (
	"encoding/json"
	"errors"
	"os"
)

// The whole history is one small JSON array: about 85 bytes per day, so a decade is
// ~300 KB. Loading it whole beats running a database for it.
const (
	historyKey    = "history.json"
	keepSnapshots = 400 // a little over a year, enough for any window worth showing
)

// loadHistory returns the stored snapshots, oldest first. A missing history is not an
// error: it's the first run.
func loadHistory() ([]Snapshot, error) {
	data, err := readHistory()
	if err != nil || data == nil {
		return nil, err
	}
	var h []Snapshot
	if err := json.Unmarshal(data, &h); err != nil {
		return nil, err
	}
	return h, nil
}

func saveHistory(h []Snapshot) error {
	data, err := json.Marshal(h)
	if err != nil {
		return err
	}
	return os.WriteFile(historyFile(), data, 0o644)
}

// readHistory returns nil data, not an error, when nothing has been stored yet.
func readHistory() ([]byte, error) {
	data, err := os.ReadFile(historyFile())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return data, err
}

func historyFile() string {
	if f := os.Getenv("HISTORY_FILE"); f != "" {
		return f
	}
	return historyKey
}

// appendSnapshot adds s unless it's the same publication as the latest entry: the free
// tier republishes about once a day, so a re-run can see rates it has already stored.
func appendSnapshot(h []Snapshot, s Snapshot) []Snapshot {
	if len(h) > 0 && h[len(h)-1].PublishedAt == s.PublishedAt {
		return h
	}
	h = append(h, s)
	if len(h) > keepSnapshots {
		h = h[len(h)-keepSnapshots:]
	}
	return h
}

// previous is the latest stored publication older than cur, so a re-run on the same
// publication compares against the one before rather than against itself.
func previous(h []Snapshot, cur Snapshot) *Snapshot {
	for i := len(h) - 1; i >= 0; i-- {
		if h[i].PublishedAt < cur.PublishedAt {
			return &h[i]
		}
	}
	return nil
}
