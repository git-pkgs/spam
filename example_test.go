package spam_test

import (
	"fmt"
	"log"

	"github.com/git-pkgs/spam"
)

func ExampleScanFile() {
	result, err := spam.ScanFile("package.json", []byte(`{"description":"Click here. Buy now. Download now."}`))
	if err != nil {
		log.Fatal(err)
	}
	for _, match := range result.Matches {
		fmt.Println(match.Rule, match.Contribution, match.Evidence[0].Path)
	}
	// Output:
	// PROMOTIONAL_PHRASES 1 package.json
}

func ExampleNewScanner() {
	scanner, err := spam.NewScanner(spam.Options{
		Rules: []spam.Rule{{
			ID: "LOCAL_OFFER", Description: "Exclusive offer phrase", Category: "promotion",
			Expression: `(?i)\bexclusive offer\b`, DefaultWeight: 1, Provenance: "local policy",
		}},
		Weights: map[string]int{"LOCAL_OFFER": 2},
	})
	if err != nil {
		log.Fatal(err)
	}
	result, err := scanner.ScanFile("README.md", []byte("Exclusive offer. Exclusive offer."))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(result.Score, result.Matches[0].Evidence[0].Count)
	// Output:
	// 2 1
}

func ExampleScanner_Scan() {
	scanner, err := spam.NewScanner(spam.Options{
		Weights: map[string]int{"PROMOTIONAL_PHRASES": 2},
	})
	if err != nil {
		log.Fatal(err)
	}
	result, err := scanner.Scan([]spam.Document{
		{Path: "package.json", Data: []byte(`{"description":"An example package"}`)},
		{Path: "README.md", Data: []byte("Click here. Buy now. Download now.")},
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(result.Documents, result.Score)
	// Output:
	// 2 2
}

func ExampleScanner_Rules() {
	scanner, err := spam.NewScanner(spam.Options{
		DisabledRules: []string{"KEYWORD_REPETITION", "REPEATED_URLS", "LINK_HEAVY", "REPEATED_LINES"},
		Weights:       map[string]int{"PROMOTIONAL_PHRASES": 2},
	})
	if err != nil {
		log.Fatal(err)
	}
	for _, rule := range scanner.Rules() {
		fmt.Println(rule.ID, rule.Category, rule.DefaultWeight)
	}
	// Output:
	// PROMOTIONAL_PHRASES promotion 1
}
