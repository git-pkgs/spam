package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestSecondBatchThroughCommand(t *testing.T) {
	cases := []struct {
		catalog         string
		raw, normalized map[string]int
	}{
		{"seed/catalog.json", map[string]int{"1000-pip-climber-system-cracked": 2}, map[string]int{"1000-pip-climber-system-cracked": 2}},
		{"validation/catalog.json", map[string]int{"MailKit": 1}, map[string]int{}},
		{"validation/stress-catalog.json", map[string]int{datasetPackage: 1}, map[string]int{datasetPackage: 1}},
	}
	for _, tc := range cases {
		for _, view := range []string{rawMode, normalizedMode} {
			t.Run(tc.catalog+"/"+view, func(t *testing.T) {
				catalog := filepath.Join("../../testdata/corpus", tc.catalog)
				got := runTestCorpus(t, secondCandidatePath, catalog, view)
				actual := verifySecondBatch(t, got)

				want := tc.raw
				if view == normalizedMode {
					want = tc.normalized
				}
				if !reflect.DeepEqual(actual, want) {
					t.Fatalf("got %v want %v", actual, want)
				}
			})
		}
	}
}

func TestSecondBatchSourceDefinitions(t *testing.T) {
	data, err := os.ReadFile(secondCandidatePath)
	if err != nil {
		t.Fatal(err)
	}
	var rules []struct {
		rule
		Line       int    `json:"line"`
		Definition string `json:"source_definition"`
	}
	if err := json.Unmarshal(data, &rules); err != nil {
		t.Fatal(err)
	}
	for _, r := range rules {
		source, err := os.ReadFile(filepath.Join("../../rules/upstream/sources", r.Project, r.File))
		if err != nil {
			t.Fatal(err)
		}
		if fmt.Sprintf("%x", sha256.Sum256(source)) != r.SourceSHA256 || strings.TrimSpace(strings.Split(string(source), "\n")[r.Line-1]) != r.Definition {
			t.Fatalf("source changed: %s", r.ID)
		}
	}
}

func verifySecondBatch(t *testing.T, got output) map[string]int {
	t.Helper()
	if got.RuleCount != 16 || got.FixturesChecked != 32 || got.Comparisons == 0 {
		t.Fatal("missing expression comparisons")
	}
	actual := map[string]int{}
	for _, row := range got.Rows {
		for _, h := range row.Hits {
			if h.Rule != clickHereRule || !strings.EqualFold(h.Excerpt, "click here") {
				t.Fatalf("unexpected match: %+v", h)
			}
			actual[row.Name]++
		}
		if (row.Name == "MailKit" || row.Name == datasetPackage) && row.Group != "negative" {
			t.Fatal("legitimate control relabelled")
		}
	}
	return actual
}
