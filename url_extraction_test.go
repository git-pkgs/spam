package spam_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/git-pkgs/spam"
)

func TestMalformedURLPrefixes(t *testing.T) {
	const validURL = "https://example.test/offer"
	cases := []struct {
		name, text string
		count      int
	}{
		{"invalid escape", "Click here: " + strings.Repeat("https://%"+validURL+" ", 3), 0},
		{"invalid port", "Click here: " + strings.Repeat("https://example.test:bad/"+validURL+" ", 3), 0},
		{"valid following token", strings.Repeat("https://%", 4096) + "\nClick here: " + strings.Repeat(validURL+" ", 3), 3},
		{"markup boundary", "https://%\">Click here: " + strings.Repeat(validURL+" ", 3), 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := spam.Document{Path: readmePath, Data: []byte(tc.text)}
			result, err := spam.ScanFile(doc.Path, doc.Data)
			if err != nil {
				t.Fatal(err)
			}
			if got := measurementFor(t, result, linkRule).Count; got != tc.count {
				t.Fatalf("URL count = %d, want %d", got, tc.count)
			}
			if (matchFor(result, urlRule) != nil) != (tc.count == 3) {
				t.Fatalf("unexpected repeated URL evidence: %+v", result.Matches)
			}
			checkEvidence(t, doc, result)
		})
	}
}

func BenchmarkMalformedURLPrefixes(b *testing.B) {
	for _, count := range []int{1024, 8192} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			data := []byte(strings.Repeat("https://%", count))
			scanner, err := spam.NewScanner(spam.Options{})
			if err != nil {
				b.Fatal(err)
			}
			b.SetBytes(int64(len(data)))
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				if _, err := scanner.ScanFile(readmePath, data); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
