package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/git-pkgs/spam/internal/corpus"
)

func TestRejectsOutsideCorpus(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "corpus")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "outside.txt"), []byte("money back guarantee"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../outside.txt", filepath.Join(dir, "escape.txt")); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"../outside.txt", "escape.txt"} {
		catalog := []map[string]any{{"name": "escape", "documents": []map[string]string{{"path": path, "member": "README.md"}}}}
		data, err := json.Marshal(catalog)
		if err != nil {
			t.Fatal(err)
		}
		catalogPath := filepath.Join(dir, "catalog.json")
		if err := os.WriteFile(catalogPath, data, 0o600); err != nil {
			t.Fatal(err)
		}
		var stdout bytes.Buffer
		if err := run([]string{rulesFlag, candidatePath, corpusFlag, catalogPath}, &stdout); err == nil || stdout.Len() != 0 {
			t.Fatalf("accepted outside path or emitted partial results: %s", path)
		}
	}
}

func TestRejectsUnboundedJSON(t *testing.T) {
	dir := t.TempDir()
	large := filepath.Join(dir, "large.json")
	if err := os.WriteFile(large, []byte("[]"+strings.Repeat(" ", corpus.MaxJSONBytes)), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{rulesFlag, large, corpusFlag, "../../testdata/corpus/seed/catalog.json"},
		{rulesFlag, candidatePath, corpusFlag, large},
	} {
		var stdout bytes.Buffer
		err := run(args, &stdout)
		if err == nil || !strings.Contains(err.Error(), "exceeds") || stdout.Len() != 0 {
			t.Fatalf("oversized JSON accepted: %v", err)
		}
	}
}

func TestRejectsNonRegularInput(t *testing.T) {
	dir := t.TempDir()
	catalog := filepath.Join(dir, "catalog.json")
	data := []byte(`[{"name":"directory","documents":[{"path":".","member":"README.md"}]}]`)
	if err := os.WriteFile(catalog, data, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{rulesFlag, dir, corpusFlag, catalog},
		{rulesFlag, candidatePath, corpusFlag, dir},
		{rulesFlag, candidatePath, corpusFlag, catalog},
	} {
		var stdout bytes.Buffer
		err := run(args, &stdout)
		if err == nil || !strings.Contains(err.Error(), "regular file") || stdout.Len() != 0 {
			t.Fatalf("non-regular file accepted: %v", err)
		}
	}
}

func TestOmitsUnrepresentableIdentity(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "doc.txt"), []byte("library documentation"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "catalog.json")
	data := []byte(`[{"registry":"swift","name":"scope.package","version":"1","documents":[{"path":"doc.txt","member":"README.md"}]}]`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	if err := run([]string{rulesFlag, candidatePath, corpusFlag, path}, &stdout); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(stdout.Bytes(), []byte(`"purl"`)) {
		t.Fatalf("unrepresentable PURL was emitted: %s", stdout.Bytes())
	}
}
