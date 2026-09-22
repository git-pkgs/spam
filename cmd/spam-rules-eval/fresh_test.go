package main

import (
	"slices"
	"testing"
)

func TestFreshCorpusThroughCommand(t *testing.T) {
	const catalog = "../../testdata/corpus/fresh/catalog.json"
	const slimchews = "slimchews-acv-gummies-weight-loss-formula-reviews:"
	const ikigai = "ikigai-reviews-weight-loss-capsule:"
	cases := []struct {
		rules string
		want  []string
	}{
		{candidatePath, []string{slimchews + moneyBackRule}},
		{secondCandidatePath, []string{ikigai + clickHereRule, ikigai + "SA___TRANSFORM_LIFE", slimchews + clickHereRule}},
	}
	for _, tc := range cases {
		for _, view := range []string{rawMode, normalizedMode} {
			t.Run(tc.rules+"/"+view, func(t *testing.T) {
				result := runTestCorpus(t, tc.rules, catalog, view)
				verifyCorpusOutput(t, catalog, result)
				if got := packageRulePairs(result); !slices.Equal(got, tc.want) {
					t.Fatalf("got %v want %v", got, tc.want)
				}
			})
		}
	}
}

func packageRulePairs(result output) []string {
	var pairs []string
	for _, row := range result.Rows {
		for _, hit := range row.Hits {
			pairs = append(pairs, row.Name+":"+hit.Rule)
		}
	}
	slices.Sort(pairs)
	return pairs
}
