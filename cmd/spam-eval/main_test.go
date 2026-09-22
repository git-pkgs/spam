package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCorpusCommand(t *testing.T) {
	dir := t.TempDir()
	data, err := os.ReadFile("../../testdata/corpus/seed/free-steam-codes-generator.nuspec.txt")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "manifest.txt"), data)
	catalog := `[{"registry":"nuget","name":"free-steam-codes-generator","version":"1.0.0","label":"promotional_spam","evaluation_group":"positive","split_group":"campaign","documents":[{"path":"manifest.txt","member":"package.nuspec"}]}]`
	writeFile(t, filepath.Join(dir, "catalog.json"), []byte(catalog))
	var stdout bytes.Buffer
	if err := run([]string{corpusFlag, filepath.Join(dir, "catalog.json")}, &stdout); err != nil {
		t.Fatal(err)
	}
	var results []output
	if err := json.Unmarshal(stdout.Bytes(), &results); err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Result.Score == 0 || results[0].EvaluationGroup != "positive" || results[0].Coverage != "selected raw documents only" {
		t.Fatalf("unexpected output: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "KEYWORD_REPETITION") {
		t.Fatalf("missing rule: %s", stdout.String())
	}
}

func TestRejectsOutsideCorpus(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "corpus")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(base, "outside.txt"), []byte("private"))
	if err := os.Symlink("../outside.txt", filepath.Join(dir, "escape.txt")); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"../outside.txt", "escape.txt"} {
		catalog := `[{"name":"escape","documents":[{"path":"` + path + `","member":"README.md"}]}]`
		writeFile(t, filepath.Join(dir, "catalog.json"), []byte(catalog))
		var stdout bytes.Buffer
		if err := run([]string{corpusFlag, filepath.Join(dir, "catalog.json")}, &stdout); err == nil {
			t.Fatalf("accepted %s", path)
		}
		if stdout.Len() != 0 {
			t.Fatal("partial output on failure")
		}
	}
}

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}
