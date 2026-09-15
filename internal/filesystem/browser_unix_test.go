//go:build !windows

package filesystem

import (
	"context"
	"errors"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestNamedPipeCannotBlockDirectoryRequest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pipe")
	if err := syscall.Mkfifo(path, 0600); err != nil {
		t.Fatal(err)
	}
	finished := make(chan error, 1)
	go func() { _, err := New("").List(context.Background(), Query{Path: path}); finished <- err }()
	select {
	case err := <-finished:
		if !errors.Is(err, ErrPath) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("directory listing blocked opening a FIFO")
	}
}
