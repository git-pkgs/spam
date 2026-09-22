package spam_test

import (
	"strings"
	"testing"

	"github.com/git-pkgs/spam"
)

func TestRepeatedURLContext(t *testing.T) {
	const destination = "https://example.test/offer"
	const reference = "https://example.test/reference"
	repeats := strings.Repeat(destination+"\n", 2)
	cases := []struct {
		name, text     string
		count, context int
		matched        bool
	}{
		{"markdown label", "[Click here](" + destination + ")\n" + repeats, 3, 3, true},
		{"html label", `<a href="` + destination + `">Click here</a>` + "\n" + repeats, 3, 3, true},
		{"plain text", "Download now: " + destination + "\n" + repeats, 3, 3, true},
		{"cue after URL", destination + " (download now)\n" + repeats, 3, 3, true},
		{"cue across newline", "Download now\n" + destination + "\n" + repeats, 3, 0, false},
		{"cue across carriage return", "Download now\r" + destination + "\n" + repeats, 3, 0, false},
		{"distant cue", "Download now" + strings.Repeat(" ", 257) + destination + "\n" + repeats, 3, 0, false},
		{"gap boundary", "Download now" + strings.Repeat(" ", 256) + destination + "\n" + repeats, 3, 3, true},
		{"unrelated donation", "[Click here](https://example.test/donate)\n\n" + repeats + destination, 3, 1, false},
		{"unrelated closer link", "[Click here](https://example.test/donate) " + destination + "\n" + repeats, 3, 1, false},
		{"unrelated higher count", strings.Repeat(reference+"\n", 4) + "[Click here](" + destination + ")\n" + repeats, 4, 3, true},
		{"distinct queries", "Click here: " + destination + "?page=1 " + destination + "?page=2 " + destination + "?page=3", 1, 1, false},
		{"same query", "Click here: " + strings.Repeat(destination+"?campaign=one ", 3), 3, 3, true},
		{"different redirect targets", "Click here: https://example.test/redirect?to=a https://example.test/redirect?to=b https://example.test/redirect?to=c", 1, 1, false},
		{"cue inside URL", strings.Repeat(destination+"?label=no survey\n", 2), 2, 0, false},
		{"no links", "Click here", 0, 0, false},
		{"no cue", repeats + destination, 3, 0, false},
	}
	scanner := newScanner(t, spam.Options{})
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := spam.Document{Path: readmePath, Data: []byte(tc.text)}
			result, err := scanner.ScanFile(doc.Path, doc.Data)
			if err != nil {
				t.Fatal(err)
			}
			measurement := measurementFor(t, result, urlRule)
			if measurement.Count != tc.count || measurement.ContextCount != tc.context {
				t.Fatalf("unexpected URL measurements: %+v", measurement)
			}
			match := matchFor(result, urlRule)
			if (match != nil) != tc.matched {
				t.Fatalf("unexpected repeated URL match: %+v", match)
			}
			if match != nil && (match.Evidence[0].Count != tc.context || !strings.HasPrefix(match.Evidence[0].Excerpt, destination)) {
				t.Fatalf("evidence does not identify the associated destination: %+v", match)
			}
			checkEvidence(t, doc, result)
		})
	}
}

func TestURLContextDoesNotCrossDocuments(t *testing.T) {
	result, err := newScanner(t, spam.Options{}).Scan([]spam.Document{
		{Path: "metadata/description.txt", Data: []byte("Click here")},
		{Path: readmePath, Data: []byte(strings.Repeat("https://example.test/offer\n", 3))},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Score != 0 || matchFor(result, urlRule) != nil {
		t.Fatalf("cue crossed document boundary: %+v", result)
	}
}
