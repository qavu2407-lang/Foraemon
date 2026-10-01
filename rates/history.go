package rates

import (
	"encoding/json"
	"errors"
	"os"
	"sort"
)

// The whole history is one small JSON array: about 85 bytes per day, so a decade is
// ~300 KB. Loading it whole beats running a database for it.
const (
	historyKey    = "history.json"
	keepSnapshots = 400 // a little over a year, enough for any window worth showing

	// SeedFile is the one-off backfill (go run ./cmd/backfill), committed to the repo so
	// the year it holds survives an evicted Actions cache: it can't be fetched again.
	SeedFile = "data/history-seed.json"
)

// LoadHistory returns the stored snapshots merged with the seed, oldest first. A missing
// file is not an error: it's the first run, or there is no seed.
func LoadHistory() ([]Snapshot, error) {
	live, err := readSnapshots(historyFile())
	if err != nil {
		return nil, err
	}
	seed, err := readSnapshots(SeedFile)
	if err != nil {
		return nil, err
	}
	return mergeDaily(seed, live), nil
}

func readSnapshots(path string) ([]Snapshot, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var h []Snapshot
	err = json.Unmarshal(data, &h)
	return h, err
}

// mergeDaily keeps one snapshot per UTC day where both have one, preferring live: the
// daily run's rates are what Today and Last are measured in.
func mergeDaily(seed, live []Snapshot) []Snapshot {
	liveDays := make(map[int64]bool, len(live))
	for _, s := range live {
		liveDays[s.PublishedAt/86400] = true
	}
	h := append([]Snapshot(nil), live...)
	for _, s := range seed {
		if !liveDays[s.PublishedAt/86400] {
			h = append(h, s)
		}
	}
	sort.Slice(h, func(i, j int) bool { return h[i].PublishedAt < h[j].PublishedAt })
	return h
}

func SaveHistory(h []Snapshot) error {
	data, err := json.Marshal(h)
	if err != nil {
		return err
	}
	return os.WriteFile(historyFile(), data, 0o644)
}

func historyFile() string {
	if f := os.Getenv("HISTORY_FILE"); f != "" {
		return f
	}
	return historyKey
}

// AppendSnapshot adds s unless it's the same publication as the latest entry: the free
// tier republishes about once a day, so a re-run can see rates it has already stored.
func AppendSnapshot(h []Snapshot, s Snapshot) []Snapshot {
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
func Previous(h []Snapshot, cur Snapshot) *Snapshot { return Before(h, cur.PublishedAt) }

// before is the latest publication strictly before unix time t, or nil.
func Before(h []Snapshot, t int64) *Snapshot {
	for i := len(h) - 1; i >= 0; i-- {
		if h[i].PublishedAt < t {
			return &h[i]
		}
	}
	return nil
}
