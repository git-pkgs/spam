package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/git-pkgs/purl"
)

const corpusFlag = "-corpus"

type savedEntry struct {
	Registry, Name, Version, Label string
	Group                          string `json:"evaluation_group"`
	SplitGroup                     string `json:"split_group"`
	Documents                      []savedDocument
}

type savedDocument struct {
	Path, Member, SHA256 string
	Bytes                int
}

func TestSavedCorpusCommand(t *testing.T) {
	for _, catalog := range []string{"seed/catalog.json", "validation/catalog.json", "validation/stress-catalog.json", "fresh/catalog.json", "documentation/catalog.json", "live/catalog.json"} {
		t.Run(catalog, func(t *testing.T) {
			path := filepath.Join("../../testdata/corpus", catalog)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var entries []savedEntry
			if err := json.Unmarshal(data, &entries); err != nil {
				t.Fatal(err)
			}
			var stdout bytes.Buffer
			if err := run([]string{corpusFlag, path}, &stdout); err != nil {
				t.Fatal(err)
			}
			var results []output
			if err := json.Unmarshal(stdout.Bytes(), &results); err != nil {
				t.Fatal(err)
			}
			if len(entries) == 0 || len(results) != len(entries) {
				t.Fatal("missing corpus results")
			}
			for i, item := range entries {
				verifySavedEntry(t, filepath.Dir(path), item, results[i])
			}
			var repeated bytes.Buffer
			if err := run([]string{corpusFlag, path}, &repeated); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(stdout.Bytes(), repeated.Bytes()) {
				t.Fatal("corpus output is not deterministic")
			}
		})
	}
}

func verifySavedEntry(t *testing.T, root string, item savedEntry, got output) {
	t.Helper()
	if got.Registry != item.Registry || got.Name != item.Name || got.Version != item.Version || got.Label != item.Label || got.EvaluationGroup != item.Group || got.SplitGroup != item.SplitGroup {
		t.Fatalf("identity or label changed: %s", item.Name)
	}
	if got.PURL == "" || got.PURL != purl.MakePURLString(item.Registry, item.Name, item.Version) {
		t.Fatalf("wrong package URL: %q", got.PURL)
	}
	if _, err := purl.Parse(got.PURL); err != nil {
		t.Fatal(err)
	}
	if len(item.Documents) == 0 || len(got.Result.Coverage) != len(item.Documents) || len(got.Result.Measurements) == 0 {
		t.Fatalf("missing scan output: %s", item.Name)
	}
	originals := map[string][]byte{}
	for j, doc := range item.Documents {
		originals[doc.Member] = readSavedDocument(t, root, doc)
		coverage := got.Result.Coverage[j]
		if coverage.Path != doc.Member || coverage.SHA256 != doc.SHA256 || coverage.Bytes != doc.Bytes {
			t.Fatalf("wrong file coverage: %s", doc.Path)
		}
	}
	for _, match := range got.Result.Matches {
		for _, evidence := range match.Evidence {
			text := originals[evidence.Path]
			if evidence.Start < 0 || evidence.End <= evidence.Start || evidence.End > len(text) || !bytes.HasPrefix(text[evidence.Start:evidence.End], []byte(evidence.Excerpt)) {
				t.Fatalf("invalid original evidence: %+v", evidence)
			}
		}
	}
}

func readSavedDocument(t *testing.T, root string, doc savedDocument) []byte {
	t.Helper()
	if !filepath.IsLocal(doc.Path) || filepath.Ext(doc.Path) != ".txt" {
		t.Fatalf("invalid text path: %s", doc.Path)
	}
	text, err := os.ReadFile(filepath.Join(root, doc.Path))
	if err != nil {
		t.Fatal(err)
	}
	if len(text) != doc.Bytes || fmt.Sprintf("%x", sha256.Sum256(text)) != doc.SHA256 {
		t.Fatalf("corpus bytes changed: %s", doc.Path)
	}
	return text
}
