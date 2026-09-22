package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/git-pkgs/purl"
	"github.com/git-pkgs/spam/internal/corpus"
)

func TestSavedCorpusThroughCommand(t *testing.T) {
	cases := []struct {
		catalog  string
		expected map[string]bool
	}{
		{"../../testdata/corpus/seed/catalog.json", map[string]bool{}},
		{"../../testdata/corpus/validation/catalog.json", map[string]bool{}},
		{"../../testdata/corpus/validation/stress-catalog.json", map[string]bool{"SA_FREE_PORN": true, numericHostRule: true, ipLinkRule: true}},
	}
	for _, tc := range cases {
		t.Run(tc.catalog, func(t *testing.T) {
			var outputBytes bytes.Buffer
			if err := run([]string{rulesFlag, candidatePath, corpusFlag, tc.catalog}, &outputBytes); err != nil {
				t.Fatal(err)
			}
			var got output
			if err := json.Unmarshal(outputBytes.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if got.FixturesChecked == 0 || got.Comparisons == 0 {
				t.Fatal("no fixture or reference comparisons")
			}
			actual := verifyCorpusOutput(t, tc.catalog, got)
			if len(actual) != len(tc.expected) {
				t.Fatalf("unexpected hits: %v", actual)
			}
			for id := range tc.expected {
				if !actual[id] {
					t.Fatalf("missing %s", id)
				}
			}
			var repeated bytes.Buffer
			if err := run([]string{rulesFlag, candidatePath, corpusFlag, tc.catalog}, &repeated); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(outputBytes.Bytes(), repeated.Bytes()) {
				t.Fatal("nondeterministic output")
			}
		})
	}
}

func TestProvenanceAndSchema(t *testing.T) {
	data, err := os.ReadFile(candidatePath)
	if err != nil {
		t.Fatal(err)
	}
	var candidates []rule
	if err := json.Unmarshal(data, &candidates); err != nil {
		t.Fatal(err)
	}
	for _, r := range candidates {
		source, err := os.ReadFile(filepath.Join("../../rules/upstream/sources", r.Project, r.File))
		if err != nil {
			t.Fatal(err)
		}
		if fmt.Sprintf("%x", sha256.Sum256(source)) != r.SourceSHA256 {
			t.Fatalf("source hash changed: %s", r.ID)
		}
		if !strings.Contains(string(source), r.UpstreamID) {
			t.Fatalf("upstream ID absent: %s", r.ID)
		}
	}
	temp := t.TempDir()
	path := filepath.Join(temp, "rules.json")
	cases := map[string][]rule{
		"duplicate": {candidates[0], candidates[0]},
		"empty":     {},
	}
	enabled := candidates[0]
	enabled.Enabled = true
	cases["enabled"] = []rule{enabled}
	weighted := candidates[0]
	weighted.DefaultWeight = 1
	cases["weighted"] = []rule{weighted}
	unattributed := candidates[0]
	unattributed.SourceSHA256 = ""
	cases["missing provenance"] = []rule{unattributed}
	wrongFixture := candidates[0]
	wrongFixture.Fixtures = []fixture{{Text: "ordinary documentation", Match: true}}
	cases["fixture mismatch"] = []rule{wrongFixture}
	invalid := candidates[0]
	invalid.Expression = "(?=unsupported)"
	cases["unsupported"] = []rule{invalid}
	for name, rules := range cases {
		t.Run(name, func(t *testing.T) {
			encoded, err := json.Marshal(rules)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, encoded, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := run([]string{rulesFlag, path, corpusFlag, "../../testdata/corpus/validation/catalog.json"}, &bytes.Buffer{}); err == nil {
				t.Fatal("invalid candidate accepted")
			}
		})
	}
}

func verifyCorpusOutput(t *testing.T, catalogPath string, got output) map[string]bool {
	t.Helper()
	catalogData, err := os.ReadFile(catalogPath)
	if err != nil {
		t.Fatal(err)
	}
	var catalog []corpus.Entry
	if err := json.Unmarshal(catalogData, &catalog); err != nil {
		t.Fatal(err)
	}
	if len(got.Rows) != len(catalog) {
		t.Fatal("missing packages")
	}
	actual := map[string]bool{}
	for i, row := range got.Rows {
		verifyCorpusRow(t, filepath.Dir(catalogPath), catalog[i], row)
		for _, h := range row.Hits {
			actual[h.Rule] = true
		}
	}
	return actual
}

func verifyCorpusRow(t *testing.T, root string, item corpus.Entry, row row) {
	t.Helper()
	if row.Registry != item.Registry || row.Name != item.Name || row.Version != item.Version || row.Label != item.Label || row.Group != item.Group || row.SplitGroup != item.SplitGroup {
		t.Fatal("package identity or label changed")
	}
	if row.PURL == "" || row.PURL != purl.MakePURLString(item.Registry, item.Name, item.Version) {
		t.Fatalf("wrong package URL: %q", row.PURL)
	}
	if _, err := purl.Parse(row.PURL); err != nil {
		t.Fatal(err)
	}
	if len(row.Coverage) != len(item.Documents) {
		t.Fatal("missing coverage")
	}
	originals := map[string][]byte{}
	for _, doc := range item.Documents {
		data, err := os.ReadFile(filepath.Join(root, doc.Path))
		if err != nil {
			t.Fatal(err)
		}
		originals[doc.Member] = data
	}
	for _, file := range row.Coverage {
		data := originals[file.Path]
		if file.Bytes != len(data) || file.SHA256 != fmt.Sprintf("%x", sha256.Sum256(data)) {
			t.Fatalf("wrong coverage: %s", file.Path)
		}
	}
	for _, h := range row.Hits {
		data := originals[h.Path]
		if h.Start < 0 || h.End > len(data) || h.Start >= h.End || !bytes.HasPrefix(data[h.Start:h.End], []byte(h.Excerpt)) {
			t.Fatalf("wrong original evidence: %+v", h)
		}
	}
}

func runTestCorpus(t *testing.T, rules, catalog, view string) output {
	t.Helper()
	var stdout bytes.Buffer
	if err := run([]string{rulesFlag, rules, corpusFlag, catalog, "-view", view}, &stdout); err != nil {
		t.Fatal(err)
	}
	var result output
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

const (
	corpusFlag      = "-corpus"
	ipLinkRule      = "SA_IP_LINK_PLUS"
	moneyBackRule   = "SA_MONEY_BACK"
	legitimateLabel = "legitimate"
	datasetPackage  = "@stdlib/datasets-spam-assassin"
)

const (
	rulesFlag        = "-rules"
	numericHostRule  = "SA_NUMERIC_HTTP_ADDR"
	promotionalLabel = "promotional_spam"
	candidatePath    = "../../rules/upstream/candidates.json"
)

const (
	clickHereRule       = "SA___CLICK_HERE"
	secondCandidatePath = "../../rules/upstream/second-candidates.json"
	rawMode             = "raw"
)
