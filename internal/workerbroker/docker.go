package workerbroker

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

type dockerCommand struct{ binary, socket, configDir string }

func newDocker(socket, configDir string) (Commander, error) {
	if runtime.GOOS == "windows" {
		return nil, errors.New("the local worker broker requires macOS or Linux with a local Docker Unix socket")
	}
	binary, err := exec.LookPath("docker")
	if err != nil {
		return nil, errors.New("Docker CLI is required for the isolated worker broker")
	}
	if os.Getuid() == 0 {
		return nil, errors.New("run the worker broker as a non-root user")
	}
	if socket == "" {
		socket = "/var/run/docker.sock"
	}
	if !filepath.IsAbs(socket) || strings.Contains(socket, "://") {
		return nil, errors.New("Docker socket must be an absolute local Unix socket path")
	}
	if err = os.MkdirAll(configDir, 0700); err != nil {
		return nil, err
	}
	return dockerCommand{binary: binary, socket: socket, configDir: configDir}, nil
}

type boundedOutput struct {
	bytes.Buffer
	limit int
}

func (b *boundedOutput) Write(p []byte) (int, error) {
	n := len(p)
	remaining := b.limit - b.Len()
	if remaining > 0 {
		if len(p) > remaining {
			p = p[:remaining]
		}
		_, _ = b.Buffer.Write(p)
	}
	return n, nil
}
func (d dockerCommand) Run(ctx context.Context, args []string, input []byte) ([]byte, error) {
	argv := append([]string{"--host", "unix://" + d.socket, "--config", d.configDir}, args...)
	cmd := exec.CommandContext(ctx, d.binary, argv...)
	// No provider keys, cloud credentials, Docker registry login or home config
	// reach the CLI, and none are added to the container environment.
	cmd.Env = dockerEnvironment()
	cmd.Stdin = bytes.NewReader(input)
	out := &boundedOutput{limit: 64 * 1024}
	cmd.Stdout = out
	cmd.Stderr = out
	err := cmd.Run()
	if err != nil {
		return out.Bytes(), errors.New("Docker operation failed; inspect the worker transcript")
	}
	return out.Bytes(), nil
}
func (b *Broker) containerArgs(r storedRun) []string {
	mount := "type=bind,src=" + r.WorkDir + ",dst=/workspace"
	if !contains(r.Request.Capabilities, "implement") {
		mount += ",readonly"
	}
	return []string{"run", "--detach", "--pull=never", "--name", r.Container, "--label", "agent-assistant.worker=" + r.Run.ID, "--network", "none", "--cap-drop", "ALL", "--security-opt", "no-new-privileges", "--read-only", "--pids-limit", "128", "--memory", "1g", "--cpus", "2", "--user", strconv.Itoa(os.Getuid()) + ":" + strconv.Itoa(os.Getgid()), "--tmpfs", "/tmp:rw,nosuid,nodev,size=256m", "--mount", mount, "--workdir", "/workspace", "--env", "HOME=/tmp/home", "--entrypoint", "/bin/sh", b.cfg.Image, "-c", "mkdir -p /tmp/home; while :; do sleep 3600; done"}
}

func dockerEnvironment() []string { return []string{"PATH=" + os.Getenv("PATH"), "LANG=C.UTF-8"} }
