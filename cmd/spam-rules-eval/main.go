package main

import (
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/git-pkgs/scan"
	"github.com/git-pkgs/spam/internal/corpus"
	"github.com/git-pkgs/spam/internal/textutil"
)

type fixture struct {
	Text  string `json:"text"`
	Match bool   `json:"match"`
}

type rule struct {
	ID            string    `json:"id"`
	Project       string    `json:"project"`
	Revision      string    `json:"revision"`
	File          string    `json:"file"`
	SourceSHA256  string    `json:"source_sha256"`
	UpstreamID    string    `json:"upstream_id"`
	Description   string    `json:"description"`
	Category      string    `json:"category"`
	License       string    `json:"license"`
	Expression    string    `json:"expression"`
	Scope         string    `json:"scope"`
	UpstreamInput string    `json:"upstream_input"`
	DefaultWeight int       `json:"default_weight"`
	Enabled       bool      `json:"enabled_by_default"`
	Fixtures      []fixture `json:"fixtures"`
}

type hit struct {
	Rule          string       `json:"rule"`
	Path          string       `json:"path"`
	Start         int          `json:"start"`
	End           int          `json:"end"`
	Excerpt       string       `json:"excerpt"`
	View          string       `json:"view,omitempty"`
	MatchedText   string       `json:"matched_text,omitempty"`
	OriginalSpans []sourceSpan `json:"original_spans,omitempty"`
}

type coverage struct {
	Path                 string `json:"path"`
	SHA256               string `json:"sha256"`
	Bytes                int    `json:"bytes"`
	NormalizedParagraphs int    `json:"normalized_paragraphs,omitempty"`
	NormalizedBytes      int    `json:"normalized_bytes,omitempty"`
}

type row struct {
	PURL       string     `json:"purl,omitempty"`
	Registry   string     `json:"registry"`
	Name       string     `json:"name"`
	Version    string     `json:"version"`
	Label      string     `json:"label"`
	Group      string     `json:"evaluation_group"`
	SplitGroup string     `json:"split_group"`
	Hits       []hit      `json:"hits"`
	Coverage   []coverage `json:"coverage"`
}

type output struct {
	RulesSHA256     string `json:"rules_sha256"`
	CorpusSHA256    string `json:"corpus_sha256"`
	Mode            string `json:"mode"`
	RuleCount       int    `json:"rule_count"`
	FixturesChecked int    `json:"fixtures_checked"`
	Comparisons     int    `json:"scan_regexp_comparisons"`
	Rows            []row  `json:"rows"`
}

type engine struct {
	Rules       []rule
	References  []*regexp.Regexp
	Databases   map[string]*scan.Database
	Scratches   map[string]*scan.Scratch
	Comparisons int
	View        string
}

var urls = regexp.MustCompile(`(?i)\b(?:https?|file)://[^\s<>"` + "`" + `'\[\]{}()]+`)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("spam-rules-eval", flag.ContinueOnError)
	rulesPath := flags.String("rules", "", "candidate rules JSON")
	corpusPath := flags.String("corpus", "", "saved package catalogue JSON")
	view := flags.String("view", "raw", "raw or normalized text")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *rulesPath == "" || *corpusPath == "" || flags.NArg() != 0 {
		return fmt.Errorf("require -rules and -corpus")
	}
	if *view != "raw" && *view != normalizedMode {
		return fmt.Errorf("unknown view %q", *view)
	}
	rulesData, err := corpus.ReadJSON(*rulesPath)
	if err != nil {
		return err
	}
	var rules []rule
	if err := json.Unmarshal(rulesData, &rules); err != nil {
		return err
	}
	evaluator, fixtures, err := compileRules(rules, *view)
	if err != nil {
		return err
	}
	catalogue, err := corpus.Open(*corpusPath)
	if err != nil {
		return err
	}
	defer func() { _ = catalogue.Close() }()
	result := output{RulesSHA256: fmt.Sprintf("%x", sha256.Sum256(rulesData)), CorpusSHA256: catalogue.SHA256, Mode: viewMode(*view), RuleCount: len(rules), FixturesChecked: fixtures, Rows: []row{}}
	for _, item := range catalogue.Entries {
		record, err := evaluator.evaluate(catalogue, item)
		if err != nil {
			return fmt.Errorf("%s: %w", item.Name, err)
		}
		result.Rows = append(result.Rows, record)
	}
	result.Comparisons = evaluator.Comparisons
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

func compileRules(rules []rule, view string) (*engine, int, error) {
	if len(rules) == 0 {
		return nil, 0, fmt.Errorf("empty rules")
	}
	e := &engine{Rules: rules, View: view, Databases: map[string]*scan.Database{}, Scratches: map[string]*scan.Scratch{}}
	scopes := []string{documentScope, urlScope}
	if view == normalizedMode {
		scopes = []string{normalizedScope, rawScope, urlScope}
	}
	patterns := map[string][]*scan.Pattern{}
	seen := map[string]bool{}
	fixtureCount := 0
	for i, r := range rules {
		if err := r.validate(seen[r.ID]); err != nil {
			return nil, 0, err
		}
		seen[r.ID] = true
		pattern, reference, err := r.compile(uint(i + 1))
		if err != nil {
			return nil, 0, err
		}
		fixtureCount += len(r.Fixtures)
		e.References = append(e.References, reference)
		for _, scope := range scopes {
			if r.inScope(scope) {
				patterns[scope] = append(patterns[scope], pattern)
			}
		}
	}
	for _, scope := range scopes {
		if len(patterns[scope]) == 0 {
			continue
		}
		db, err := scan.Compile(patterns[scope]...)
		if err != nil {
			return nil, 0, err
		}
		e.Databases[scope] = db
		e.Scratches[scope] = scan.NewScratch(db)
	}
	return e, fixtureCount, nil
}

func (r rule) validate(duplicate bool) error {
	if r.ID == "" || duplicate || (r.Scope != documentScope && r.Scope != urlScope) || r.DefaultWeight != 0 || r.Enabled {
		return fmt.Errorf("invalid candidate %q", r.ID)
	}
	if !supportedInput(r) {
		return fmt.Errorf("unsupported input %s", r.ID)
	}
	if r.Project == "" || r.Revision == "" || r.File == "" || r.SourceSHA256 == "" || r.License == "" || r.Description == "" || r.Category == "" || r.UpstreamID == "" {
		return fmt.Errorf("missing provenance %s", r.ID)
	}
	return nil
}

func (r rule) compile(id uint) (*scan.Pattern, *regexp.Regexp, error) {
	reference, err := regexp.Compile(r.Expression)
	if err != nil {
		return nil, nil, fmt.Errorf("%s regexp: %w", r.ID, err)
	}
	if reference.MatchString("") {
		return nil, nil, fmt.Errorf("empty match %s", r.ID)
	}
	pattern := &scan.Pattern{ID: id, Expression: r.Expression, Flags: scan.SomLeftMost | scan.SingleMatch | scan.UTF8}
	db, err := scan.Compile(pattern)
	if err != nil {
		return nil, nil, fmt.Errorf("%s scan: %w", r.ID, err)
	}
	if err := r.checkFixtures(db, reference); err != nil {
		return nil, nil, err
	}
	return pattern, reference, nil
}

func (r rule) checkFixtures(db *scan.Database, reference *regexp.Regexp) error {
	positive, negative := false, false
	for _, f := range r.Fixtures {
		matched, err := db.Match([]byte(f.Text), nil)
		if err != nil {
			return err
		}
		if matched != f.Match || reference.MatchString(f.Text) != f.Match {
			return fmt.Errorf("fixture mismatch %s: %q", r.ID, f.Text)
		}
		positive = positive || f.Match
		negative = negative || !f.Match
	}
	if !positive || !negative {
		return fmt.Errorf("missing positive/negative fixtures %s", r.ID)
	}
	return nil
}

func (e *engine) evaluate(catalogue *corpus.Catalog, item corpus.Entry) (row, error) {
	result := row{PURL: item.PURL(), Registry: item.Registry, Name: item.Name, Version: item.Version, Label: item.Label, Group: item.Group, SplitGroup: item.SplitGroup, Hits: []hit{}, Coverage: []coverage{}}
	documents, err := catalogue.Documents(item)
	if err != nil {
		return result, err
	}
	for _, document := range documents {
		data := document.Data
		fileCoverage := coverage{Path: document.Path, SHA256: fmt.Sprintf("%x", sha256.Sum256(data)), Bytes: len(data)}
		var found []hit
		if e.View == normalizedMode {
			found, fileCoverage.NormalizedParagraphs, fileCoverage.NormalizedBytes, err = e.normalizedMatches(data, document.Path)
		} else {
			found, err = e.match(data, documentScope, document.Path, 0)
		}
		if err != nil {
			return result, err
		}
		result.Coverage = append(result.Coverage, fileCoverage)
		result.Hits = append(result.Hits, found...)
		seen := map[string]bool{}
		for _, span := range urls.FindAllIndex(data, -1) {
			text := strings.TrimRight(string(data[span[0]:span[1]]), ".,;:!?")
			found, err := e.match([]byte(text), urlScope, document.Path, span[0])
			if err != nil {
				return result, err
			}
			for _, h := range found {
				if !seen[h.Rule] {
					result.Hits = append(result.Hits, h)
					seen[h.Rule] = true
				}
			}
		}
	}
	return result, nil
}

func (e *engine) match(data []byte, scope, path string, offset int) ([]hit, error) {
	found := []hit{}
	db := e.Databases[scope]
	if db == nil {
		return found, nil
	}
	matched := map[uint]bool{}
	err := db.Scan(data, e.Scratches[scope], func(m scan.Match) error {
		if m.ID == 0 || int(m.ID) > len(e.Rules) || m.From >= m.To || m.To > uint64(len(data)) {
			return fmt.Errorf("invalid scan evidence")
		}
		matched[m.ID] = true
		found = append(found, hit{Rule: e.Rules[m.ID-1].ID, Path: path, Start: offset + int(m.From), End: offset + int(m.To), Excerpt: textutil.Excerpt(data[m.From:m.To])})
		return nil
	})
	if err != nil {
		return nil, err
	}
	for i, r := range e.Rules {
		if !r.inScope(scope) {
			continue
		}
		e.Comparisons++
		if e.References[i].Match(data) != matched[uint(i+1)] {
			return nil, fmt.Errorf("scan/regexp disagreement %s in %s", r.ID, path)
		}
	}
	return found, nil
}

const (
	normalizedMode  = "normalized"
	documentScope   = "document"
	urlScope        = "url"
	normalizedScope = "normalized_document"
	rawScope        = "raw_document"
)
