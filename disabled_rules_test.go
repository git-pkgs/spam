package spam_test

import (
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/git-pkgs/spam"
)

func TestDisabledRulesPreserveEnabledAnalysis(t *testing.T) {
	baseline := newScanner(t, spam.Options{Rules: []spam.Rule{offerRule()}})
	for _, path := range []string{
		"testdata/corpus/seed/free-steam-codes-generator.nuspec.txt",
		"testdata/corpus/documentation/awesome-standard-README.md.txt",
	} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		want, err := baseline.ScanFile(path, data)
		if err != nil {
			t.Fatal(err)
		}
		for _, keep := range []string{promotionalRule, keywordRule, urlRule, linkRule, lineRule, customRuleID} {
			t.Run(path+"/"+keep, func(t *testing.T) {
				var disabled []string
				for _, rule := range baseline.Rules() {
					if rule.ID != keep && rule.ID != customRuleID {
						disabled = append(disabled, rule.ID)
					}
				}
				scanner := newScanner(t, spam.Options{Rules: []spam.Rule{offerRule()}, DisabledRules: disabled})
				got, err := scanner.ScanFile(path, data)
				if err != nil {
					t.Fatal(err)
				}
				assertEnabledAnalysis(t, got, want, disabled)
			})
		}
	}
}

func assertEnabledAnalysis(t *testing.T, got, want spam.Result, disabled []string) {
	t.Helper()
	measurements := slices.DeleteFunc(slices.Clone(want.Measurements), func(m spam.Measurement) bool {
		return slices.Contains(disabled, m.Rule)
	})
	if !reflect.DeepEqual(got.Measurements, measurements) || !reflect.DeepEqual(got.Coverage, want.Coverage) {
		t.Fatalf("enabled measurements or coverage changed: %+v", got)
	}
	expectedMatches := 0
	for id, weight := range want.Weights {
		if slices.Contains(disabled, id) {
			if _, exists := got.Weights[id]; exists || matchFor(got, id) != nil {
				t.Fatalf("disabled rule retained: %s", id)
			}
			continue
		}
		if actual, exists := got.Weights[id]; !exists || actual != weight {
			t.Fatalf("enabled weight changed: %s", id)
		}
		if original := matchFor(want, id); original != nil {
			expectedMatches++
			match := matchFor(got, id)
			if match == nil || match.Weight != original.Weight || !reflect.DeepEqual(match.Evidence, original.Evidence) {
				t.Fatalf("enabled evidence changed: %s", id)
			}
		}
	}
	if len(got.Matches) != expectedMatches {
		t.Fatalf("unexpected matches: %+v", got.Matches)
	}
}

func TestRepeatedURLsWithPromotionDisabled(t *testing.T) {
	data := []byte("Contact us for support: " + strings.Repeat("https://example.test/support\n", 3))
	scanner := newScanner(t, spam.Options{DisabledRules: []string{promotionalRule, keywordRule, lineRule, linkRule}})
	result, err := scanner.ScanFile(readmePath, data)
	if err != nil {
		t.Fatal(err)
	}
	if result.Score != 1 || len(result.Matches) != 1 || result.Matches[0].Rule != urlRule || measurementFor(t, result, urlRule).Count != 3 {
		t.Fatalf("disabled promotion removed shared cues: %+v", result)
	}
}

func TestAllRulesDisabledPreserveCoverage(t *testing.T) {
	data, err := os.ReadFile("testdata/corpus/seed/free-steam-codes-generator.nuspec.txt")
	if err != nil {
		t.Fatal(err)
	}
	scanner := newScanner(t, spam.Options{DisabledRules: []string{promotionalRule, keywordRule, urlRule, linkRule, lineRule}})
	result, err := scanner.Scan([]spam.Document{{Path: "package.nuspec", Data: data}, {Path: "copy.nuspec", Data: data}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Score != 0 || len(result.Matches) != 0 || len(result.Measurements) != 0 || len(result.Weights) != 0 || len(scanner.Rules()) != 0 {
		t.Fatalf("disabled rules still reported: %+v", result)
	}
	if result.Documents != 2 || result.DuplicateDocuments != 1 || len(result.Coverage) != 2 || result.Coverage[1].DuplicateOf != "package.nuspec" {
		t.Fatalf("disabling rules lost coverage: %+v", result)
	}
}

func TestDisabledWeightConflict(t *testing.T) {
	for _, id := range []string{promotionalRule, customRuleID} {
		for _, weight := range []int{0, 1} {
			_, err := spam.NewScanner(spam.Options{
				Rules: []spam.Rule{offerRule()}, Weights: map[string]int{id: weight}, DisabledRules: []string{id},
			})
			if err == nil || !strings.Contains(err.Error(), id) || !strings.Contains(err.Error(), "weight override and be disabled") {
				t.Fatalf("contradictory options not rejected: %s weight=%d: %v", id, weight, err)
			}
		}
	}
}

func BenchmarkScanDisabledRules(b *testing.B) {
	data, err := os.ReadFile("testdata/corpus/seed/controls/forex-python-METADATA.txt")
	if err != nil {
		b.Fatal(err)
	}
	for _, tc := range []struct {
		name     string
		disabled []string
	}{
		{"default", nil},
		{"without_repetition", []string{keywordRule, lineRule}},
		{"all_disabled", []string{promotionalRule, keywordRule, urlRule, linkRule, lineRule}},
	} {
		b.Run(tc.name, func(b *testing.B) {
			scanner, err := spam.NewScanner(spam.Options{DisabledRules: tc.disabled})
			if err != nil {
				b.Fatal(err)
			}
			b.SetBytes(int64(len(data)))
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				if _, err := scanner.ScanFile("METADATA", data); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
