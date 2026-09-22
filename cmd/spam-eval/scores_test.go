package main

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestDefaultCorpusScores(t *testing.T) {
	positive := map[string]bool{
		"free-steam-codes-generator":         true,
		"project-makeover-hack-gems":         true,
		"1000-pip-climber-system-cracked":    true,
		"ikigai-reviews-weight-loss-capsule": true,
	}
	seen := make(map[string]bool)
	for _, catalog := range []string{"seed/catalog", "validation/catalog", "validation/stress-catalog", "fresh/catalog", "documentation/catalog", "live/catalog"} {
		t.Run(catalog, func(t *testing.T) {
			var stdout bytes.Buffer
			if err := run([]string{corpusFlag, "../../testdata/corpus/" + catalog + ".json"}, &stdout); err != nil {
				t.Fatal(err)
			}
			var rows []output
			if err := json.Unmarshal(stdout.Bytes(), &rows); err != nil {
				t.Fatal(err)
			}
			for _, row := range rows {
				want := 0
				if positive[row.Name] {
					want = 1
					seen[row.Name] = true
				}
				if row.Result.Score != want {
					t.Errorf("%s (%s): score %d, want %d", row.Name, row.Label, row.Result.Score, want)
				}
			}
		})
	}
	if len(seen) != len(positive) {
		t.Fatal("missing positive regression cases")
	}
}
