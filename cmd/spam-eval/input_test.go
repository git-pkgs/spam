package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/git-pkgs/spam/internal/corpus"
)

func TestRejectsUnboundedOrNonRegularInput(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "catalog.json")
	writeFile(t, path, []byte("[]"+strings.Repeat(" ", corpus.MaxJSONBytes)))
	var stdout bytes.Buffer
	err := run([]string{corpusFlag, path}, &stdout)
	if err == nil || !strings.Contains(err.Error(), "exceeds") || stdout.Len() != 0 {
		t.Fatalf("oversized catalogue accepted: %v", err)
	}
	writeFile(t, path, []byte(`[{"name":"directory","documents":[{"path":".","member":"README.md"}]}]`))
	err = run([]string{corpusFlag, path}, &stdout)
	if err == nil || !strings.Contains(err.Error(), "regular file") || stdout.Len() != 0 {
		t.Fatalf("directory document accepted: %v", err)
	}
}

func TestOmitsUnrepresentableIdentity(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "doc.txt"), []byte("library documentation"))
	path := filepath.Join(dir, "catalog.json")
	writeFile(t, path, []byte(`[{"registry":"swift","name":"scope.package","version":"1","documents":[{"path":"doc.txt","member":"README.md"}]}]`))
	var stdout bytes.Buffer
	if err := run([]string{corpusFlag, path}, &stdout); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(stdout.Bytes(), []byte(`"purl"`)) {
		t.Fatalf("unrepresentable PURL was emitted: %s", stdout.Bytes())
	}
}
