package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMixedNamespacePathsRequireExplicitPair(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(root, "state"))
	for _, dir := range []string{"agent-assistant", "agent-assistant.paulie.app"} {
		path := filepath.Join(root, "config", dir, "config.json")
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("{}"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	cmd := NewRoot("test")
	cmd.SetArgs([]string{"config", "path"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "both legacy") {
		t.Fatalf("expected namespace ambiguity, got %v", err)
	}
	cmd = NewRoot("test")
	cmd.SetArgs([]string{"--config", filepath.Join(root, "chosen.json"), "--state", filepath.Join(root, "chosen.db"), "config", "validate"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
}
