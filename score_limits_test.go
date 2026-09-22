package spam_test

import (
	"testing"

	"github.com/git-pkgs/spam"
)

func TestWeightSumCannotOverflowScore(t *testing.T) {
	const maxScore = int(^uint(0) >> 1)
	if _, err := spam.NewScanner(spam.Options{Weights: map[string]int{promotionalRule: maxScore}}); err == nil {
		t.Fatal("accepted weights that could overflow the score")
	}
	rule := offerRule()
	rule.DefaultWeight = maxScore
	if _, err := spam.NewScanner(spam.Options{Rules: []spam.Rule{rule}}); err == nil {
		t.Fatal("accepted custom weight that could overflow the score")
	}
	scanner := newScanner(t, spam.Options{
		Weights: map[string]int{promotionalRule: maxScore}, DisabledRules: []string{urlRule},
	})
	result, err := scanner.ScanFile(readmePath, []byte("Click here. Buy now. Download now."))
	if err != nil {
		t.Fatal(err)
	}
	if result.Score != maxScore || result.Matches[0].Contribution != maxScore {
		t.Fatalf("maximum valid score changed: %+v", result)
	}
}
