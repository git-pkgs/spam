package corpus

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"unicode/utf8"

	"github.com/git-pkgs/purl"
	"github.com/git-pkgs/spam"
)

const MaxJSONBytes = 4 << 20

type Document struct {
	Path   string `json:"path"`
	Member string `json:"member"`
}

type Entry struct {
	Registry   string     `json:"registry"`
	Name       string     `json:"name"`
	Version    string     `json:"version"`
	Label      string     `json:"label"`
	Group      string     `json:"evaluation_group"`
	SplitGroup string     `json:"split_group"`
	Documents  []Document `json:"documents"`
}

func (e Entry) PURL() string {
	if e.Registry == "" || e.Name == "" {
		return ""
	}
	return purl.MakePURLString(e.Registry, e.Name, e.Version)
}

type Catalog struct {
	Entries []Entry
	SHA256  string
	root    *os.Root
}

func Open(path string) (*Catalog, error) {
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	catalog := &Catalog{root: root}
	data, err := readFile(root, filepath.Base(path), MaxJSONBytes)
	if err == nil {
		err = json.Unmarshal(data, &catalog.Entries)
	}
	if err != nil {
		_ = root.Close()
		return nil, err
	}
	catalog.SHA256 = fmt.Sprintf("%x", sha256.Sum256(data))
	return catalog, nil
}

func (c *Catalog) Close() error {
	return c.root.Close()
}

func (c *Catalog) Documents(entry Entry) ([]spam.Document, error) {
	if len(entry.Documents) == 0 || len(entry.Documents) > spam.MaxDocuments {
		return nil, fmt.Errorf("expected 1 to %d selected documents", spam.MaxDocuments)
	}
	documents := make([]spam.Document, 0, len(entry.Documents))
	total := 0
	for _, doc := range entry.Documents {
		data, err := readFile(c.root, doc.Path, spam.MaxDocumentBytes)
		if err != nil {
			return nil, err
		}
		total += len(data)
		if total > spam.MaxPackageBytes {
			return nil, fmt.Errorf("package text exceeds %d bytes", spam.MaxPackageBytes)
		}
		if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
			return nil, fmt.Errorf("document %q is not UTF-8 text", doc.Path)
		}
		documents = append(documents, spam.Document{Path: doc.Member, Data: data})
	}
	return documents, nil
}

func ReadJSON(path string) ([]byte, error) {
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	return readFile(root, filepath.Base(path), MaxJSONBytes)
}

func readFile(root *os.Root, path string, limit int64) ([]byte, error) {
	info, err := root.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%q is not a regular file", path)
	}
	f, err := root.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	info, err = f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%q is not a regular file", path)
	}
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%q exceeds %d bytes", path, limit)
	}
	return data, nil
}
