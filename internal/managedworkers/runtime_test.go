package managedworkers

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type commandCall struct {
	bin   string
	args  []string
	input string
	env   []string
	dir   string
}
type commandFunc func(context.Context, string, []string, []byte, []string, string) ([]byte, error)

func (f commandFunc) Run(c context.Context, b string, a []string, i []byte, e []string, d string) ([]byte, error) {
	return f(c, b, a, i, e, d)
}
func TestPreparationUsesFixedToolchainAndNoAmbientCredentials(t *testing.T) {
	root := t.TempDir()
	var calls []commandCall
	built := false
	t.Setenv("AWS_SECRET_ACCESS_KEY", "must-not-inherit")
	t.Setenv("DOCKER_HOST", "tcp://remote:1234")
	t.Setenv("DOCKER_CONTEXT", "remote")
	r := &localRuntime{home: "/synthetic/home", platform: "linux", lookPath: func(name string) (string, error) { return "/bin/" + name, nil }, exists: func(p string) bool { return p == "/var/run/docker.sock" }}
	r.command = commandFunc(func(ctx context.Context, bin string, args []string, input []byte, env []string, dir string) ([]byte, error) {
		calls = append(calls, commandCall{bin, args, string(input), env, dir})
		joined := strings.Join(args, " ")
		if strings.Contains(joined, "context inspect") {
			return []byte(`"tcp://forbidden:2375"`), nil
		}
		if strings.Contains(joined, "image inspect") {
			if !built {
				return nil, errors.New("absent")
			}
			return []byte(testImage), nil
		}
		if strings.Contains(joined, "build --pull") {
			built = true
		}
		return []byte("ok"), nil
	})
	env, err := r.Prepare(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if env.Socket != "/var/run/docker.sock" || env.Image != testImage {
		t.Fatal(env)
	}
	found := false
	for _, call := range calls {
		if strings.Contains(strings.Join(call.env, " "), "must-not-inherit") || strings.Contains(strings.Join(call.env, " "), "DOCKER_") {
			t.Fatal("inherited ambient context/credentials")
		}
		if call.dir != root {
			t.Fatal("setup ran in project directory")
		}
		if strings.Contains(strings.Join(call.args, " "), "tcp://") {
			t.Fatal("connected to remote context")
		}
		if strings.Contains(strings.Join(call.args, " "), "build --pull") {
			found = true
			if call.input != toolchainDockerfile || call.args[len(call.args)-1] != "-" {
				t.Fatal("project directory used as build context")
			}
		}
	}
	if !found {
		t.Fatal("did not prepare toolchain")
	}
	calls = nil
	if err = r.Check(context.Background(), root, env); err != nil {
		t.Fatal(err)
	}
	for _, call := range calls {
		args := strings.Join(call.args, " ")
		if strings.Contains(args, "build") || strings.Contains(args, "pull") || strings.Contains(args, "start") || strings.Contains(args, "install") {
			t.Fatal("recovery provisions")
		}
	}
}
func TestMacInstallsOnlyFreeFixedDependenciesAndKeepsContext(t *testing.T) {
	root := t.TempDir()
	installed := false
	started := false
	var calls []commandCall
	r := &localRuntime{home: "/synthetic/home", platform: "darwin", exists: func(string) bool { return false }}
	r.lookPath = func(name string) (string, error) {
		if name == "brew" || installed {
			return "/bin/" + name, nil
		}
		return "", os.ErrNotExist
	}
	r.command = commandFunc(func(ctx context.Context, bin string, args []string, input []byte, env []string, dir string) ([]byte, error) {
		calls = append(calls, commandCall{bin, args, string(input), env, dir})
		if bin == "/bin/brew" {
			if !reflect.DeepEqual(args, []string{"install", "docker", "docker-buildx", "colima"}) {
				t.Fatal(args)
			}
			installed = true
		}
		if bin == "/bin/colima" {
			started = true
			joined := strings.Join(args, " ")
			if !strings.Contains(joined, "--activate=false") || !strings.Contains(joined, "--ssh-config=false") || !strings.Contains(joined, "--profile agent-assistant") {
				t.Fatal(args)
			}
		}
		if strings.Contains(strings.Join(args, " "), "image inspect") {
			return []byte(testImage), nil
		}
		return []byte("ok"), nil
	})
	env, err := r.Prepare(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !installed || !started || env.Socket != filepath.Join(r.home, ".colima", "agent-assistant", "docker.sock") {
		t.Fatal(env)
	}
	if len(calls) == 0 {
		t.Fatal("no setup")
	}
}
func TestInvalidSavedEnvironmentNeverConnects(t *testing.T) {
	r := newLocalRuntime()
	r.command = commandFunc(func(context.Context, string, []string, []byte, []string, string) ([]byte, error) {
		t.Fatal("unsafe connection attempted")
		return nil, nil
	})
	for _, socket := range []string{"tcp://remote:2375", "relative.sock", "/tmp/a\n.sock"} {
		if err := r.Check(context.Background(), t.TempDir(), environment{Socket: socket, Image: testImage}); err == nil {
			t.Fatal("accepted", socket)
		}
	}
}
