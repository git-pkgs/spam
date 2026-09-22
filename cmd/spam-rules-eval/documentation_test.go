package main

import "testing"

func TestDocumentationAndLiveCorporaThroughCommand(t *testing.T) {
	for _, name := range []string{"documentation", "live"} {
		catalog := "../../testdata/corpus/" + name + "/catalog.json"
		for _, rules := range []string{candidatePath, secondCandidatePath} {
			for _, view := range []string{rawMode, normalizedMode} {
				t.Run(name+"/"+rules+"/"+view, func(t *testing.T) {
					result := runTestCorpus(t, rules, catalog, view)
					verifyCorpusOutput(t, catalog, result)
					if got := packageRulePairs(result); len(got) != 0 {
						t.Fatalf("unexpected upstream matches: %v", got)
					}
				})
			}
		}
	}
}
