package workerbroker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDependencyMountsStayOfflineAndReadOnly(t *testing.T) {
	cache, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	mounts := []DependencyMount{{Source: cache, Target: "/opt/agent-assistant/gomod"}, {Source: cache, Target: "/workspace/frontend/node_modules"}}
	if err = validateDependencies(mounts); err != nil {
		t.Fatal(err)
	}
	b := &Broker{cfg: Config{Image: "sha256:fixture", Dependencies: mounts}}
	args := b.containerArgs(storedRun{WorkDir: "/private/run/workspace"})
	text := strings.Join(args, " ")
	for _, want := range []string{"--network none", "dst=/opt/agent-assistant/gomod,readonly", "GOPROXY=off", "GOSUMDB=off"} {
		if !strings.Contains(text, want) {
			t.Fatal("missing", want, text)
		}
	}
	if !strings.Contains(text, "src=/private/run/workspace/frontend/node_modules,dst=/workspace/frontend/node_modules") || strings.Contains(text, "dst=/workspace/frontend/node_modules,readonly") {
		t.Fatal("review dependencies must be writable per-run copies", text)
	}
	if !strings.Contains(text, "src=/private/run/workspace,dst=/workspace,readonly") {
		t.Fatal("review source must remain read-only", text)
	}
	if args[len(args)-3] != "sha256:fixture" {
		t.Fatal("docker options placed after image")
	}
	for _, target := range []string{"/workspace", "/workspace/../etc/node_modules", "/etc", "/var/run/docker.sock"} {
		if validateDependencies([]DependencyMount{{Source: cache, Target: target}}) == nil {
			t.Fatal("unsafe mount accepted", target)
		}
	}
	link := filepath.Join(t.TempDir(), "linked")
	if os.Symlink(cache, link) == nil {
		if validateDependencies([]DependencyMount{{Source: link, Target: "/workspace/node_modules"}}) == nil {
			t.Fatal("linked cache allowed")
		}
	}
}
