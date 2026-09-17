package workerbroker

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shhac/agent-assistant/internal/engine"
	"github.com/shhac/agent-assistant/internal/integrations/worker"
	"github.com/shhac/lib-agent-harness/completion"
)

func providerRun(t *testing.T, b *Broker) string {
	t.Helper()
	resp := request(t, b, "/runs", "dispatch-one", startRequest())
	if resp.Code != 201 {
		t.Fatal(resp.Body.String())
	}
	var run worker.Run
	if err := json.Unmarshal(resp.Body.Bytes(), &run); err != nil {
		t.Fatal(err)
	}
	if err := b.update(run.ID, func(r *storedRun) error {
		r.Run.Status = "running"
		r.ModelCalls = 1
		r.Transcript = []modelMessage{{Role: "user", Content: "saved request"}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return run.ID
}
func TestProviderRetryPublishesOnlyAfterCleanupAndSurvivesRestart(t *testing.T) {
	b, cfg := newFixture(t, "https://provider.test/v1", &fakeDocker{})
	id := providerRun(t, b)
	b.modelFailure(id, &completion.RequestError{Kind: completion.ErrorOverloaded, RetryAfter: time.Minute})
	pending, err := b.snapshot(id)
	if err != nil {
		t.Fatal(err)
	}
	if pending.Run.Status != "running" || pending.PendingStatus != "retry_wait" || pending.Run.ProviderFailures != 1 || !pending.Run.RetryAt.After(time.Now().Add(50*time.Second)) {
		t.Fatalf("retry released execution before cleanup: %+v", pending)
	}
	b.cleanupUncertain(id, "cleanup unconfirmed")
	held, _ := b.snapshot(id)
	if held.Run.Status != "running" || held.PendingStatus != "retry_wait" {
		t.Fatal("cleanup uncertainty published retry")
	}
	if err = b.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	recovered, err := restarted.snapshot(id)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Run.Status != "retry_wait" || recovered.PendingStatus != "" || !recovered.Run.RetryAt.Equal(pending.Run.RetryAt) || !reflect.DeepEqual(recovered.Transcript, pending.Transcript) || recovered.ModelCalls != 1 {
		t.Fatalf("restart lost scheduled retry or context: %+v", recovered)
	}
	if resp := request(t, restarted, "/runs/"+id+"/resume", "too-early", map[string]string{"instruction": "continue"}); resp.Code != 409 {
		t.Fatalf("broker resumed early: %d %s", resp.Code, resp.Body.String())
	}
	if resp := request(t, restarted, "/runs/"+id+"/messages", "bypass", map[string]string{"message": "wake up"}); resp.Code != 409 {
		t.Fatalf("message bypassed backoff: %d", resp.Code)
	}
	if err = restarted.update(id, func(r *storedRun) error { r.Run.RetryAt = time.Now().Add(-time.Second); return nil }); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if resp := request(t, restarted, "/runs/"+id+"/resume", "retry-once", map[string]string{"instruction": "continue"}); resp.Code != 200 {
			t.Fatalf("due resume failed: %d %s", resp.Code, resp.Body.String())
		}
	}
	queued, _ := restarted.snapshot(id)
	if queued.Run.Status != "queued" || len(queued.Messages) != 1 {
		t.Fatal("resume replay duplicated instruction", queued.Messages)
	}
}
func TestProviderRecoveryRefusesUnknownExhaustedAndOwnerHeldFailures(t *testing.T) {
	for _, kind := range []completion.ErrorKind{completion.ErrorUnknown, completion.ErrorAuthentication, completion.ErrorContextLimit} {
		t.Run(string(kind), func(t *testing.T) {
			b, _ := newFixture(t, "https://provider.test/v1", &fakeDocker{})
			defer b.Close()
			id := providerRun(t, b)
			b.modelFailure(id, &completion.RequestError{Kind: kind})
			r, _ := b.snapshot(id)
			if r.PendingStatus != "blocked" || !r.Run.RetryAt.IsZero() || r.Run.ProviderFailures != 0 {
				t.Fatalf("nonretryable failure auto scheduled: %+v", r)
			}
		})
	}
	for _, hold := range []string{"paused", "cancelled"} {
		t.Run(hold, func(t *testing.T) {
			b, _ := newFixture(t, "https://provider.test/v1", &fakeDocker{})
			defer b.Close()
			id := providerRun(t, b)
			b.update(id, func(r *storedRun) error { r.PendingStatus = hold; return nil })
			b.modelFailure(id, &completion.RequestError{Kind: completion.ErrorUnavailable})
			r, _ := b.snapshot(id)
			if r.PendingStatus != hold || !r.Run.RetryAt.IsZero() || r.Run.ProviderFailures != 0 {
				t.Fatal("provider failure overwrote owner hold")
			}
		})
	}
	t.Run("provider recovery exhausted", func(t *testing.T) {
		b, _ := newFixture(t, "https://provider.test/v1", &fakeDocker{})
		defer b.Close()
		id := providerRun(t, b)
		b.update(id, func(r *storedRun) error { r.Run.ProviderFailures = maxProviderFailures - 1; return nil })
		b.modelFailure(id, &completion.RequestError{Kind: completion.ErrorUnavailable})
		r, _ := b.snapshot(id)
		if r.PendingStatus != "blocked" || !r.Run.RetryAt.IsZero() {
			t.Fatal("exhausted recovery scheduled more work")
		}
	})
	// A long history of successful calls is diagnostic only. It must not shorten
	// the recovery a genuine provider outage is entitled to.
	t.Run("long history keeps recovery", func(t *testing.T) {
		b, _ := newFixture(t, "https://provider.test/v1", &fakeDocker{})
		defer b.Close()
		id := providerRun(t, b)
		b.update(id, func(r *storedRun) error { r.ModelCalls = 512; return nil })
		b.modelFailure(id, &completion.RequestError{Kind: completion.ErrorUnavailable})
		r, _ := b.snapshot(id)
		if r.PendingStatus != "retry_wait" || r.Run.RetryAt.IsZero() {
			t.Fatal("call history was treated as a spent allowance", r.PendingStatus)
		}
	})
}
func awaitProviderStatus(t *testing.T, b *Broker, id, status string) storedRun {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		r, err := b.snapshot(id)
		if err != nil {
			t.Fatal(err)
		}
		if r.Run.Status == status {
			return r
		}
		time.Sleep(5 * time.Millisecond)
	}
	r, _ := b.snapshot(id)
	t.Fatalf("status did not reach %s: %+v", status, r.Run)
	return storedRun{}
}
func TestProviderRecoveryPreservesToolEvidenceWithoutReplay(t *testing.T) {
	var calls atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := calls.Add(1)
		if count == 2 {
			w.WriteHeader(503)
			w.Write([]byte(`{"error":{"type":"unavailable_error"}}`))
			return
		}
		call := toolCall{ID: "write-once", Type: "function"}
		call.Function.Name = "write_file"
		call.Function.Arguments = `{"path":"hello.txt","content":"after\n"}`
		if count > 2 {
			call.ID = "finish"
			call.Function.Name = "finish"
			call.Function.Arguments = `{"summary":"Preserved previous write after provider recovery"}`
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": modelMessage{Role: "assistant", ToolCalls: []toolCall{call}}}}})
	}))
	defer provider.Close()
	docker := &fakeDocker{}
	b, _ := newFixture(t, provider.URL, docker)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- b.Run(ctx) }()
	defer func() { cancel(); <-done; b.Close() }()
	response := request(t, b, "/runs", "dispatch-one", startRequest())
	var run worker.Run
	json.Unmarshal(response.Body.Bytes(), &run)
	waiting := awaitProviderStatus(t, b, run.ID, "retry_wait")
	if waiting.ModelCalls != 2 || len(waiting.Transcript) != 2 {
		t.Fatalf("failed completion altered saved conversation: calls=%d transcript=%+v", waiting.ModelCalls, waiting.Transcript)
	}
	docker.mu.Lock()
	stopped := docker.stopped
	docker.mu.Unlock()
	if !stopped {
		t.Fatal("retry wait published before container removal")
	}
	if err := b.update(run.ID, func(r *storedRun) error { r.Run.RetryAt = time.Now().Add(-time.Second); return nil }); err != nil {
		t.Fatal(err)
	}
	if resp := request(t, b, "/runs/"+run.ID+"/resume", "provider-retry", map[string]string{"instruction": "Resume saved progress"}); resp.Code != 200 {
		t.Fatal(resp.Body.String())
	}
	completed := awaitProviderStatus(t, b, run.ID, "completed")
	if completed.ModelCalls != 3 || calls.Load() != 3 || completed.Run.ProviderFailures != 0 || !completed.Run.RetryAt.IsZero() {
		t.Fatalf("unexpected recovery counters: %+v calls=%d", completed, calls.Load())
	}
	docker.mu.Lock()
	writes := 0
	for _, args := range docker.calls {
		if len(args) > 0 && args[0] == "exec" && contains(args, "--interactive") {
			writes++
		}
	}
	docker.mu.Unlock()
	if writes != 1 {
		t.Fatalf("replayed acknowledged write %d times", writes)
	}
}
func TestProviderCleanupFailureCannotBecomeRetryable(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }))
	defer provider.Close()
	docker := &fakeDocker{}
	b, _ := newFixture(t, provider.URL, docker)
	b.cfg.Command = CommandFunc(func(ctx context.Context, args []string, data []byte) ([]byte, error) {
		if args[0] == "rm" {
			return nil, errors.New("cleanup unavailable")
		}
		return docker.Run(ctx, args, data)
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- b.Run(ctx) }()
	defer func() { cancel(); <-done; b.Close() }()
	response := request(t, b, "/runs", "dispatch-one", startRequest())
	var run worker.Run
	json.Unmarshal(response.Body.Bytes(), &run)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		r, _ := b.snapshot(run.ID)
		if strings.Contains(r.Run.Summary, "cleanup could not be confirmed") {
			if r.Run.Status != "running" || r.PendingStatus != "retry_wait" {
				t.Fatal("uncertain cleanup advertised safe retry")
			}
			if resp := request(t, b, "/runs/"+run.ID+"/resume", "unsafe", map[string]string{"instruction": "continue"}); resp.Code != 409 {
				t.Fatal("uncertain cleanup resumed")
			}
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("cleanup failure not observed")
}

func TestNonRetryableModelFailureRemainsExplicitlyResumable(t *testing.T) {
	for _, tc := range []struct {
		name    string
		failure error
		kind    string
	}{
		{"authentication", &completion.RequestError{Kind: completion.ErrorAuthentication}, "authentication"},
		{"context", &completion.RequestError{Kind: completion.ErrorContextLimit}, "context_limit"},
		{"working-context", engine.ErrContextPressure, "context_limit"},
		{"unknown", errors.New("secret raw transport content"), "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, _ := newFixture(t, "https://provider.test/v1", &fakeDocker{})
			defer b.Close()
			id := providerRun(t, b)
			b.modelFailure(id, tc.failure)
			r, _ := b.snapshot(id)
			if r.PendingStatus != "blocked" || r.Run.ProviderFailureKind != tc.kind || !r.Run.RetryAt.IsZero() || strings.Contains(r.PendingSummary, "secret") {
				t.Fatalf("unsafe/invisible model failure: %+v", r)
			}
			b.finalize(id, r.PendingStatus, r.PendingSummary, nil)
			if response := request(t, b, "/runs/"+id+"/messages", "automatic-wake", map[string]string{"message": "continue"}); response.Code != 409 {
				t.Fatal("message automatically recovered a model failure")
			}
			if response := request(t, b, "/runs/"+id+"/resume", "owner-retry", map[string]string{"instruction": "Owner inspected saved work and corrected setup"}); response.Code != 200 {
				t.Fatalf("explicit resume denied: %d %s", response.Code, response.Body.String())
			}
		})
	}
	for _, hold := range []string{"paused", "cancelled"} {
		t.Run("held-"+hold, func(t *testing.T) {
			b, _ := newFixture(t, "https://provider.test/v1", &fakeDocker{})
			defer b.Close()
			id := providerRun(t, b)
			b.update(id, func(r *storedRun) error { r.PendingStatus = hold; r.PendingSummary = "Owner hold"; return nil })
			b.modelFailure(id, &completion.RequestError{Kind: completion.ErrorAuthentication})
			r, _ := b.snapshot(id)
			if r.PendingStatus != hold || r.PendingSummary != "Owner hold" || r.Run.ProviderFailureKind != "" {
				t.Fatal("authentication failure overwrote owner hold")
			}
		})
	}
}

func TestRestartPreservesPendingModelBlockerAfterCleanup(t *testing.T) {
	for _, kind := range []string{"authentication", "unknown", "budget"} {
		t.Run(kind, func(t *testing.T) {
			b, cfg := newFixture(t, "https://provider.test/v1", &fakeDocker{})
			id := providerRun(t, b)
			switch kind {
			case "authentication":
				b.modelFailure(id, &completion.RequestError{Kind: completion.ErrorAuthentication})
			case "unknown":
				b.modelFailure(id, errors.New("unclassified model failure"))
			case "budget":
				b.terminal(id, "blocked", "Cumulative model allowance exhausted")
			}
			before, _ := b.snapshot(id)
			if before.Run.Status != "running" || before.PendingStatus != "blocked" {
				t.Fatal("fixture did not stop before cleanup")
			}
			if err := b.Close(); err != nil {
				t.Fatal(err)
			}
			restarted, err := New(cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer restarted.Close()
			after, err := restarted.snapshot(id)
			if err != nil {
				t.Fatal(err)
			}
			if after.Run.Status != "blocked" || after.PendingStatus != "" || after.Run.Summary != before.PendingSummary || after.Run.ProviderFailureKind != before.Run.ProviderFailureKind || after.ModelCalls != before.ModelCalls {
				t.Fatalf("restart converted blocker into automatic recovery: %+v", after)
			}
		})
	}
}

func TestModelFailureDiagnosticsPersistAndClearOnSuccess(t *testing.T) {
	b, _ := newFixture(t, "https://provider.test/v1", &fakeDocker{})
	defer b.Close()
	id := providerRun(t, b)
	exit := 1
	b.modelFailure(id, &completion.RequestError{Kind: completion.ErrorStructuredOutputLimit, Engine: "claude", Phase: completion.PhaseResponse, Code: "error_max_structured_output_retries", ExitCode: &exit})
	r, _ := b.snapshot(id)
	if r.Run.ProviderFailureKind != "structured_output_limit" || r.Run.ModelFailureCode != "error_max_structured_output_retries" || r.Run.ModelFailurePhase != "response" || r.Run.ModelFailureEngine != "claude" || r.Run.ModelExitCode == nil || *r.Run.ModelExitCode != 1 {
		t.Fatalf("missing diagnostics: %+v", r.Run)
	}
	b.clearProviderFailure(id)
	r, _ = b.snapshot(id)
	if r.Run.ProviderFailureKind != "" || r.Run.ModelFailureCode != "" || r.Run.ModelExitCode != nil {
		t.Fatal("stale diagnosis survived successful recovery")
	}
}

// An owner seeing "unknown" cannot tell whether the provider returned an
// unrecognised classification or whether no typed envelope arrived at all.
// Recording which evidence existed keeps that distinction without inventing a
// cause for either case.
func TestModelFailureRecordsWhichEvidenceWasAvailable(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		kind     string
		evidence string
	}{
		{"untyped failure", errors.New("worker process ended unexpectedly"), "unknown", evidenceUntyped},
		{"unclassified provider kind", &completion.RequestError{Kind: completion.ErrorKind("teapot")}, "unknown", evidenceUnclassified},
		{"classified provider kind", &completion.RequestError{Kind: completion.ErrorAuthentication}, "authentication", evidenceTyped},
		{"local context measurement", engine.ErrContextPressure, "context_limit", evidenceLocalPreflight},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b, _ := newFixture(t, "https://provider.test/v1", &fakeDocker{})
			defer b.Close()
			id := providerRun(t, b)
			b.modelFailure(id, c.err)
			r, err := b.snapshot(id)
			if err != nil {
				t.Fatal(err)
			}
			if r.Run.ProviderFailureKind != c.kind {
				t.Fatalf("kind = %q, want %q", r.Run.ProviderFailureKind, c.kind)
			}
			if r.Run.ModelFailureEvidence != c.evidence {
				t.Fatalf("evidence = %q, want %q", r.Run.ModelFailureEvidence, c.evidence)
			}
			if c.evidence == evidenceUntyped && (r.Run.ModelFailureCode != "" || r.Run.ModelFailurePhase != "") {
				t.Fatalf("invented a diagnosis for an untyped failure: %+v", r.Run)
			}
			b.clearProviderFailure(id)
			r, _ = b.snapshot(id)
			if r.Run.ModelFailureEvidence != "" {
				t.Fatal("stale failure evidence survived successful recovery")
			}
		})
	}
}

// A retryable rejection always arrived as a typed envelope; the scheduled retry
// must not be described as if its classification were missing.
func TestRetryableModelFailureRecordsTypedEvidence(t *testing.T) {
	b, _ := newFixture(t, "https://provider.test/v1", &fakeDocker{})
	defer b.Close()
	id := providerRun(t, b)
	b.modelFailure(id, &completion.RequestError{Kind: completion.ErrorOverloaded, RetryAfter: time.Minute})
	r, err := b.snapshot(id)
	if err != nil {
		t.Fatal(err)
	}
	if r.Run.ModelFailureEvidence != evidenceTyped {
		t.Fatalf("evidence = %q, want %q", r.Run.ModelFailureEvidence, evidenceTyped)
	}
}
