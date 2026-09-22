package spam_test

import (
	"bytes"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	"github.com/git-pkgs/spam"
)

func TestScanFileAPIs(t *testing.T) {
	data, err := os.ReadFile("testdata/corpus/seed/free-steam-codes-generator.nuspec.txt")
	if err != nil {
		t.Fatal(err)
	}
	for _, options := range []spam.Options{{}, {Weights: map[string]int{promotionalRule: 4}}} {
		scanner := newScanner(t, options)
		want, err := scanner.Scan([]spam.Document{{Path: readmePath, Data: data}})
		if err != nil {
			t.Fatal(err)
		}
		got, err := scanner.ScanFile(readmePath, data)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("ScanFile differs from Scan: %v, %+v", err, got)
		}
		if options.Weights == nil {
			shared, err := spam.ScanFile(readmePath, data)
			if err != nil || !reflect.DeepEqual(shared, want) {
				t.Fatalf("default ScanFile differs: %v, %+v", err, shared)
			}
		}
	}
}

func TestScanFileOwnsEvidence(t *testing.T) {
	phrase := strings.Repeat("İ", 81) + " Café Σχήμα ẞtraße "
	data := []byte(strings.Repeat(phrase, 6))
	original := bytes.Clone(data)
	result, err := spam.ScanFile(readmePath, data)
	if err != nil {
		t.Fatal(err)
	}
	match := matchFor(result, keywordRule)
	if match == nil || match.Evidence[0].Count != 6 {
		t.Fatalf("missing Unicode repetition: %+v", result)
	}
	checkEvidence(t, spam.Document{Path: readmePath, Data: data}, result)
	if !bytes.Equal(data, original) {
		t.Fatal("scan modified the input")
	}
	excerpt := match.Evidence[0].Excerpt
	if !utf8.ValidString(excerpt) || len(excerpt) > 160 {
		t.Fatalf("invalid truncated excerpt: %q", excerpt)
	}
	clear(data)
	if match.Evidence[0].Excerpt != excerpt || !strings.HasPrefix(phrase, match.Evidence[0].Excerpt) {
		t.Fatal("result retained caller-owned bytes")
	}
}

func TestSharedScanFileConcurrentAndIndependent(t *testing.T) {
	data := []byte("Click here. Buy now. Download now.")
	want, err := newScanner(t, spam.Options{}).ScanFile(readmePath, data)
	if err != nil {
		t.Fatal(err)
	}
	var group sync.WaitGroup
	for range 8 {
		group.Go(func() {
			got, err := spam.ScanFile(readmePath, data)
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Errorf("shared scanner changed result: %v, %+v", err, got)
				return
			}
			got.Weights[promotionalRule] = 99
		})
	}
	group.Wait()
	got, err := spam.ScanFile(readmePath, data)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("result mutation changed defaults: %v, %+v", err, got)
	}
}

func TestScanFileInputValidation(t *testing.T) {
	scanner := newScanner(t, spam.Options{})
	for _, scan := range []func(string, []byte) (spam.Result, error){scanner.ScanFile, spam.ScanFile} {
		for _, data := range [][]byte{{0xff}, {'a', 0}, make([]byte, spam.MaxDocumentBytes+1)} {
			if _, err := scan(readmePath, data); err == nil {
				t.Fatal("invalid input accepted")
			}
		}
		result, err := scan(readmePath, nil)
		if err != nil || result.Documents != 1 || result.Score != 0 {
			t.Fatalf("empty file rejected: %v, %+v", err, result)
		}
	}
}
