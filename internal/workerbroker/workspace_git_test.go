package workerbroker

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

// The dashboard groups evidence for reading by matching these prefixes. They
// are a contract with the UI, not incidental wording: changing one without
// changing internal/dashboard/ui/src/evidence.ts silently degrades every
// grouped view into an undifferentiated list.
func TestEvidenceDigestKeepsThePrefixesTheDashboardGroupsBy(t *testing.T) {
	lines := evidenceDigest(
		[]string{"a.go", "b.ts"},
		[]commandRecord{
			{Command: "go test ./...", Success: true, Output: "ok"},
			{Command: "npm test", Success: false, Output: "failed"},
		},
		"diff --git a/a.go b/a.go\n",
	)
	joined := strings.Join(lines, "\n---\n")
	for _, prefix := range []string{
		"Broker evidence: 2 changed files; 2 recorded commands: 1 SUCCEEDED, 1 FAILED.",
		"Changed paths (",
		"Command excerpts: ",
		"Content patch SHA-256: ",
	} {
		if !strings.Contains(joined, prefix) {
			t.Fatalf("evidence no longer carries the prefix %q the dashboard groups by", prefix)
		}
	}
	commands := 0
	for _, line := range lines {
		if strings.HasPrefix(line, "Command 1 SUCCEEDED: ") || strings.HasPrefix(line, "Command 2 FAILED: ") {
			commands++
		}
	}
	if commands != 2 {
		t.Fatalf("command evidence lines = %d, want 2 in the \"Command <n> <STATUS>: \" form", commands)
	}
	if !strings.Contains(lines[0], "not proof that acceptance criteria are met") {
		t.Fatal("evidence no longer states that command success is not acceptance")
	}
}
