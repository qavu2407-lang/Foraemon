package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_SignTest(t *testing.T) {
	assert.InDelta(t, 1.0/64, signTest(7, 0), 1e-12, "7 of 7: 2 × 1/128")
	assert.InDelta(t, 16.0/128, signTest(6, 1), 1e-12, "6 of 7: 2 × 8/128")
	assert.Equal(t, 1.0, signTest(3, 3))
	assert.Equal(t, 1.0, signTest(0, 0), "all ties: no evidence either way")
}

func write(t *testing.T, dir, name, date, judge string, explained bool, precision float64) {
	t.Helper()
	data := fmt.Sprintf(`{"date":%q,"judge_model":%q,"prompts_hash":"h","explained":%v,"precision":%v,
		"claims":[{"quote":"x","quote_found":false},{"quote":"","quote_found":false}]}`, date, judge, explained, precision)
	assert.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(data), 0o644))
}

func Test_Compare(t *testing.T) {
	base, variant := t.TempDir(), t.TempDir()
	write(t, base, "d1.score.json", "d1", "j", true, 0.5)
	write(t, base, "d2.score.json", "d2", "j", true, 0.8)
	write(t, base, "d3.score.json", "d3", "j", true, 0.9)
	write(t, base, "d4.score.json", "d4", "j", true, 0.9) // the variant has no d4: unpaired
	write(t, variant, "d1.score.json", "d1", "j", true, 0.6)
	write(t, variant, "d1.2.score.json", "d1", "j", true, 0.8) // second sample: d1 averages 0.7
	write(t, variant, "d2.score.json", "d2", "j", true, 0.8)
	write(t, variant, "d3.score.json", "d3", "j", false, 0) // failed: counted, not paired

	rows, unexplained, n, err := compare(base, variant)
	assert.NoError(t, err)
	assert.Equal(t, 2, n, "d1 and d2")
	assert.Equal(t, [2]int{0, 1}, unexplained)
	prec := rows[0]
	assert.Equal(t, "precision", prec.metric)
	assert.InDelta(t, 0.65, prec.base, 1e-9)
	assert.InDelta(t, 0.75, prec.variant, 1e-9)
	assert.Equal(t, [3]int{1, 0, 1}, [3]int{prec.wins, prec.losses, prec.ties})
	for _, r := range rows {
		if r.metric == "badq" {
			assert.Equal(t, 1.0, r.base, "one quote not found; an empty quote doesn't count")
		}
	}

	other := t.TempDir()
	write(t, other, "d1.score.json", "d1", "another judge", true, 0.5)
	_, _, _, err = compare(base, other)
	assert.ErrorContains(t, err, "different judges")
}
