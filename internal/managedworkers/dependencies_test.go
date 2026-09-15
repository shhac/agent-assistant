package managedworkers

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const npmPackage = `{"name":"synthetic","version":"1.0.0","scripts":{"postinstall":"do-not-run"},"dependencies":{"example":"^1.0.0"}}`
const npmLock = `{"lockfileVersion":3,"packages":{"":{"dependencies":{"example":"^1.0.0"}},"node_modules/example":{"version":"1.0.0","resolved":"https://registry.npmjs.org/example/-/example-1.0.0.tgz","integrity":"sha512-synthetic"}}}`

func TestDependencyManifestsRejectPrivateAndExecutableSources(t *testing.T) {
	if err := validateNPM([]byte(npmPackage), []byte(npmLock)); err != nil {
		t.Fatal(err)
	}
	for _, lock := range []string{
		strings.ReplaceAll(npmLock, "registry.npmjs.org", "private.example"), strings.ReplaceAll(npmLock, "https://registry.npmjs.org", "https://token@registry.npmjs.org"), strings.ReplaceAll(npmLock, "https://registry.npmjs.org/example/-/example-1.0.0.tgz", "git+ssh://github.com/private/repo"), strings.ReplaceAll(npmLock, `"version":"1.0.0","resolved"`, `"link":true,"version":"1.0.0","resolved"`),
	} {
		if err := validateNPM([]byte(npmPackage), []byte(lock)); err == nil {
			t.Fatal("unsafe dependency allowed", lock)
		}
	}
	for _, spec := range []string{"github:user/repo", "git+https://example.com/repo", "file:../private", "https://private.example/file.tgz"} {
		pkg := strings.ReplaceAll(npmPackage, "^1.0.0", spec)
		if err := validateNPM([]byte(pkg), []byte(npmLock)); err == nil {
			t.Fatal(spec)
		}
	}
	workspace := t.TempDir()
	os.WriteFile(filepath.Join(workspace, "go.mod"), []byte("module example.com/test\nreplace private => ../private\n"), 0600)
	if _, err := dependencyManifests(workspace); err == nil {
		t.Fatal("local go replacement allowed")
	}
}
func TestDependencyPreparationDownloadsOnlyManifestsAndReusesCache(t *testing.T) {
	workspace := t.TempDir()
	root := t.TempDir()
	project := t.TempDir()
	for name, value := range map[string]string{"package.json": npmPackage, "package-lock.json": npmLock, "go.mod": "module example.com/fixture\ngo 1.26\n", "app.go": "// source must never enter online preparation", ".npmrc": "secret"} {
		if err := os.WriteFile(filepath.Join(workspace, name), []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	var calls []commandCall
	r := &localRuntime{home: "/synthetic/home", platform: "linux", lookPath: func(s string) (string, error) { return "/bin/" + s, nil }, exists: func(string) bool { return true }}
	r.command = commandFunc(func(ctx context.Context, bin string, args []string, input []byte, env []string, dir string) ([]byte, error) {
		calls = append(calls, commandCall{bin, args, string(input), env, dir})
		joined := strings.Join(args, " ")
		if strings.Contains(joined, "run --rm") {
			for _, key := range []string{"--network bridge", "--cap-drop ALL", "--read-only", "--entrypoint /usr/bin/timeout", "--kill-after=10s 600", "GOPROXY=https://proxy.golang.org", "GOPRIVATE=", "NPM_CONFIG_USERCONFIG=/dev/null"} {
				if !strings.Contains(joined, key) {
					t.Fatal("missing isolation option", key)
				}
			}
			if strings.Contains(joined, "npm ci") && !strings.Contains(joined, "--ignore-scripts") {
				t.Fatal("package scripts enabled")
			}
			for _, arg := range args {
				if strings.HasPrefix(arg, "type=bind,src=") && strings.Contains(arg, ",dst=/manifests") {
					source := strings.Split(strings.TrimPrefix(arg, "type=bind,src="), ",")[0]
					for _, bad := range []string{"app.go", ".npmrc"} {
						if _, err := os.Stat(filepath.Join(source, bad)); !os.IsNotExist(err) {
							t.Fatal("project code/credential config copied to preparation")
						}
					}
				}
				if strings.HasPrefix(arg, "type=bind,src=") && strings.HasSuffix(arg, ",dst=/deps") {
					cache := strings.TrimSuffix(strings.TrimPrefix(arg, "type=bind,src="), ",dst=/deps")
					if err := os.MkdirAll(filepath.Join(cache, "npm", "node_modules"), 0755); err != nil {
						t.Fatal(err)
					}
				}
			}
		}
		return nil, nil
	})
	mounts, err := r.prepareDependencies(context.Background(), root, project, workspace, environment{Socket: "/synthetic.sock", Image: testImage})
	if err != nil {
		t.Fatal(err)
	}
	if len(mounts) != 2 {
		t.Fatal(mounts)
	}
	count := len(calls)
	if _, err = r.prepareDependencies(context.Background(), root, project, workspace, environment{Socket: "/synthetic.sock", Image: testImage}); err != nil {
		t.Fatal(err)
	}
	if count != len(calls) {
		t.Fatal("valid dependency cache re-downloaded")
	}
	runCount := 0
	cleanCount := 0
	for _, c := range calls {
		joined := strings.Join(c.args, " ")
		if strings.Contains(joined, "run --rm") {
			runCount++
		}
		if strings.Contains(joined, "rm --force") {
			cleanCount++
		}
	}
	if runCount != 2 || cleanCount != 2 {
		t.Fatalf("runs=%d cleanup=%d", runCount, cleanCount)
	}
}
func TestDependencyCancellationStillRemovesPreparationContainer(t *testing.T) {
	root := t.TempDir()
	cleanup := false
	r := &localRuntime{home: "/synthetic", platform: "linux", lookPath: func(s string) (string, error) { return s, nil }}
	r.command = commandFunc(func(ctx context.Context, bin string, args []string, input []byte, env []string, dir string) ([]byte, error) {
		if strings.Contains(strings.Join(args, " "), "rm --force") {
			cleanup = true
			if ctx.Err() != nil {
				t.Fatal("cleanup inherited cancelled context")
			}
			return nil, nil
		}
		return nil, context.Canceled
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := r.download(ctx, root, "/inputs", "/cache", environment{Socket: "/synthetic.sock", Image: testImage}, "/manifests", []string{"go", "mod", "download"})
	if !cleanup || !errors.Is(err, context.Canceled) {
		t.Fatal(cleanup, err)
	}
}
