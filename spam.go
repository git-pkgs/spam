package spam

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"sync"
	"unicode/utf8"

	"github.com/git-pkgs/scan"
	"github.com/git-pkgs/spam/internal/textutil"
)

const (
	// MaxDocumentBytes limits the size of each UTF-8 document.
	MaxDocumentBytes = 512 << 10
	// MaxPackageBytes limits the total input bytes, including duplicates.
	MaxPackageBytes = 2 << 20
	// MaxDocuments limits the number of documents, including duplicates.
	MaxDocuments     = 64
	ruleset          = "raw-text-6"
	maxEvidence      = 4
	promotionPattern = 1
	urlPattern       = 2
	offerPattern     = 3
	linkHeavyRule    = "LINK_HEAVY"
	keywordRule      = "KEYWORD_REPETITION"
	promotionalRule  = "PROMOTIONAL_PHRASES"
	repeatedURLRule  = "REPEATED_URLS"
	repeatedLineRule = "REPEATED_LINES"
)

// Document contains caller-selected UTF-8 text. Path is a label, never opened.
type Document struct {
	Path string
	Data []byte
}

// Evidence identifies a match using original byte offsets [Start, End).
// Excerpt is a bounded copy of untrusted input text, not safe HTML.
type Evidence struct {
	Path    string `json:"path"`
	Start   int    `json:"start"`
	End     int    `json:"end"`
	Excerpt string `json:"excerpt"`
	Count   int    `json:"count"`
	Detail  string `json:"detail"`
}

// Match records a rule's evidence, configured weight and grouped contribution.
type Match struct {
	Rule         string     `json:"rule"`
	Weight       int        `json:"weight"`
	Contribution int        `json:"contribution"`
	Evidence     []Evidence `json:"evidence"`
}

// Result contains evidence and measurements, without a verdict or threshold.
// Configuration is a SHA-256 identity of definitions, rule order and weights.
type Result struct {
	Ruleset            string         `json:"ruleset"`
	Configuration      string         `json:"configuration"`
	Weights            map[string]int `json:"weights"`
	Score              int            `json:"score"`
	Matches            []Match        `json:"matches"`
	Measurements       []Measurement  `json:"measurements"`
	Coverage           []Coverage     `json:"coverage"`
	Documents          int            `json:"documents"`
	DuplicateDocuments int            `json:"duplicate_documents"`
}

// Measurement records per-document counts, including below-threshold findings.
// Custom expression counts are boolean: zero or one per document.
type Measurement struct {
	Path         string `json:"path"`
	Rule         string `json:"rule"`
	Count        int    `json:"count"`
	TextBytes    int    `json:"text_bytes"`
	URLBytes     int    `json:"url_bytes,omitempty"`
	TextCount    int    `json:"text_count,omitempty"`
	OfferCount   int    `json:"offer_count,omitempty"`
	ContextCount int    `json:"context_count,omitempty"`
}

// Coverage identifies supplied text and whether it was scanned or deduplicated.
type Coverage struct {
	Path        string `json:"path"`
	SHA256      string `json:"sha256"`
	Bytes       int    `json:"bytes"`
	Status      string `json:"status"`
	DuplicateOf string `json:"duplicate_of,omitempty"`
}

// Options configures a scanner. NewScanner copies the supplied definitions and weights.
type Options struct {
	// Rules appends custom expressions after the built-in rules, in this order.
	// Definitions are trusted configuration; compilation has no resource budget.
	Rules []Rule
	// A zero weight removes the score contribution, retaining evidence.
	Weights map[string]int
	// DisabledRules removes rules from matches, measurements and weights.
	// Disabled custom expressions are not compiled or syntax-checked.
	// A disabled rule cannot also appear in Weights.
	DisabledRules []string
}

// Scanner holds compiled rules and supports concurrent scans.
// Construct it with NewScanner; its zero value is not usable.
type Scanner struct {
	db            *scan.Database
	weights       map[string]int
	rules         []Rule
	custom        map[uint]Rule
	configuration string
}

// NewScanner compiles enabled expressions once. IDs must be unique,
// weights nonnegative with their enabled sum fitting in an int, and enabled
// custom expressions must not match empty text.
func NewScanner(opts Options) (*Scanner, error) {
	s := &Scanner{}
	patterns, err := s.configure(opts)
	if err != nil {
		return nil, err
	}
	if len(patterns) > 0 {
		s.db, err = scan.Compile(patterns...)
		if err != nil {
			return nil, fmt.Errorf("compile rules: %w", err)
		}
	}
	return s, nil
}

var defaultScanner = sync.OnceValues(func() (*Scanner, error) {
	return NewScanner(Options{})
})

// ScanFile scans one file with default weights using a shared scanner.
func ScanFile(path string, data []byte) (Result, error) {
	scanner, err := defaultScanner()
	if err != nil {
		return Result{}, err
	}
	return scanner.ScanFile(path, data)
}

// ScanFile scans one file with this scanner's weights.
func (s *Scanner) ScanFile(path string, data []byte) (Result, error) {
	return s.Scan([]Document{{Path: path, Data: data}})
}

// Scan reads input bytes for the duration of the call without modifying or retaining them.
// Callers must not modify the input during a scan. Returned evidence owns its text.
func (s *Scanner) Scan(documents []Document) (Result, error) {
	if err := validateDocuments(documents); err != nil {
		return Result{}, err
	}
	result := Result{Ruleset: ruleset, Configuration: s.configuration, Documents: len(documents), Matches: []Match{}, Weights: make(map[string]int)}
	for id, weight := range s.weights {
		result.Weights[id] = weight
	}
	seen := make(map[[sha256.Size]byte]string)
	evidence := make(map[string][]Evidence)
	var scratch *scan.Scratch
	if s.db != nil {
		scratch = scan.NewScratch(s.db)
	}
	for _, doc := range documents {
		digest := sha256.Sum256(doc.Data)
		coverage := Coverage{Path: doc.Path, SHA256: fmt.Sprintf("%x", digest), Bytes: len(doc.Data), Status: "scanned"}
		if original, exists := seen[digest]; exists {
			result.DuplicateDocuments++
			coverage.Status, coverage.DuplicateOf = "duplicate", original
			result.Coverage = append(result.Coverage, coverage)
			continue
		}
		seen[digest] = doc.Path
		result.Coverage = append(result.Coverage, coverage)
		analysis, err := s.inspect(doc, scratch)
		if err != nil {
			return Result{}, err
		}
		result.Measurements = append(result.Measurements, analysis.measurements...)
		for id, item := range analysis.evidence {
			if len(evidence[id]) < maxEvidence {
				evidence[id] = append(evidence[id], item)
			}
		}
	}
	for _, rule := range s.rules {
		id := rule.ID
		if len(evidence[id]) == 0 {
			continue
		}
		result.Matches = append(result.Matches, Match{Rule: id, Weight: s.weights[id], Evidence: evidence[id]})
	}
	result.Score = assignContributions(result.Matches)
	return result, nil
}

func assignContributions(matches []Match) int {
	groups := map[string]int{}
	for i, match := range matches {
		group := "rule:" + match.Rule
		switch match.Rule {
		case keywordRule, repeatedLineRule:
			group = "repetition"
		case repeatedURLRule, linkHeavyRule:
			group = "links"
		}
		previous, exists := groups[group]
		if !exists || match.Weight > matches[previous].Weight {
			groups[group] = i
		}
	}
	score := 0
	for _, i := range groups {
		matches[i].Contribution = matches[i].Weight
		score += matches[i].Contribution
	}
	return score
}

func validateDocuments(documents []Document) error {
	if len(documents) > MaxDocuments {
		return fmt.Errorf("document count exceeds %d", MaxDocuments)
	}
	total := 0
	for _, doc := range documents {
		if len(doc.Data) > MaxDocumentBytes {
			return fmt.Errorf("document %q exceeds %d bytes", doc.Path, MaxDocumentBytes)
		}
		if !utf8.Valid(doc.Data) || bytes.IndexByte(doc.Data, 0) >= 0 {
			return fmt.Errorf("document %q is not UTF-8 text", doc.Path)
		}
		total += len(doc.Data)
	}
	if total > MaxPackageBytes {
		return fmt.Errorf("package text exceeds %d bytes", MaxPackageBytes)
	}
	return nil
}

func newEvidence(doc Document, start, end, count int, detail string) Evidence {
	return Evidence{Path: doc.Path, Start: start, End: end, Excerpt: textutil.Excerpt(doc.Data[start:end]), Count: count, Detail: detail}
}
