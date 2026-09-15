package workerbroker

import (
	"errors"
	"os"
	"path"
	"path/filepath"
	"strings"
)

func validateDependencies(mounts []DependencyMount) error {
	if len(mounts) > 17 {
		return errors.New("too many worker dependency mounts")
	}
	seen := map[string]bool{}
	for _, m := range mounts {
		if !filepath.IsAbs(m.Source) || strings.ContainsAny(m.Source, ",\x00\r\n") || strings.ContainsAny(m.Target, ",\x00\r\n\\") || seen[m.Target] {
			return errors.New("invalid worker dependency mount")
		}
		if m.Target != "/opt/agent-assistant/gomod" && !(strings.HasPrefix(m.Target, "/workspace/") && strings.HasSuffix(m.Target, "/node_modules") && path.Clean(m.Target) == m.Target) {
			return errors.New("dependency mounts must target module caches only")
		}
		resolved, err := filepath.EvalSymlinks(m.Source)
		if err != nil || resolved != m.Source {
			return errors.New("worker dependency cache must be a real directory without symlinks")
		}
		info, err := os.Lstat(m.Source)
		if err != nil || !info.IsDir() {
			return errors.New("worker dependency cache is unavailable")
		}
		seen[m.Target] = true
	}
	return nil
}
