package workerbroker

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Repository snapshots include tracked and untracked source, but no ignored
// build outputs. Git only lists paths; hooks, filters and fsmonitor never run.
func workspaceSelection(source string) (map[string]bool, error) {
	if _, err := os.Lstat(filepath.Join(source, ".git")); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "--no-optional-locks", "-c", "core.fsmonitor=false", "-c", "core.untrackedCache=false", "-C", source, "ls-files", "--cached", "--others", "--exclude-standard", "-z")
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=" + os.DevNull, "GIT_TERMINAL_PROMPT=0"}
	out := &boundedOutput{limit: 2*1024*1024 + 1}
	cmd.Stdout = out
	if err := cmd.Run(); err != nil {
		return nil, errors.New("could not list project source files; check that Git is installed and the folder is a readable repository")
	}
	if out.Len() > 2*1024*1024 {
		return nil, errors.New("project file list is too large for an isolated snapshot")
	}
	selected := map[string]bool{}
	for _, raw := range bytes.Split(out.Bytes(), []byte{0}) {
		name := string(raw)
		if name == "" {
			continue
		}
		if !fs.ValidPath(name) || strings.Contains(name, "\\") {
			return nil, errors.New("project contains unsupported source paths")
		}
		selected[name] = true
		for parent := filepath.ToSlash(filepath.Dir(name)); parent != "."; parent = filepath.ToSlash(filepath.Dir(parent)) {
			selected[parent] = true
		}
	}
	return selected, nil
}
