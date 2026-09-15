package managedworkers

import (
	"bytes"
	"context"
	"os/exec"
	"time"
)

type hostCommand struct{}
type output struct{ bytes.Buffer }

func (o *output) Write(p []byte) (int, error) {
	n := len(p)
	available := 64*1024 - o.Len()
	if available > 0 {
		if len(p) > available {
			p = p[:available]
		}
		_, _ = o.Buffer.Write(p)
	}
	return n, nil
}
func (hostCommand) Run(ctx context.Context, bin string, args []string, input []byte, env []string, dir string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = env
	cmd.Dir = dir
	cmd.Stdin = bytes.NewReader(input)
	out := &output{}
	cmd.Stdout = out
	cmd.Stderr = out
	configureCommand(cmd)
	cmd.WaitDelay = 5 * time.Second
	err := cmd.Run()
	return out.Bytes(), commandError(err)
}
