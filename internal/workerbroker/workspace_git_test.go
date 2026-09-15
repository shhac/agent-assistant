package workerbroker

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestSnapshotHonorsGitignoreWithoutRunningProjectHooks(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	source := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", source}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git: %s %v", out, err)
		}
	}
	run("init", "--quiet")
	if err := os.WriteFile(filepath.Join(source, ".gitignore"), []byte("build-output\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "build-output"), make([]byte, 3*1024*1024), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "source.go"), []byte("package example\n"), 0600); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(t.TempDir(), "hook-ran")
	// fsmonitor is an executable Git configuration surface; the snapshot must disable it.
	run("config", "core.fsmonitor", "touch "+marker)
	baseline, err := copyWorkspace(source, filepath.Join(t.TempDir(), "copy"))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := baseline["source.go"]; !ok {
		t.Fatal("untracked source omitted")
	}
	if _, ok := baseline["build-output"]; ok {
		t.Fatal("ignored artifact copied")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("Git executed project fsmonitor")
	}
}
