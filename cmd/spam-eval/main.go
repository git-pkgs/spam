package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/git-pkgs/spam"
	"github.com/git-pkgs/spam/internal/corpus"
)

type output struct {
	PURL            string      `json:"purl,omitempty"`
	Registry        string      `json:"registry"`
	Name            string      `json:"name"`
	Version         string      `json:"version"`
	Label           string      `json:"label"`
	EvaluationGroup string      `json:"evaluation_group"`
	SplitGroup      string      `json:"split_group"`
	Coverage        string      `json:"coverage"`
	Result          spam.Result `json:"result"`
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("spam-eval", flag.ContinueOnError)
	catalog := flags.String("corpus", "", "path to a corpus catalogue JSON file")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *catalog == "" || flags.NArg() != 0 {
		return fmt.Errorf("usage: spam-eval -corpus path/to/catalog.json")
	}
	catalogue, err := corpus.Open(*catalog)
	if err != nil {
		return err
	}
	defer func() { _ = catalogue.Close() }()

	scanner, err := spam.NewScanner(spam.Options{})
	if err != nil {
		return err
	}
	results := make([]output, 0, len(catalogue.Entries))
	for _, item := range catalogue.Entries {
		result, err := evaluate(catalogue, scanner, item)
		if err != nil {
			return fmt.Errorf("%s: %w", item.Name, err)
		}
		results = append(results, result)
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(results)
}

func evaluate(catalogue *corpus.Catalog, scanner *spam.Scanner, item corpus.Entry) (output, error) {
	documents, err := catalogue.Documents(item)
	if err != nil {
		return output{}, err
	}
	result, err := scanner.Scan(documents)
	if err != nil {
		return output{}, err
	}
	return output{PURL: item.PURL(), Registry: item.Registry, Name: item.Name, Version: item.Version,
		Label: item.Label, EvaluationGroup: item.Group, SplitGroup: item.SplitGroup,
		Coverage: "selected raw documents only", Result: result}, nil
}
