package workerbroker

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestNPMDependenciesAreWritableOnlyInRunCopy(t *testing.T) {
	cache := t.TempDir()
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cache, ".bin"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cache, "tool.js"), []byte("synthetic"), 0755); err != nil {
		t.Fatal(err)
	}
	linked := os.Symlink("../tool.js", filepath.Join(cache, ".bin", "tool")) == nil
	b := &Broker{cfg: Config{Dependencies: []DependencyMount{{Source: cache, Target: "/workspace/frontend/node_modules"}}}}
	if err := b.prepareRunDependencies(context.Background(), workspace); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(workspace, "frontend", "node_modules")
	if err := os.MkdirAll(filepath.Join(target, ".vite-temp"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "tool.js"), []byte("changed in run"), 0644); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(cache, "tool.js"))
	if err != nil || string(raw) != "synthetic" {
		t.Fatal("shared cache changed", err)
	}
	if _, err := os.Stat(filepath.Join(cache, ".vite-temp")); !os.IsNotExist(err) {
		t.Fatal("run cache write leaked")
	}
	if linked {
		if _, err := os.ReadFile(filepath.Join(target, ".bin", "tool")); err != nil {
			t.Fatal("npm executable link lost", err)
		}
	}
}
func TestNPMCopyRejectsEscapesLargeFilesAndCancellation(t *testing.T) {
	source := t.TempDir()
	target := t.TempDir()
	var total int64
	count := 0
	if err := os.Symlink("../../secret", filepath.Join(source, "escape")); err == nil {
		if err = copyDependencyTree(context.Background(), source, target, &total, &count); err == nil {
			t.Fatal("escaping link copied")
		}
		os.Remove(filepath.Join(source, "escape"))
	}
	f, err := os.Create(filepath.Join(source, "large"))
	if err != nil {
		t.Fatal(err)
	}
	if err = f.Truncate(65 * 1024 * 1024); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if err = copyDependencyTree(context.Background(), source, target, &total, &count); err == nil {
		t.Fatal("large dependency copied")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err = copyDependencyTree(ctx, source, target, &total, &count); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
