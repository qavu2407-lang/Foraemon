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

func Test_Scenario_ReadsAsOneSentence(t *testing.T) {
	e := Explanation{Scenarios: []Scenario{
		{If: "If US payrolls are stronger than forecast.", Then: "The dollar could stay supported. FXStreet cited it."},
		{If: "the ECB sounds hawkish", Then: "then AUD/VND may recover"},
		{If: "payrolls miss", Then: "Friday's close could test the low"},
	}}.checked(0)
	assert.Equal(t, Scenario{If: "US payrolls are stronger than forecast", Then: "the dollar could stay supported. FXStreet cited it."}, e.Scenarios[0])
	assert.Equal(t, Scenario{If: "the ECB sounds hawkish", Then: "AUD/VND may recover."}, e.Scenarios[1], "acronyms keep their capitals")
	assert.Equal(t, "Friday's close could test the low.", e.Scenarios[2].Then, "names keep their capitals")
}
