package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"testing"
)

func TestLiveCorpusControlsThroughCommand(t *testing.T) {
	const catalog = "../../testdata/corpus/live/catalog.json"
	const repeatedLines = "REPEATED_LINES"
	const fragcapGroup = "fragcap"
	want := map[string]struct{ group, rule string }{
		"yugabyte/yugaware":                  {"yugabyte", ""},
		"aws.sdk.kotlin:chime-jvm":           {"aws-sdk-kotlin", repeatedLines},
		"aws.sdk.kotlin:arczonalshift":       {"aws-sdk-kotlin", repeatedLines},
		fragcapGroup:                         {fragcapGroup, repeatedLines},
		"fragcap-core":                       {fragcapGroup, ""},
		"@nx/eslint":                         {"nx", "LINK_HEAVY"},
		"@midnight-ntwrk/compact-js-command": {"midnight-compact", "KEYWORD_REPETITION"},
		"itenterprise.maui.printing":         {"itenterprise-printing", repeatedLines},
		"github.com/python-caldav/caldav":    {"python-caldav", repeatedLines},
		"graphatoms":                         {"graphatoms", ""},
	}
	var stdout bytes.Buffer
	if err := run([]string{corpusFlag, catalog}, &stdout); err != nil {
		t.Fatal(err)
	}
	var results []output
	if err := json.Unmarshal(stdout.Bytes(), &results); err != nil {
		t.Fatal(err)
	}
	if len(results) != len(want) {
		t.Fatal("missing live controls")
	}
	for _, result := range results {
		expected, ok := want[result.Name]
		if !ok || result.SplitGroup != expected.group || result.Label != "legitimate" || result.EvaluationGroup != "negative" || result.Result.Score != 0 {
			t.Fatalf("unexpected control result: %+v", result)
		}
		delete(want, result.Name)
		if expected.rule == "" {
			if len(result.Result.Matches) != 0 {
				t.Fatalf("unexpected signals: %s", result.Name)
			}
			continue
		}
		if len(result.Result.Matches) != 1 || result.Result.Matches[0].Rule != expected.rule || result.Result.Matches[0].Contribution != 0 {
			t.Fatalf("informational signal changed: %s", result.Name)
		}
	}
}

func TestLiveSelectionCatalogueHash(t *testing.T) {
	selection, err := os.ReadFile("../../testdata/corpus/live/selection.json")
	if err != nil {
		t.Fatal(err)
	}
	var record struct {
		SHA256 string `json:"catalogue_sha256"`
	}
	if err := json.Unmarshal(selection, &record); err != nil {
		t.Fatal(err)
	}
	catalog, err := os.ReadFile("../../testdata/corpus/live/catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	if record.SHA256 != fmt.Sprintf("%x", sha256.Sum256(catalog)) {
		t.Fatal("catalogue differs from selection record")
	}
}
