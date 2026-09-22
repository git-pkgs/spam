package main

import (
	"encoding/json"
	"os"
	"testing"
)

func TestCompilesOnlyActiveScopes(t *testing.T) {
	data, err := os.ReadFile(candidatePath)
	if err != nil {
		t.Fatal(err)
	}
	var rules []rule
	if err := json.Unmarshal(data, &rules); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		view   string
		scopes []string
	}{
		{rawMode, []string{documentScope, urlScope}},
		{normalizedMode, []string{normalizedScope, rawScope, urlScope}},
	}
	for _, tc := range cases {
		evaluator, _, err := compileRules(rules, tc.view)
		if err != nil {
			t.Fatal(err)
		}
		if len(evaluator.Databases) != len(tc.scopes) || len(evaluator.Scratches) != len(tc.scopes) {
			t.Fatalf("%s: compiled unused scopes", tc.view)
		}
		for _, scope := range tc.scopes {
			if evaluator.Databases[scope] == nil || evaluator.Scratches[scope] == nil {
				t.Fatalf("%s: missing scope %s", tc.view, scope)
			}
		}
	}
}
