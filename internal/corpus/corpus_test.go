package corpus_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/git-pkgs/purl"
	"github.com/git-pkgs/spam"
	"github.com/git-pkgs/spam/internal/corpus"
)

func TestDocumentLimits(t *testing.T) {
	cases := []struct {
		name  string
		data  []byte
		count int
	}{
		{"no documents", nil, 0},
		{"too many documents", []byte("text"), spam.MaxDocuments + 1},
		{"oversized document", []byte(strings.Repeat("a", spam.MaxDocumentBytes+1)), 1},
		{"oversized package", []byte(strings.Repeat("a", spam.MaxDocumentBytes)), 5},
		{"invalid UTF-8", []byte{0xff}, 1},
		{"NUL", []byte{'a', 0}, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			write(t, filepath.Join(dir, "doc.txt"), tc.data)
			entry := corpus.Entry{Documents: make([]corpus.Document, tc.count)}
			for i := range entry.Documents {
				entry.Documents[i] = corpus.Document{Path: "doc.txt", Member: "README.md"}
			}
			data, err := json.Marshal([]corpus.Entry{entry})
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "catalog.json")
			write(t, path, data)
			catalog, err := corpus.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = catalog.Close() }()
			if _, err := catalog.Documents(catalog.Entries[0]); err == nil {
				t.Fatal("invalid documents accepted")
			}
		})
	}
}

func TestPackageURLs(t *testing.T) {
	cases := []struct {
		registry, name, version, want string
	}{
		{"npm", "@scope/package", "1.0.0", "pkg:npm/%40scope/package@1.0.0"},
		{"pypi", "Some_Package", "1.0", "pkg:pypi/some-package@1.0"},
		{"nuget", "Humanizer", "2.0", "pkg:nuget/Humanizer@2.0"},
		{"rubygems", "casino", "4.1.2", "pkg:gem/casino@4.1.2"},
		{"synthetic", "example", "1", "pkg:synthetic/example@1"},
		{"", "example", "1", ""},
		{"npm", "", "1", ""},
		{"swift", "scope.package", "1", ""},
	}
	for _, tc := range cases {
		entry := corpus.Entry{Registry: tc.registry, Name: tc.name, Version: tc.version}
		got := entry.PURL()
		if got != tc.want {
			t.Errorf("%s/%s: got %q want %q", tc.registry, tc.name, got, tc.want)
		}
		if got != "" {
			if _, err := purl.Parse(got); err != nil {
				t.Error(err)
			}
		}
	}
}

func write(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
