package spam_test

import (
	"bytes"
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/git-pkgs/spam"
)

const keywordRule = "KEYWORD_REPETITION"
const promotionalRule = "PROMOTIONAL_PHRASES"
const linkRule = "LINK_HEAVY"
const lineRule = "REPEATED_LINES"
const urlRule = "REPEATED_URLS"
const readmePath = "README.md"

func TestRawPackageDocuments(t *testing.T) {
	scanner := newScanner(t, spam.Options{})
	for _, tc := range []struct {
		name         string
		count, score int
		keyword      bool
	}{
		{"free-steam-codes-generator.nuspec.txt", 7, 1, true},
		{"controls/forex-python-METADATA.txt", 8, 0, false},
		{"controls/jupyter-METADATA.txt", 6, 0, false},
		{"project-makeover-hack-gems.nuspec.txt", 2, 1, false},
		{"pypi/copilot3d-metadata.txt", 3, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, err := os.ReadFile("testdata/corpus/seed/" + tc.name)
			if err != nil {
				t.Fatal(err)
			}
			doc := spam.Document{Path: tc.name, Data: data}
			result, err := scanner.Scan([]spam.Document{doc})
			if err != nil {
				t.Fatal(err)
			}
			match := matchFor(result, keywordRule)
			if (match != nil) != tc.keyword || measurementFor(t, result, keywordRule).Count != tc.count || result.Score != tc.score {
				t.Fatalf("unexpected raw-text evidence: %+v", result)
			}
			checkEvidence(t, doc, result)
		})
	}
}

func checkEvidence(t *testing.T, doc spam.Document, result spam.Result) {
	t.Helper()
	for _, match := range result.Matches {
		for _, evidence := range match.Evidence {
			if evidence.Start < 0 || evidence.End > len(doc.Data) || evidence.Start >= evidence.End {
				t.Fatalf("invalid evidence: %+v", evidence)
			}
			if !bytes.HasPrefix(doc.Data[evidence.Start:evidence.End], []byte(evidence.Excerpt)) {
				t.Fatalf("excerpt differs: %+v", evidence)
			}
		}
	}
}

func TestSignalFamilies(t *testing.T) {
	var links strings.Builder
	for i := range 10 {
		fmt.Fprintf(&links, "https://example.test/path/%d\n", i)
	}
	for _, tc := range []struct {
		name, text, rule string
		count            int
	}{
		{"promotional cues", "Click here. BUY NOW! Download now.", promotionalRule, 3},
		{"keywords", strings.Repeat("small useful financial package ", 6), keywordRule, 6},
		{"fragment variants", "Contact us: https://example.test/a#one https://EXAMPLE.test/a#two https://example.test/a#part", urlRule, 3},
		{"dense links", links.String(), linkRule, 10},
		{"lines", strings.Repeat("A repeated sentence with enough text.\n", 3), lineRule, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := newScanner(t, spam.Options{}).Scan([]spam.Document{{Path: "input.txt", Data: []byte(tc.text)}})
			if err != nil {
				t.Fatal(err)
			}
			match := matchFor(result, tc.rule)
			if match == nil || match.Evidence[0].Count != tc.count {
				t.Fatalf("expected %s count %d, got %+v", tc.rule, tc.count, result)
			}
		})
	}
}

func TestSingleURLAndOrdinaryPromotion(t *testing.T) {
	for _, text := range []string{
		"https://example.test/a-long-enough-path-to-have-many-regex-endpoints",
		"Contact us for custom development. Read the API docs at https://example.test/docs.",
		"https://example.test/?redirect=https://other.test/?next=https://third.test/",
	} {
		result, err := newScanner(t, spam.Options{}).Scan([]spam.Document{{Path: readmePath, Data: []byte(text)}})
		if err != nil {
			t.Fatal(err)
		}
		if result.Score != 0 {
			t.Fatalf("unexpected evidence for %q: %+v", text, result)
		}
	}
}

func TestDeduplicationAndGroupedContributions(t *testing.T) {
	doc := spam.Document{Path: readmePath, Data: []byte("Download now.\n" + strings.Repeat("https://example.test/repeated/path\n", 12))}
	opts := spam.Options{Weights: map[string]int{lineRule: 2, urlRule: 3, linkRule: 5}}
	result, err := newScanner(t, opts).Scan([]spam.Document{doc, {Path: "copy.txt", Data: doc.Data}})
	if err != nil {
		t.Fatal(err)
	}
	if result.DuplicateDocuments != 1 || result.Score != 7 {
		t.Fatalf("unexpected grouped score: %+v", result)
	}
	total := 0
	for _, match := range result.Matches {
		total += match.Contribution
		if len(match.Evidence) != 1 {
			t.Fatalf("duplicate evidence: %+v", match)
		}
	}
	if total != result.Score {
		t.Fatalf("contributions=%d score=%d", total, result.Score)
	}
}

func TestOfferContext(t *testing.T) {
	for _, tc := range []struct {
		name, text string
		score      int
	}{
		{"offer and destination", "Free coins and unlimited gems. Generate here: https://example.test/rewards", 1},
		{"no destination", "Free coins and unlimited gems. Generate here.", 0},
		{"no cue", "Free coins and unlimited gems. https://example.test/rewards", 0},
		{"overlapping cues and offers", "No survey. No human verification. https://example.test/rewards", 1},
		{"overlapping offers without destination", "No survey. No human verification.", 0},
		{"repeated overlapping offer", "Guaranteed profit. Guaranteed profit. https://example.test/rewards", 1},
		{"one offer", "Unlimited credits. Generate here: https://example.test/rewards", 0},
		{"currency library", "Foreign exchange rates, currency conversion, and market data. Contact us: https://example.test/services", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := spam.Document{Path: readmePath, Data: []byte(tc.text)}
			result, err := newScanner(t, spam.Options{}).Scan([]spam.Document{doc})
			if err != nil {
				t.Fatal(err)
			}
			if result.Score != tc.score {
				t.Fatalf("unexpected offer score: %+v", result)
			}
			checkEvidence(t, doc, result)
		})
	}
}

func TestOfferContextDoesNotCrossDocuments(t *testing.T) {
	result, err := newScanner(t, spam.Options{}).Scan([]spam.Document{
		{Path: "offers.txt", Data: []byte("Free coins and unlimited gems. https://example.test/rewards")},
		{Path: readmePath, Data: []byte("Generate here.")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Score != 0 {
		t.Fatalf("combined unrelated documents: %+v", result)
	}
}

func TestRepetitionBoundaries(t *testing.T) {
	for _, text := range []string{
		strings.Repeat("Classifier: Programming Language :: Python\n", 6),
		strings.Repeat("https://example.test/repeated/path\n", 6),
		strings.Repeat("version>3.0.10\n", 6),
		strings.Repeat("one two\nthree four\n", 6),
	} {
		result, err := newScanner(t, spam.Options{}).Scan([]spam.Document{{Path: "input.txt", Data: []byte(text)}})
		if err != nil {
			t.Fatal(err)
		}
		measurement := measurementFor(t, result, keywordRule)
		if measurement.Count < 6 || measurement.TextCount >= 6 || matchFor(result, keywordRule) != nil {
			t.Fatalf("structural repetition treated as text phrase: %+v", result)
		}
		if result.Score != 0 {
			t.Fatalf("unqualified repetition contributed: %+v", result)
		}
	}
}

func TestRepeatedReferencesRemainMeasurements(t *testing.T) {
	text := strings.Repeat("https://example.test/reference\n", 3)
	result, err := newScanner(t, spam.Options{}).Scan([]spam.Document{{Path: readmePath, Data: []byte(text)}})
	if err != nil {
		t.Fatal(err)
	}
	if measurementFor(t, result, urlRule).Count != 3 || matchFor(result, urlRule) != nil || result.Score != 0 {
		t.Fatalf("references without a promotional cue contributed: %+v", result)
	}
}

func measurementFor(t *testing.T, result spam.Result, id string) spam.Measurement {
	t.Helper()
	for _, item := range result.Measurements {
		if item.Rule == id {
			return item
		}
	}
	t.Fatalf("missing measurement %s", id)
	return spam.Measurement{}
}

func TestWeightsAndConcurrentScans(t *testing.T) {
	weights := map[string]int{promotionalRule: 7, keywordRule: 0}
	scanner := newScanner(t, spam.Options{Weights: weights})
	weights[promotionalRule] = 99
	docs := []spam.Document{{Path: "text", Data: []byte("click here buy now download now " + strings.Repeat("small useful financial package ", 6))}}
	want, err := scanner.Scan(docs)
	if err != nil {
		t.Fatal(err)
	}
	if want.Score != 7 || len(want.Matches) != 2 {
		t.Fatalf("weight override failed: %+v", want)
	}
	if matchFor(want, keywordRule).Contribution != 0 {
		t.Fatal("zero-weight match changed score")
	}
	var group sync.WaitGroup
	for range 8 {
		group.Go(func() {
			got, err := scanner.Scan(docs)
			if err != nil {
				t.Error(err)
				return
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("nondeterministic result: %+v", got)
			}
		})
	}
	group.Wait()
}

func TestInputLimits(t *testing.T) {
	scanner := newScanner(t, spam.Options{})
	for _, docs := range [][]spam.Document{
		{{Data: []byte(strings.Repeat("a", spam.MaxDocumentBytes+1))}},
		{{Data: []byte("\xff")}}, {{Data: []byte("a\x00b")}},
		make([]spam.Document, spam.MaxDocuments+1),
		{{Data: []byte(strings.Repeat("a", spam.MaxDocumentBytes))}, {Data: []byte(strings.Repeat("a", spam.MaxDocumentBytes))}, {Data: []byte(strings.Repeat("a", spam.MaxDocumentBytes))}, {Data: []byte(strings.Repeat("a", spam.MaxDocumentBytes))}, {Data: []byte("a")}},
	} {
		if _, err := scanner.Scan(docs); err == nil {
			t.Fatal("expected input rejection")
		}
	}
	for _, weights := range []map[string]int{{"unknown": 1}, {lineRule: -1}} {
		if _, err := spam.NewScanner(spam.Options{Weights: weights}); err == nil {
			t.Fatal("expected invalid weight rejection")
		}
	}
}

func TestMeasurementsBelowThreshold(t *testing.T) {
	doc := spam.Document{Path: "METADATA", Data: []byte("Contact us at https://example.test/docs.")}
	result, err := newScanner(t, spam.Options{}).Scan([]spam.Document{doc, {Path: "copy.txt", Data: doc.Data}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Score != 0 || len(result.Measurements) != 5 {
		t.Fatalf("missing unscored facts: %+v", result)
	}
	for _, measurement := range result.Measurements {
		if measurement.Rule == promotionalRule && measurement.Count != 1 {
			t.Fatalf("lost promotion count: %+v", measurement)
		}
		if measurement.Rule == linkRule && (measurement.Count != 1 || measurement.URLBytes != len("https://example.test/docs")) {
			t.Fatalf("lost URL measurement: %+v", measurement)
		}
	}
	if len(result.Coverage) != 2 || result.Coverage[1].DuplicateOf != doc.Path || result.Coverage[0].SHA256 != result.Coverage[1].SHA256 {
		t.Fatalf("missing file provenance: %+v", result.Coverage)
	}
}

func TestUnicodeOffsets(t *testing.T) {
	text := "日本語: " + strings.Repeat("café été café hiver ", 6)
	result, err := newScanner(t, spam.Options{}).Scan([]spam.Document{{Path: "text", Data: []byte(text)}})
	if err != nil {
		t.Fatal(err)
	}
	match := matchFor(result, keywordRule)
	if match == nil {
		t.Fatal("missing repeated Unicode phrase")
	}
	evidence := match.Evidence[0]
	if text[evidence.Start:evidence.End] != "café été café hiver" {
		t.Fatalf("incorrect byte offsets: %+v", evidence)
	}
}

func TestCombiningMarksThroughScanFile(t *testing.T) {
	for _, phrase := range []string{"विज्ञापन", "cafe\u0301"} {
		t.Run(phrase, func(t *testing.T) {
			result, err := spam.ScanFile("README.md", []byte(strings.Repeat(phrase+" ", 6)))
			if err != nil {
				t.Fatal(err)
			}
			measurement := measurementFor(t, result, keywordRule)
			if measurement.Count != 3 || measurement.TextCount != 3 || matchFor(result, keywordRule) != nil {
				t.Fatalf("combining marks split words: %+v", result)
			}
		})
	}
	for _, phrase := range []string{"विज्ञापन यहाँ बार बार", "cafe\u0301 ouvert toute anne\u0301e"} {
		t.Run(phrase, func(t *testing.T) {
			text := "日本語: " + strings.Repeat(phrase+" ", 6)
			result, err := spam.ScanFile("README.md", []byte(text))
			if err != nil {
				t.Fatal(err)
			}
			match := matchFor(result, keywordRule)
			if match == nil {
				t.Fatal("missing repeated four-word phrase")
			}
			evidence := match.Evidence[0]
			if text[evidence.Start:evidence.End] != phrase {
				t.Fatalf("incorrect byte offsets: %+v", evidence)
			}
		})
	}
}

func newScanner(t *testing.T, opts spam.Options) *spam.Scanner {
	t.Helper()
	scanner, err := spam.NewScanner(opts)
	if err != nil {
		t.Fatal(err)
	}
	return scanner
}

func matchFor(result spam.Result, id string) *spam.Match {
	for _, match := range result.Matches {
		if match.Rule == id {
			return &match
		}
	}
	return nil
}

func BenchmarkScanREADME(b *testing.B) {
	data, err := os.ReadFile("testdata/corpus/seed/controls/forex-python-METADATA.txt")
	if err != nil {
		b.Fatal(err)
	}
	scanner, err := spam.NewScanner(spam.Options{})
	if err != nil {
		b.Fatal(err)
	}
	docs := []spam.Document{{Path: "METADATA", Data: data}}
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := scanner.Scan(docs); err != nil {
			b.Fatal(err)
		}
	}
}
