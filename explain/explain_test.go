package explain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_Explanation_DropsMadeUpCitations(t *testing.T) {
	e := Explanation{
		Macro: []Claim{
			{Text: "real", Cites: []int{1, 2}},
			{Text: "invented source", Cites: []int{3}},
			{Text: "zero id", Cites: []int{0}},
			{Text: "honest gap"},
			{Text: "", Cites: []int{1}},
		},
		Scenarios: make([]Scenario, 5),
	}.checked(2)
	assert.Equal(t, []Claim{{Text: "real", Cites: []int{1, 2}}, {Text: "honest gap"}}, e.Macro)
	assert.Len(t, e.Scenarios, 3)
}
