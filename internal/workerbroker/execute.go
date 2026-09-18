package workerbroker

import (
	"context"
	"errors"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// execute runs one admitted attempt. Its lifetime is the daemon's: there is no
// task-wide clock, because stopping useful work at an arbitrary elapsed time is
// not a resource policy. Every individual request, command and container
// operation stays separately bounded, and owner pause and stop are honoured
// between operations.
func (b *Broker) execute(parent context.Context, id string) {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	r, err := b.snapshot(id)
	if err != nil {
		return
	}
	containerCreated := false
	defer func() {
		if containerCreated {
			cleanupCtx, stop := context.WithTimeout(context.Background(), 15*time.Second)
			removeErr := reconcileContainer(cleanupCtx, b.cfg.Command, r)
			stop()
			if removeErr != nil {
				b.reportFailure(id, "container_cleanup", removeErr)
				b.cleanupUncertain(id, "Container cleanup could not be confirmed; execution capacity remains reserved until restart reconciliation")
				return
			}
			b.settleContainerProcesses(id)
		}
		current, getErr := b.snapshot(id)
		if getErr != nil {
			return
		}
		var evidence []string
		if current.WorkDir != "" {
			var artifactErr error
			evidence, artifactErr = b.artifacts(current)
			if artifactErr != nil {
				b.reportFailure(id, "artifact_collection", artifactErr)
				_ = b.update(id, func(run *storedRun) error {
					if run.PendingStatus == "paused" {
						run.PendingSummary += ". Artifact collection failed; inspect the preserved workspace before continuing"
					}
					return nil
				})
				b.finalize(id, "interrupted", "Artifact collection failed; preserve the isolated workspace for inspection", nil)
				return
			}
		}
		status, summary := current.PendingStatus, current.PendingSummary
		if status == "" {
			status = "interrupted"
			summary = "Worker execution interrupted; inspect preserved evidence before recovery"
		}
		b.finalize(id, status, summary, evidence)
	}()
	if b.pauseRequested(id) {
		return
	}
	if r.WorkDir == "" {
		workDir := filepath.Join(b.cfg.StateDir, "runs", id, "workspace")
		baseline, copyErr := copyWorkspace(b.cfg.Workspace, workDir)
		if copyErr != nil {
			b.reportFailure(id, "workspace_copy", copyErr)
			b.terminal(id, "interrupted", copyErr.Error())
			return
		}
		if dependencyErr := b.prepareRunDependencies(ctx, workDir); dependencyErr != nil {
			b.reportFailure(id, "dependency_preparation", dependencyErr)
			b.terminal(id, "interrupted", dependencyErr.Error())
			return
		}
		err = b.update(id, func(run *storedRun) error { run.WorkDir = workDir; run.Baseline = baseline; return nil })
		if err != nil {
			b.terminal(id, "interrupted", "Could not record isolated workspace")
			return
		}
		r.WorkDir = workDir
		r.Baseline = baseline
	}
	commandCtx, stop := context.WithTimeout(ctx, 30*time.Second)
	_, err = b.cfg.Command.Run(commandCtx, b.containerArgs(r), nil)
	stop()
	containerCreated = true
	if err != nil {
		b.reportFailure(id, "container_start", err)
		b.terminal(id, "interrupted", "Could not establish the isolated worker container; no host commands were executed")
		return
	}
	b.runNative(ctx, id, r)
}

func safePath(p string) (string, error) {
	if p == "" || strings.ContainsAny(p, "\x00\n\r") || path.IsAbs(p) || path.Clean(p) == ".." || strings.HasPrefix(path.Clean(p), "../") {
		return "", errors.New("use a relative path inside the isolated workspace")
	}
	return "/workspace/" + path.Clean(p), nil
}
func (b *Broker) pauseRequested(id string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	r := b.state.Runs[id]
	return r == nil || r.PendingStatus == "paused" || r.PendingStatus == "cancelled"
}
