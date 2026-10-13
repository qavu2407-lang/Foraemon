// Package clean is the transform stage between fetching and the model. It is deterministic
// and calls no model, so the same raw text always gives the same input. Callers keep the raw
// text beside the result (ELT), so a changed rule can be re-run over old days.
package clean

import (
	"html"
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Report counts what the steps found, for the run record. Invisible characters and
// mixed-script words are how text gets hidden from a human reader: a non-zero count is
// worth a look.
type Report struct {
	Invisible   int      `json:"invisible_removed"`
	MixedScript []string `json:"mixed_script_words"`
	Duplicates  int      `json:"near_duplicates_dropped"`
	Truncated   int      `json:"truncated"`
}

var tags = regexp.MustCompile(`<[^>]*>`)

// Text runs the steps in order and cuts the result to max runes. r collects what was found;
// nil when nobody needs to know.
//
//  1. markup: tags dropped, entities decoded (feeds escape their HTML, so decode twice)
//  2. invisible characters: zero-width, direction overrides and other format/control
//     characters dropped, since they carry no visible text
//  3. NFKC: one form per character, so a Vietnamese letter typed as base + accent matches
//     the precomposed one, and full-width letters become plain ones
//  4. mixed scripts: words mixing Latin with Cyrillic or Greek ("Fеd" with a Cyrillic е)
//     are flagged, not changed: rewriting them would be guessing
//  5. whitespace collapsed
//  6. length: cut to max runes
func Text(s string, max int, r *Report) string {
	if r == nil {
		r = &Report{}
	}
	s = html.UnescapeString(tags.ReplaceAllString(html.UnescapeString(s), " "))
	s = strings.Map(func(c rune) rune {
		if unicode.Is(unicode.Cf, c) || (unicode.IsControl(c) && !unicode.IsSpace(c)) {
			r.Invisible++
			return -1
		}
		return c
	}, s)
	s = norm.NFKC.String(s)
	for _, w := range strings.FieldsFunc(s, func(c rune) bool { return !unicode.IsLetter(c) }) {
		if mixed(w) && len(r.MixedScript) < 20 {
			r.MixedScript = append(r.MixedScript, w)
		}
	}
	s = strings.Join(strings.Fields(s), " ")
	if rs := []rune(s); len(rs) > max {
		r.Truncated++
		s = string(rs[:max]) + "…"
	}
	return s
}

// mixed reports whether a word has Latin letters and Cyrillic or Greek ones.
func mixed(w string) bool {
	var latin, other bool
	for _, c := range w {
		latin = latin || unicode.Is(unicode.Latin, c)
		other = other || unicode.Is(unicode.Cyrillic, c) || unicode.Is(unicode.Greek, c)
	}
	return latin && other
}

// Key is a title reduced to lowercase letters and digits, so the same story with different
// punctuation, case or spacing counts once.
func Key(title string) string {
	return strings.Join(strings.FieldsFunc(strings.ToLower(title), func(c rune) bool {
		return !unicode.IsLetter(c) && !unicode.IsDigit(c)
	}), " ")
}
