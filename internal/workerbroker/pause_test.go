package workerbroker

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shhac/agent-assistant/internal/integrations/worker"
)

func awaitPaused(t *testing.T, b *Broker, id string) storedRun {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		r, _ := b.snapshot(id)
		if r.Run.Status == "paused" {
			return r
		}
		select {
		case <-deadline:
			t.Fatal("worker did not confirm paused", r.Run)
		case <-time.After(5 * time.Millisecond):
		}
	}
}
func TestPauseFinishesModelBoundarySkipsToolsAndWaitsForCleanup(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	cleanup := make(chan struct{})
	releaseCleanup := make(chan struct{})
	var tools atomic.Int32
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"should-not-run","type":"function","function":{"name":"run_command","arguments":"{\"command\":\"echo skip\"}"}}]}}]}`))
	}))
	defer model.Close()
	d := &fakeDocker{}
	b, _ := newFixture(t, model.URL, d)
	b.cfg.Command = CommandFunc(func(ctx context.Context, args []string, in []byte) ([]byte, error) {
		if args[0] == "exec" {
			tools.Add(1)
		}
		if args[0] == "rm" {
			close(cleanup)
			select {
			case <-releaseCleanup:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		return d.Run(ctx, args, in)
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- b.Run(ctx) }()
	defer func() { cancel(); <-done; b.Close() }()
	response := request(t, b, "/runs", "dispatch-one", startRequest())
	var run worker.Run
	_ = json.Unmarshal(response.Body.Bytes(), &run)
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("model not started")
	}
	response = request(t, b, "/runs/"+run.ID+"/pause", "pause-once", struct{}{})
	_ = json.Unmarshal(response.Body.Bytes(), &run)
	if response.Code != 200 || run.Status != "running" || !run.PauseRequested {
		t.Fatal("pause falsely confirmed before boundary", response.Code, run)
	}
	select {
	case <-cleanup:
		t.Fatal("pause killed active model")
	default:
	}
	close(release)
	select {
	case <-cleanup:
	case <-time.After(3 * time.Second):
		t.Fatal("cleanup not started")
	}
	r, _ := b.snapshot(run.ID)
	if r.Run.Status != "running" || tools.Load() != 0 {
		t.Fatal("pause ran tools or released before cleanup", r.Run, tools.Load())
	}
	close(releaseCleanup)
	r = awaitPaused(t, b, run.ID)
	if len(r.Transcript) == 0 || r.Run.PauseRequested {
		t.Fatal("checkpoint lost or request remained", r)
	}
	response = request(t, b, "/runs/"+run.ID+"/messages", "must-not-wake", map[string]string{"message": "wake"})
	if response.Code != 409 {
		t.Fatal("ordinary message resumed paused worker")
	}
}
func TestPauseSurvivesBrokerRestartAndResumeReusesWorkspace(t *testing.T) {
	b, cfg := newFixture(t, "http://127.0.0.1:9999", &fakeDocker{})
	response := request(t, b, "/runs", "dispatch-one", startRequest())
	var run worker.Run
	_ = json.Unmarshal(response.Body.Bytes(), &run)
	if err := b.update(run.ID, func(r *storedRun) error {
		r.Run.Status = "running"
		r.Run.PauseRequested = true
		r.PendingStatus = "paused"
		r.Transcript = []modelMessage{{Role: "user", Content: "Preserved context"}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	b.Close()
	reopened, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	r, _ := reopened.snapshot(run.ID)
	if r.Run.Status != "paused" || r.Run.PauseRequested {
		t.Fatal("lost owner hold", r.Run)
	}
	response = request(t, reopened, "/runs/"+run.ID+"/resume", "owner-resume", map[string]string{"instruction": "Continue"})
	if response.Code != 200 {
		t.Fatal(response.Code, response.Body.String())
	}
	resumed, _ := reopened.snapshot(run.ID)
	if resumed.Run.Status != "queued" || len(resumed.Transcript) != 1 || resumed.Run.ID != run.ID {
		t.Fatal("resume discarded original session", resumed)
	}
}

func TestPauseLetsActiveToolFinishButSkipsRemainingOperations(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"first","type":"function","function":{"name":"run_command","arguments":"{\"command\":\"echo one\"}"}},{"id":"second","type":"function","function":{"name":"run_command","arguments":"{\"command\":\"echo two\"}"}}]}}]}`))
	}))
	defer model.Close()
	d := &fakeDocker{}
	b, _ := newFixture(t, model.URL, d)
	b.cfg.Command = CommandFunc(func(ctx context.Context, args []string, in []byte) ([]byte, error) {
		if args[0] == "exec" {
			if calls.Add(1) == 1 {
				close(started)
				select {
				case <-release:
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}
		}
		return d.Run(ctx, args, in)
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- b.Run(ctx) }()
	defer func() { cancel(); <-done; b.Close() }()
	response := request(t, b, "/runs", "dispatch-one", startRequest())
	var run worker.Run
	_ = json.Unmarshal(response.Body.Bytes(), &run)
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("tool did not start")
	}
	response = request(t, b, "/runs/"+run.ID+"/pause", "pause-tool", struct{}{})
	if response.Code != 200 {
		t.Fatal(response.Body.String())
	}
	current, _ := b.snapshot(run.ID)
	if current.Run.Status != "running" {
		t.Fatal("active tool interrupted by pause")
	}
	close(release)
	current = awaitPaused(t, b, run.ID)
	if calls.Load() != 1 || len(current.Commands) != 1 || !current.Commands[0].Success {
		t.Fatal("pause lost active result or executed another command", calls.Load(), current.Commands)
	}
	repaired := repairTranscript(current.Transcript)
	if repaired[len(repaired)-1].ToolCallID != "second" {
		t.Fatal("resume cannot reconcile deferred calls", repaired)
	}
}
func TestPausePreservesPeerOutboxBeforeResume(t *testing.T) {
	b, _ := newFixture(t, "http://127.0.0.1:9999", &fakeDocker{})
	defer b.Close()
	response := request(t, b, "/runs", "dispatch-one", startRequest())
	var run worker.Run
	_ = json.Unmarshal(response.Body.Bytes(), &run)
	_ = b.update(run.ID, func(r *storedRun) error {
		r.Run.Status = "running"
		r.PendingMessage = &worker.PeerMessage{RequestID: "peer-msg", TargetAgentID: "peer", Message: "Evidence"}
		return nil
	})
	response = request(t, b, "/runs/"+run.ID+"/pause", "pause-outbox", struct{}{})
	if response.Code != 200 {
		t.Fatal(response.Body.String())
	}
	b.finalize(run.ID, "waiting", "Peer delivery pending", nil)
	current, _ := b.snapshot(run.ID)
	if current.Run.Status != "paused" || current.Run.Message == nil || current.PendingMessage != nil {
		t.Fatal("outbox lost on paused cleanup", current)
	}
	response = request(t, b, "/runs/"+run.ID+"/resume", "resume-outbox", map[string]string{"instruction": "Continue"})
	_ = json.Unmarshal(response.Body.Bytes(), &run)
	if response.Code != 200 || run.Status != "waiting" || run.Message == nil {
		t.Fatal("resumed before routing saved peer message", run)
	}
}

func TestPauseCleanupFailureNeverClaimsCheckpointIsStopped(t *testing.T) {
	b, _ := newFixture(t, "http://127.0.0.1:9999", &fakeDocker{})
	defer b.Close()
	response := request(t, b, "/runs", "dispatch-one", startRequest())
	var run worker.Run
	_ = json.Unmarshal(response.Body.Bytes(), &run)
	_ = b.update(run.ID, func(r *storedRun) error {
		r.Run.Status = "running"
		r.PendingStatus = "paused"
		r.Run.PauseRequested = true
		return nil
	})
	b.cleanupUncertain(run.ID, "Cleanup unconfirmed")
	current, _ := b.snapshot(run.ID)
	if current.Run.Status != "running" || current.PendingStatus != "paused" {
		t.Fatal("unconfirmed cleanup released execution", current)
	}
	b.terminal(run.ID, "interrupted", "Shutdown", nil)
	current, _ = b.snapshot(run.ID)
	if current.PendingStatus != "paused" {
		t.Fatal("shutdown lost owner pause")
	}
}
