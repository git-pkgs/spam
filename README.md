# spam

An offline Go library for examining promotional text in software packages. Pass raw manifests, READMEs, or other text as bytes directly to the scanner. It uses git-pkgs/scan for phrase and URL-prefix matching, with byte offsets into the supplied data.

The first five rules are exploratory. Their scores are uncalibrated, and ordinary package metadata triggers several of them. Results include per-file measurements even below a rule's threshold, so callers can store those facts and compare them independently of the score.

## Installation

Requires Go 1.26 or later. The package builds with `CGO_ENABLED=0`.

```bash
go get github.com/git-pkgs/spam
```

## Usage

```go
package main

import (
    "fmt"
    "log"

    "github.com/git-pkgs/spam"
)

func main() {
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
}
```

This prints `2 2`: two documents scanned and a score of two. For one file, call `spam.ScanFile("README.md", data)` with default weights or `scanner.ScanFile("README.md", data)` with a configured scanner. The package-level function initializes a shared scanner once; both forms accept file contents without opening a path.

## Rules and scoring

Weights must be nonnegative, and the sum of enabled weights must fit in an `int`. Generic phrase repetition, line repetition and link density default to zero; the other rules default to one. A zero weight retains matches and measurements but contributes no score. Set `Weights: map[string]int{"LINK_HEAVY": 1}` to give link density a positive weight. Repeated phrases and repeated lines share one contribution; repeated URLs and link density share another. Each group contributes its highest matched weight, with ties resolved in rule order.

`DisabledRules: []string{"LINK_HEAVY"}` skips that rule's analysis and removes its matches, measurements and weight from results. Shared text matching still runs when another enabled rule needs it. A rule cannot appear in both `Weights` and `DisabledRules`. Disabled custom expressions are not compiled or syntax-checked; their required fields and default weights are still validated. Expression errors surface when the rule is enabled. `scanner.Rules()` returns an independent copy of the enabled definitions, including descriptions, categories, default weights and provenance. Weight overrides appear in results; they do not change the definitions' defaults.

| Rule | Initial trigger |
| --- | --- |
| PROMOTIONAL_PHRASES | Three promotional cues, or two offer claims with a cue and a URL in the same document. |
| KEYWORD_REPETITION | A four-word sequence of letters with attached combining marks, separated by spaces or tabs, occurs six times in one document. Default weight: zero. |
| REPEATED_URLS | Three URLs share a destination after removing fragments. One occurrence must be the nearest URL to a promotional cue on the same line, within 256 raw bytes. Queries are retained. |
| LINK_HEAVY | At least ten URLs occupy 35% of raw document bytes. Default weight: zero. |
| REPEATED_LINES | A trimmed line of at least 24 bytes occurs three times. Default weight: zero. |

`no survey`, `no human verification` and `guaranteed profit` count as both promotional cues and offer claims. Two such occurrences with a URL satisfy the offer trigger without a separate call to action. Counts include repeated occurrences of the same phrase.

### Custom rules

Add custom rules through `Options.Rules`. Each rule requires a unique ID, description, category, expression and provenance. Built-in IDs cannot be replaced. Expressions and built-in metadata are data: the built-in definitions are embedded from `rules/builtin.json`, and callers can decode a JSON array into `[]spam.Rule` or embed their own definitions with `go:embed`.

```go
scanner, err := spam.NewScanner(spam.Options{
    Rules: []spam.Rule{{
        ID: "LOCAL_OFFER",
        Description: "Exclusive offer phrase",
        Category: "promotion",
        Expression: `(?i)\bexclusive offer\b`,
        DefaultWeight: 1,
        Provenance: "local policy",
    }},
    Weights: map[string]int{"LOCAL_OFFER": 2},
})
if err != nil {
    return err
}
result, err := scanner.ScanFile("README.md", []byte("Exclusive offer. Exclusive offer."))
if err != nil {
    return err
}
```

Custom expressions scan the supplied raw UTF-8 text with git-pkgs/scan. Matching is case-sensitive unless an inline flag such as `(?i)` changes it. Empty matches, look-around and backreferences are rejected. Definitions are trusted configuration: compile them once with `NewScanner`, outside the package-input path. The compiler has no resource budget for caller-supplied expressions.

A custom rule reports at most one finding per document and contributes its weight once per scan. Its measurement is zero or one, even when the text repeats or a variable-length expression matches several end offsets. Evidence uses the first matching end offset and earliest start at that endpoint; `offer [0-9]+` can therefore report `offer 1` within `offer 123`. Custom rules contribute independently, so callers should avoid assigning weight to several expressions for the same signal. Rule order follows the built-ins, then `Options.Rules`.

## Results and limits

The scanner returns each rule's weight and contribution, bounded evidence, raw counts, file hashes, and coverage. `Result.Configuration` is a SHA-256 identity of the ruleset, embedded definitions, enabled rule order and effective weights. Byte-identical documents are scanned once, with duplicate paths recorded. Evidence offsets refer to the original UTF-8 text; excerpts are limited to 160 bytes and four documents per rule. Treat excerpts as untrusted text when displaying them. The library does not fetch links, render markup, execute package code, or classify a package as spam.

Reuse a scanner across concurrent calls. Input bytes are read without modification and must remain unchanged until the call returns; the result owns its evidence text, so callers can reuse their buffers afterward. Limits are 64 documents, 512 KiB per document, and 2 MiB of text per call; oversized or non-UTF-8 input returns an error. The caller chooses files and records anything skipped outside that call. A pipeline can scan one file at a time and attach its release identity, file role, and analyzer version to the result.

Raw markup, code examples, classifier headers, dependency declarations, and URL text still participate in the repetition measurement, `count`. The keyword rule uses `text_count`, which excludes sequences joined through punctuation, digits, or line breaks. Promotional measurements also include `offer_count` for phrases such as free coins, unlimited credits, and guaranteed returns. The repeated-URL rule adds `context_count`: the largest destination count with an occurrence near a cue. Its evidence points to that occurrence and records the cue's byte offsets. These counts describe occurrences, including repeated statements; they do not establish intent.

URL extraction is lexical; it does not render Markdown or decode HTML entities, and namespace or reference URLs also count. URL schemes and hosts are lowercased; paths and queries retain their original spelling. Nested URL prefixes are skipped even when the containing token is malformed. Cues inside extracted URLs do not qualify as repeated-link context. A nearby cue can still refer to an unrelated link, and wrapped labels or long HTML attributes can prevent association. Long API references can repeat ordinary prose, and short advertisements may produce no matches. Changing rule definitions requires another scan; stored counts permit later weight changes and threshold comparisons for these measurements. Results carry the ruleset identifier `raw-text-6`, which keeps combining marks attached to words. It retains the URL extraction changes from `raw-text-5` and narrower context and query handling introduced in `raw-text-4`. Counts from rulesets before `raw-text-4` cannot reconstruct the context measurement.

## Corpus evaluation

The corpus command reads a JSON array of entries containing package identity, labels, and a `documents` array. Each document supplies `path`, relative to the catalogue directory, and `member`, its original archive path. Both evaluators limit JSON inputs to 4 MiB, require regular files, and confine document reads to the catalogue directory. They compute each output row's `purl` using git-pkgs/purl without changing the saved catalogue, omitting the field when identity is incomplete or cannot be represented. The corpus command writes JSON results while preserving evaluation groups:

```sh
go run ./cmd/spam-eval -corpus testdata/corpus/seed/catalog.json
go run ./cmd/spam-eval -corpus testdata/corpus/validation/catalog.json
```

The [saved corpus](testdata/corpus/README.md) includes promotional packages, legitimate controls, and a separate quoted-spam stress case. Tests check the original text hashes and run all six catalogues through the command. Default scores are nonzero for four of thirteen promotional releases across three of eight campaign groups, and zero for all thirty-three legitimate controls, including MailKit's donation button and protocol references. These examples were used during development. The nine missed adverts and the small, selected control set prevent interpreting scores as probabilities or claiming a production false-positive rate. Labels are independent of rule matches or scores.

### Upstream rules

`spam-rules-eval` evaluates the [upstream candidate rules](rules/upstream/README.md) against the same corpus. It loads expressions from JSON, checks their positive and negative fixtures, and compares scan's matches with Go regexp. Candidates are not loaded by the library; this command explicitly evaluates them and returns boolean findings with original byte offsets, file hashes and labels. Their research JSON includes input scopes and fixtures that differ from the public `Rule` format. Copying an expression into a custom rule requires preserving its source attribution and checking whether raw document input is appropriate.

```sh
go run ./cmd/spam-rules-eval -rules rules/upstream/candidates.json -corpus testdata/corpus/seed/catalog.json
go run ./cmd/spam-rules-eval -rules rules/upstream/second-candidates.json -corpus testdata/corpus/validation/catalog.json -view normalized
```

The default view scans raw text. `-view normalized` removes HTML markup, decodes character references and folds whitespace within paragraphs for body-text rules. Script, style and template contents are excluded, including after self-closing start tags. Markup and URL rules retain their separate input views. Normalized findings identify the view as `html-text-v2` and include spans mapping back to the original text. The command reads local files without fetching links or executing package code.

## Testing

```bash
go test -race ./...
```

Tests cover the public API, executable examples, both evaluators and the saved corpus. They use local text fixtures without registry access or package installation.

## License

[MIT](LICENSE). The [upstream rule candidates](rules/upstream/README.md) include Apache-2.0 source material with its original license and attribution notices.
