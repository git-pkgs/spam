//go:build darwin || linux

package corpus_test

import (
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/git-pkgs/spam/internal/corpus"
)

func TestRejectsFIFOWithoutWaitingForWriter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pipe.json")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := corpus.ReadJSON(path)
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "regular file") {
			t.Fatalf("FIFO accepted: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("blocked opening a FIFO")
	}
}
