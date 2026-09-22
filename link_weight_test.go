package spam_test

import (
	"os"
	"reflect"
	"testing"

	"github.com/git-pkgs/spam"
)

func TestLinkDensityDefaultAndOverride(t *testing.T) {
	data, err := os.ReadFile("testdata/corpus/documentation/awesome-standard-README.md.txt")
	if err != nil {
		t.Fatal(err)
	}
	doc := spam.Document{Path: readmePath, Data: data}
	baseline, err := newScanner(t, spam.Options{}).Scan([]spam.Document{doc})
	if err != nil {
		t.Fatal(err)
	}
	match := matchFor(baseline, linkRule)
	if baseline.Score != 0 || baseline.Weights[linkRule] != 0 || match == nil || match.Weight != 0 || match.Contribution != 0 || len(match.Evidence) == 0 {
		t.Fatalf("missing zero-weight link evidence: %+v", baseline)
	}
	measurement := measurementFor(t, baseline, linkRule)
	if measurement.Count != 48 || measurement.URLBytes != 2129 || measurement.TextBytes != len(data) {
		t.Fatalf("lost link measurements: %+v", measurement)
	}
	checkEvidence(t, doc, baseline)

	const weight = 3
	weighted, err := newScanner(t, spam.Options{Weights: map[string]int{linkRule: weight}}).Scan([]spam.Document{doc})
	if err != nil {
		t.Fatal(err)
	}
	custom := matchFor(weighted, linkRule)
	if weighted.Score != weight || weighted.Weights[linkRule] != weight || custom == nil || custom.Weight != weight || custom.Contribution != weight {
		t.Fatalf("link weight override failed: %+v", weighted)
	}
	if !reflect.DeepEqual(custom.Evidence, match.Evidence) || !reflect.DeepEqual(weighted.Measurements, baseline.Measurements) || !reflect.DeepEqual(weighted.Coverage, baseline.Coverage) {
		t.Fatal("weight override changed evidence, measurements or coverage")
	}
}
