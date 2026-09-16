package workerbroker

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/shhac/agent-assistant/internal/integrations/worker"
)

func TestActiveCancellationWaitsForCleanup(t *testing.T) {
	modelStarted := make(chan struct{}, 1)
	releaseModel := make(chan struct{})
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case modelStarted <- struct{}{}:
		default:
		}
		select {
		case <-r.Context().Done():
		case <-releaseModel:
		}
	}))
	defer model.Close()
	defer close(releaseModel)
	d := &fakeDocker{}
	b, _ := newFixture(t, model.URL, d)
	cleanupStarted := make(chan struct{}, 1)
	releaseCleanup := make(chan struct{})
	b.cfg.Command = CommandFunc(func(ctx context.Context, args []string, in []byte) ([]byte, error) {
		if args[0] == "rm" {
			select {
			case cleanupStarted <- struct{}{}:
			default:
			}
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
	if json.Unmarshal(response.Body.Bytes(), &run) != nil {
		t.Fatal(response.Body.String())
	}
	select {
	case <-modelStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("model did not start")
	}
	response = request(t, b, "/runs/"+run.ID+"/cancel", "cancel-once", struct{}{})
	if response.Code != 200 {
		t.Fatal(response.Body.String())
	}
	json.Unmarshal(response.Body.Bytes(), &run)
	if run.Status != "running" {
		t.Fatal("cancel released capacity before cleanup", run.Status)
	}
	select {
	case <-cleanupStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("cleanup did not start")
	}
	for _, action := range []string{"messages", "resume"} {
		payload := map[string]string{"message": "continue"}
		if action == "resume" {
			payload = map[string]string{"instruction": "continue"}
		}
		response = request(t, b, "/runs/"+run.ID+"/"+action, "forbidden-"+action, payload)
		if response.Code != 409 {
			t.Fatal("pending cancellation accepted execution", action, response.Code)
		}
	}
	current, _ := b.snapshot(run.ID)
	if current.Run.Status != "running" || current.PendingStatus != "cancelled" {
		t.Fatal(current.Run.Status, current.PendingStatus)
	}
	close(releaseCleanup)
	deadline := time.After(3 * time.Second)
	for {
		current, _ = b.snapshot(run.ID)
		if current.Run.Status == "cancelled" {
			break
		}
		select {
		case <-deadline:
			t.Fatal("cleanup did not finalize cancellation")
		case <-time.After(5 * time.Millisecond):
		}
	}
	response = request(t, b, "/runs/"+run.ID+"/cancel", "cancel-once", struct{}{})
	json.Unmarshal(response.Body.Bytes(), &run)
	if run.Status != "cancelled" {
		t.Fatal("duplicate cancel did not return final state")
	}
}
func TestQueuedCancellationNeverDispatches(t *testing.T) {
	b, _ := newFixture(t, "http://127.0.0.1:9999", &fakeDocker{})
	defer b.Close()
	response := request(t, b, "/runs", "dispatch-one", startRequest())
	var run worker.Run
	json.Unmarshal(response.Body.Bytes(), &run)
	response = request(t, b, "/runs/"+run.ID+"/cancel", "cancel-queued", struct{}{})
	json.Unmarshal(response.Body.Bytes(), &run)
	if run.Status != "cancelled" {
		t.Fatal(run.Status)
	}
	current, _ := b.snapshot(run.ID)
	if current.PendingStatus != "" {
		t.Fatal("queued cancellation unnecessarily pending")
	}
}
func TestCleanupFailureDoesNotClaimCancelled(t *testing.T) {
	b, _ := newFixture(t, "http://127.0.0.1:9999", &fakeDocker{})
	defer b.Close()
	response := request(t, b, "/runs", "dispatch-one", startRequest())
	var run worker.Run
	json.Unmarshal(response.Body.Bytes(), &run)
	b.update(run.ID, func(r *storedRun) error {
		r.Run.Status = "running"
		r.PendingStatus = "cancelled"
		r.PendingSummary = "Cancel after cleanup"
		return nil
	})
	b.cleanupUncertain(run.ID, "Cleanup unavailable")
	current, _ := b.snapshot(run.ID)
	if current.Run.Status != "running" || current.PendingStatus != "cancelled" {
		t.Fatal("unknown cleanup reported terminal", current.Run.Status)
	}
	b.terminal(run.ID, "interrupted", "Context cancelled")
	current, _ = b.snapshot(run.ID)
	if current.PendingStatus != "cancelled" {
		t.Fatal("interruption overwrote requested cancellation")
	}
	b.finalize(run.ID, "interrupted", "Artifact unavailable", nil)
	current, _ = b.snapshot(run.ID)
	if current.Run.Status != "cancelled" {
		t.Fatal("confirmed stop did not honor cancellation")
	}
}
func TestRestartFinishesPersistedCancellation(t *testing.T) {
	d := &fakeDocker{}
	b, cfg := newFixture(t, "http://127.0.0.1:9999", d)
	response := request(t, b, "/runs", "dispatch-one", startRequest())
	var run worker.Run
	json.Unmarshal(response.Body.Bytes(), &run)
	b.update(run.ID, func(r *storedRun) error { r.Run.Status = "running"; r.PendingStatus = "cancelled"; return nil })
	b.Close()
	cfg.Command = CommandFunc(func(ctx context.Context, args []string, in []byte) ([]byte, error) {
		if args[0] == "container" && args[1] == "inspect" {
			return nil, errors.New("absent")
		}
		return d.Run(ctx, args, in)
	})
	reopened, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	current, _ := reopened.snapshot(run.ID)
	if current.Run.Status != "cancelled" {
		t.Fatal(current.Run.Status)
	}
}

func TestCancelAfterConfirmedCleanupDoesNotReopenRun(t *testing.T) {
	b, _ := newFixture(t, "http://127.0.0.1:9999", &fakeDocker{})
	defer b.Close()
	response := request(t, b, "/runs", "dispatch-one", startRequest())
	var run worker.Run
	json.Unmarshal(response.Body.Bytes(), &run)
	b.update(run.ID, func(r *storedRun) error { r.Run.Status = "completed"; return nil })
	b.mu.Lock()
	b.active[run.ID] = func() {}
	b.mu.Unlock()
	response = request(t, b, "/runs/"+run.ID+"/cancel", "late-cancel", struct{}{})
	json.Unmarshal(response.Body.Bytes(), &run)
	if run.Status != "completed" {
		t.Fatal("post-finalize active-map window reopened completed work", run.Status)
	}
}
