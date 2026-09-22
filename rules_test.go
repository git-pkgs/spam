package spam_test

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/git-pkgs/spam"
)

const customRuleID = "LOCAL_OFFER"
const groupCollisionID = "links"
const secondCustomRuleID = "SECOND"

func offerRule() spam.Rule {
	return spam.Rule{
		ID: customRuleID, Description: "Free Steam code offer", Category: "promotion",
		Expression: `(?i)\bfree steam codes\b`, DefaultWeight: 2, Provenance: "test policy",
	}
}

func TestCustomRuleOnSavedPackage(t *testing.T) {
	scanner := newScanner(t, spam.Options{Rules: []spam.Rule{offerRule()}})
	for _, tc := range []struct {
		path  string
		score int
		count int
	}{
		{"free-steam-codes-generator.nuspec.txt", 3, 1},
		{"controls/forex-python-METADATA.txt", 0, 0},
	} {
		t.Run(tc.path, func(t *testing.T) {
			data, err := os.ReadFile("testdata/corpus/seed/" + tc.path)
			if err != nil {
				t.Fatal(err)
			}
			got, err := scanner.ScanFile(tc.path, data)
			if err != nil {
				t.Fatal(err)
			}
			if got.Score != tc.score || measurementFor(t, got, customRuleID).Count != tc.count {
				t.Fatalf("unexpected custom result: %+v", got)
			}
			checkEvidence(t, spam.Document{Path: tc.path, Data: data}, got)
		})
	}
}

func TestCustomRuleBooleanMatchesAndOffsets(t *testing.T) {
	rule := offerRule()
	rule.Expression = `café [0-9]+`
	scanner := newScanner(t, spam.Options{Rules: []spam.Rule{rule}})
	data := []byte("日本語: café 123 café 456")
	result, err := scanner.ScanFile(readmePath, data)
	if err != nil {
		t.Fatal(err)
	}
	match := matchFor(result, rule.ID)
	if match == nil || match.Contribution != rule.DefaultWeight || measurementFor(t, result, rule.ID).Count != 1 {
		t.Fatalf("expected one contribution for repeated and variable-length matches: %+v", result)
	}
	evidence := match.Evidence[0]
	if evidence.Count != 1 || evidence.Start != len("日本語: ") || evidence.Excerpt != "café 1" {
		t.Fatalf("expected first matching endpoint: %+v", evidence)
	}
	checkEvidence(t, spam.Document{Path: readmePath, Data: data}, result)
	clear(data)
	if evidence.Excerpt != "café 1" {
		t.Fatal("custom evidence retained input bytes")
	}
}

func TestCustomRulesOrderBoundsAndIsolation(t *testing.T) {
	rules := []spam.Rule{offerRule(), offerRule()}
	rules[0].ID = groupCollisionID
	rules[1].ID = secondCustomRuleID
	weights := map[string]int{groupCollisionID: 3, secondCustomRuleID: 0}
	scanner := newScanner(t, spam.Options{Rules: rules, Weights: weights})
	rules[0].Expression = "changed"
	weights[groupCollisionID] = 99
	metadata := scanner.Rules()
	metadata[0].ID = "CHANGED_ID"
	metadata[len(metadata)-1].Expression = "modified expression"
	var docs []spam.Document
	for _, prefix := range []string{"one", "two", "three", "four", "five"} {
		docs = append(docs, spam.Document{Path: prefix, Data: []byte(prefix + " free steam codes. Contact us: https://example.test/a https://example.test/a https://example.test/a")})
	}
	docs = append(docs, spam.Document{Path: "copy", Data: docs[0].Data})
	want, err := scanner.Scan(docs)
	if err != nil {
		t.Fatal(err)
	}
	if want.Score != 4 || want.DuplicateDocuments != 1 {
		t.Fatalf("custom rule collided with built-in link group or counted duplicates: %+v", want)
	}
	last := want.Matches[len(want.Matches)-2:]
	if last[0].Rule != groupCollisionID || last[1].Rule != secondCustomRuleID || last[1].Contribution != 0 || len(last[0].Evidence) != 4 {
		t.Fatalf("incorrect rule order or evidence bounds: %+v", last)
	}
	if got := scanner.Rules(); got[0].ID != promotionalRule || got[len(got)-1].Expression != offerRule().Expression {
		t.Fatalf("metadata mutation changed rules: %+v", got)
	}
	var workers sync.WaitGroup
	for range 8 {
		workers.Go(func() {
			got, err := scanner.Scan(docs)
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Errorf("concurrent scan changed: %v, %+v", err, got)
			}
		})
	}
	workers.Wait()
}

func TestDisabledRules(t *testing.T) {
	rule := offerRule()
	rule.Expression = "["
	scanner := newScanner(t, spam.Options{
		Rules: []spam.Rule{rule}, DisabledRules: []string{customRuleID, promotionalRule},
	})
	result, err := scanner.ScanFile(readmePath, []byte("Click here. Buy now. Download now."))
	if err != nil {
		t.Fatal(err)
	}
	if result.Score != 0 || len(result.Matches) != 0 || len(result.Measurements) != 4 || len(scanner.Rules()) != 4 {
		t.Fatalf("disabled rules still reported: %+v", result)
	}
	if _, exists := result.Weights[promotionalRule]; exists {
		t.Fatal("disabled weight retained")
	}
	if _, err := spam.NewScanner(spam.Options{Rules: []spam.Rule{rule}}); err == nil {
		t.Fatal("invalid expression accepted when enabled")
	}
}

func TestInvalidRuleDefinitions(t *testing.T) {
	for _, field := range []string{"id", "description", "category", "provenance", "expression", "weight"} {
		t.Run(field, func(t *testing.T) {
			rule := offerRule()
			switch field {
			case "id":
				rule.ID = " "
			case "description":
				rule.Description = ""
			case "category":
				rule.Category = ""
			case "provenance":
				rule.Provenance = ""
			case "expression":
				rule.Expression = ""
			case "weight":
				rule.DefaultWeight = -1
			}
			if _, err := spam.NewScanner(spam.Options{Rules: []spam.Rule{rule}}); err == nil {
				t.Fatal("invalid rule accepted")
			}
		})
	}
	for _, expression := range []string{"[", "a*", `(?=offer)`, `(a)\1`} {
		rule := offerRule()
		rule.Expression = expression
		if _, err := spam.NewScanner(spam.Options{Rules: []spam.Rule{rule}}); err == nil {
			t.Fatalf("unsupported expression accepted: %q", expression)
		}
	}
	builtinCollision := offerRule()
	builtinCollision.ID = promotionalRule
	for _, opts := range []spam.Options{
		{Rules: []spam.Rule{offerRule(), offerRule()}},
		{Rules: []spam.Rule{builtinCollision}},
		{DisabledRules: []string{"missing"}},
		{DisabledRules: []string{linkRule, linkRule}},
		{Rules: []spam.Rule{offerRule()}, Weights: map[string]int{customRuleID: -1}},
	} {
		if _, err := spam.NewScanner(opts); err == nil {
			t.Fatalf("invalid options accepted: %+v", opts)
		}
	}
}

func TestRuleConfigurationIdentity(t *testing.T) {
	identity := func(opts spam.Options) string {
		t.Helper()
		result, err := newScanner(t, opts).Scan(nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Configuration) != 64 {
			t.Fatalf("missing configuration hash: %+v", result)
		}
		return result.Configuration
	}
	base := identity(spam.Options{})
	if base != identity(spam.Options{Weights: map[string]int{promotionalRule: 1}}) {
		t.Fatal("equivalent weights changed identity")
	}
	custom := identity(spam.Options{Rules: []spam.Rule{offerRule()}})
	changed := offerRule()
	changed.Expression = "different offer"
	for _, opts := range []spam.Options{
		{},
		{Rules: []spam.Rule{changed}},
		{Rules: []spam.Rule{offerRule()}, Weights: map[string]int{customRuleID: 0}},
		{Rules: []spam.Rule{offerRule()}, DisabledRules: []string{linkRule}},
	} {
		if identity(opts) == custom {
			t.Fatalf("different configuration has same identity: %+v", opts)
		}
	}
	if identity(spam.Options{DisabledRules: []string{linkRule, lineRule}}) != identity(spam.Options{DisabledRules: []string{lineRule, linkRule}}) {
		t.Fatal("disable-list ordering changed identity")
	}
	second := offerRule()
	second.ID = secondCustomRuleID
	if identity(spam.Options{Rules: []spam.Rule{offerRule(), second}}) == identity(spam.Options{Rules: []spam.Rule{second, offerRule()}}) {
		t.Fatal("rule order missing from identity")
	}
}

func TestRulesFromJSON(t *testing.T) {
	var rules []spam.Rule
	err := json.Unmarshal([]byte(`[{"id":"LOCAL_OFFER","description":"Offer phrase","category":"promotion","expression":"(?i)exclusive offer","default_weight":2,"provenance":"local policy"}]`), &rules)
	if err != nil {
		t.Fatal(err)
	}
	result, err := newScanner(t, spam.Options{Rules: rules, Weights: map[string]int{customRuleID: 3}}).ScanFile("package.json", []byte(`{"description":"EXCLUSIVE OFFER"}`))
	if err != nil || result.Score != 3 || !strings.Contains(matchFor(result, customRuleID).Evidence[0].Excerpt, "EXCLUSIVE") {
		t.Fatalf("JSON rules were not used: %v, %+v", err, result)
	}
}
