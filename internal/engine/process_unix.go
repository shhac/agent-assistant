//go:build !windows

package engine

import (
	"os/exec"
	"syscall"
)

// Unix descendants remain in the engine's own process group, outside the
// daemon's foreground group. Forced cancellation stops the entire group.
type codexProcess struct{ cmd *exec.Cmd }

func newCodexProcess(cmd *exec.Cmd) (*codexProcess, error) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return &codexProcess{cmd: cmd}, nil
}
func (p *codexProcess) run() error { return p.cmd.Run() }
func (p *codexProcess) stop() {
	if p.cmd.Process != nil {
		_ = syscall.Kill(-p.cmd.Process.Pid, syscall.SIGKILL)
	}
}
func (p *codexProcess) close() {}
