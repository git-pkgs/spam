package spam

import (
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/git-pkgs/scan"
)

// Rule describes a built-in signal or a caller-supplied raw-text expression.
// Expression is empty for built-in structural rules. Custom expressions use
// git-pkgs/scan syntax with UTF-8 matching; inline flags such as (?i) are supported.
// Provenance records the author or upstream source and any adaptations.
type Rule struct {
	ID            string `json:"id"`
	Description   string `json:"description"`
	Category      string `json:"category"`
	Expression    string `json:"expression,omitempty"`
	DefaultWeight int    `json:"default_weight"`
	Provenance    string `json:"provenance"`
}

//go:embed rules/builtin.json
var builtinData []byte

type builtinDefinitions struct {
	Rules    []Rule `json:"rules"`
	Patterns []struct {
		ID         uint   `json:"id"`
		Expression string `json:"expression"`
	} `json:"patterns"`
}

// Rules returns an independent copy of the enabled definitions in match order.
// DefaultWeight is the definition's weight before Options.Weights overrides.
func (s *Scanner) Rules() []Rule {
	return slices.Clone(s.rules)
}

func (s *Scanner) configure(opts Options) ([]*scan.Pattern, error) {
	var builtin builtinDefinitions
	if err := json.Unmarshal(builtinData, &builtin); err != nil {
		return nil, fmt.Errorf("built-in rules: %w", err)
	}
	definitions := slices.Concat(builtin.Rules, opts.Rules)
	s.weights = make(map[string]int, len(definitions))
	for i, rule := range definitions {
		if err := validateRule(rule, i >= len(builtin.Rules)); err != nil {
			return nil, err
		}
		if _, exists := s.weights[rule.ID]; exists {
			return nil, fmt.Errorf("duplicate rule %q", rule.ID)
		}
		s.weights[rule.ID] = rule.DefaultWeight
	}
	if err := s.configureWeights(opts); err != nil {
		return nil, err
	}
	patterns := make([]*scan.Pattern, 0, len(builtin.Patterns)+len(opts.Rules))
	_, promotion := s.weights[promotionalRule]
	_, repeatedURL := s.weights[repeatedURLRule]
	_, linkDensity := s.weights[linkHeavyRule]
	needed := map[uint]bool{
		promotionPattern: promotion || repeatedURL,
		urlPattern:       promotion || repeatedURL || linkDensity,
		offerPattern:     promotion,
	}
	var nextID uint
	for _, pattern := range builtin.Patterns {
		nextID = max(nextID, pattern.ID)
		if needed[pattern.ID] {
			patterns = append(patterns, &scan.Pattern{ID: pattern.ID, Expression: pattern.Expression, Flags: scan.SomLeftMost})
		}
	}
	s.custom = make(map[uint]Rule)
	for _, rule := range definitions {
		if _, enabled := s.weights[rule.ID]; !enabled {
			continue
		}
		s.rules = append(s.rules, rule)
		if rule.Expression != "" {
			nextID++
			s.custom[nextID] = rule
			patterns = append(patterns, &scan.Pattern{ID: nextID, Expression: rule.Expression,
				Flags: scan.SomLeftMost | scan.SingleMatch | scan.UTF8})
		}
	}
	identity, err := json.Marshal(struct {
		Ruleset string          `json:"ruleset"`
		Builtin json.RawMessage `json:"builtin"`
		Rules   []Rule          `json:"rules"`
		Weights map[string]int  `json:"weights"`
	}{ruleset, builtinData, s.rules, s.weights})
	if err != nil {
		return nil, err
	}
	s.configuration = fmt.Sprintf("%x", sha256.Sum256(identity))
	return patterns, nil
}

func validateRule(rule Rule, custom bool) error {
	if strings.TrimSpace(rule.ID) == "" || strings.TrimSpace(rule.Description) == "" ||
		strings.TrimSpace(rule.Category) == "" || strings.TrimSpace(rule.Provenance) == "" {
		return fmt.Errorf("rule %q requires an ID, description, category and provenance", rule.ID)
	}
	if custom && rule.Expression == "" {
		return fmt.Errorf("rule %q requires an expression", rule.ID)
	}
	if rule.DefaultWeight < 0 {
		return fmt.Errorf("rule %q has negative default weight", rule.ID)
	}
	return nil
}

func (s *Scanner) configureWeights(opts Options) error {
	ids := make([]string, 0, len(opts.Weights))
	for id := range opts.Weights {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	for _, id := range ids {
		if _, exists := s.weights[id]; !exists {
			return fmt.Errorf("unknown rule %q", id)
		}
		if opts.Weights[id] < 0 {
			return fmt.Errorf("rule %q has negative weight", id)
		}
		s.weights[id] = opts.Weights[id]
	}
	for _, id := range opts.DisabledRules {
		if _, exists := s.weights[id]; !exists {
			return fmt.Errorf("unknown or duplicate disabled rule %q", id)
		}
		if _, overridden := opts.Weights[id]; overridden {
			return fmt.Errorf("rule %q cannot have a weight override and be disabled", id)
		}
		delete(s.weights, id)
	}
	const maxScore = int(^uint(0) >> 1)
	total := 0
	for _, weight := range s.weights {
		if weight > maxScore-total {
			return fmt.Errorf("sum of enabled rule weights exceeds int range")
		}
		total += weight
	}
	return nil
}
