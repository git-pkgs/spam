package main

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestDocumentationComparisonThroughCommand(t *testing.T) {
	cases := []struct {
		catalog string
		scores  map[string]int
	}{
		{"seed", map[string]int{
			"ai-interior-design-info": 0,
		}},
		{"documentation", map[string]int{
			"node-style-guide":       0,
			"javascript-style-guide": 0,
			"awesome-standard":       0,
			"awesome-electron":       0,
		}},
		{"fresh", map[string]int{
			"essay-writing-tips-for-descriptive-essays-read": 0,
			"long-distance-movers":                           0,
		}},
	}
	for _, tc := range cases {
		t.Run(tc.catalog, func(t *testing.T) {
			var stdout bytes.Buffer
			if err := run([]string{corpusFlag, "../../testdata/corpus/" + tc.catalog + "/catalog.json"}, &stdout); err != nil {
				t.Fatal(err)
			}
			var results []output
			if err := json.Unmarshal(stdout.Bytes(), &results); err != nil {
				t.Fatal(err)
			}
			verifyComparisonScores(t, results, tc.scores)
		})
	}
}

func verifyComparisonScores(t *testing.T, results []output, scores map[string]int) {
	t.Helper()
	checked := 0
	for _, result := range results {
		want, ok := scores[result.Name]
		if !ok {
			continue
		}
		checked++
		if result.Result.Score != want || result.Result.Ruleset != "raw-text-6" {
			t.Fatalf("%s: score %d, want %d", result.Name, result.Result.Score, want)
		}
		for _, match := range result.Result.Matches {
			if match.Contribution != 0 {
				t.Fatalf("%s: unexpected contribution from %s", result.Name, match.Rule)
			}
		}
	}
	if checked != len(scores) {
		t.Fatal("missing comparison packages")
	}
}
