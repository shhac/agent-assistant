package workerbroker

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/shhac/agent-assistant/internal/integrations/worker"
	"github.com/shhac/lib-agent-harness/completion"
)

type diagnosticTransport func(*http.Request) (*http.Response, error)

func (f diagnosticTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestInvalidWorkerToolCallHasSafeNonretryableDiagnostic(t *testing.T) {
	secret := "synthetic-private-output"
	var valid toolCall
	valid.ID, valid.Type, valid.Function.Name, valid.Function.Arguments = "one", "function", "finish", `{}`
	tests := []struct {
		name      string
		alter     func(*toolCall)
		duplicate bool
	}{
		{"missing-id", func(c *toolCall) { c.ID = "" }, false},
		{"duplicate-id", func(*toolCall) {}, true},
		{"type", func(c *toolCall) { c.Type = secret }, false},
		{"name", func(c *toolCall) { c.Function.Name = secret }, false},
		{"arguments", func(c *toolCall) { c.Function.Arguments = secret }, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			call := valid
			tt.alter(&call)
			calls := []toolCall{call}
			if tt.duplicate {
				calls = append(calls, call)
			}
			payload, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"message": modelMessage{Role: "assistant", Content: secret, ToolCalls: calls}}}})
			requests := 0
			b := &Broker{cfg: Config{Engine: "openai-compatible", Model: "fixture", ModelEndpoint: "https://model.test", HTTPClient: &http.Client{Transport: diagnosticTransport(func(*http.Request) (*http.Response, error) {
				requests++
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(payload)))}, nil
			})}}}
			reply, err := b.completeForRun(context.Background(), "", nil)
			var failure *completion.RequestError
			var safe interface{ SafeDiagnostic() string }
			if !errors.As(err, &failure) || failure.Code != "invalid_worker_tool_call" || failure.Phase != completion.PhaseResponse || failure.Retryable() {
				t.Fatalf("missing tool validation diagnostic: %v", err)
			}
			if !errors.As(err, &safe) || strings.Contains(safe.SafeDiagnostic(), secret) || strings.Contains(err.Error(), secret) {
				t.Fatal("leaked model output")
			}
			if requests != 1 || len(reply.ToolCalls) != 0 {
				t.Fatal("invalid batch escaped or was retried")
			}
		})
	}
}

// A resource hold is not a provider failure. It must stop before the transport,
// leave accounting and context untouched, and never be classified as something
// worth retrying.
func TestResourceHoldStopsBeforeTransportWithoutFailureClassification(t *testing.T) {
	for _, summary := range []bool{false, true} {
		t.Run(map[bool]string{false: "completion", true: "summary"}[summary], func(t *testing.T) {
			b, _ := newFixture(t, "https://model.test", &fakeDocker{})
			defer b.Close()
			b.cfg.HTTPClient = &http.Client{Transport: diagnosticTransport(func(*http.Request) (*http.Response, error) {
				t.Fatal("held admission reached transport")
				return nil, nil
			})}
			b.cfg.TokenBudget = func() int64 { return 5000 }
			id := contextRun(t, b)
			if err := b.update(id, func(r *storedRun) error { r.UsageInputTokens = 5000; return nil }); err != nil {
				t.Fatal(err)
			}
			before, _ := b.snapshot(id)
			var err error
			if summary {
				_, err = b.prepareContext(context.Background(), id)
			} else {
				_, err = b.completeForRun(context.Background(), id, nil)
			}
			if !errors.Is(err, worker.ErrResourceHold) {
				t.Fatalf("lost resource-hold identity: %v", err)
			}
			var failure *completion.RequestError
			if errors.As(err, &failure) {
				t.Fatalf("hold carried a provider classification: %+v", failure)
			}
			after, _ := b.snapshot(id)
			if before.ModelCalls != after.ModelCalls || !reflect.DeepEqual(before.Transcript, after.Transcript) || len(after.ContextCheckpoints) != 0 || after.PendingUsage != nil {
				t.Fatal("hold changed accounting or context")
			}
			if after.Run.ResourceHold == nil || after.Run.ResourceHold.Kind != worker.HoldTokenBudget || !after.Run.ResourceHold.OwnerAction {
				t.Fatalf("budget hold not recorded as an owner decision: %+v", after.Run.ResourceHold)
			}
			if after.Run.ProviderFailures != 0 || !after.Run.RetryAt.IsZero() {
				t.Fatal("hold spent provider recovery allowance")
			}
		})
	}
}

func TestWorkerSummaryDiagnosticIsPreservedInBlockedRun(t *testing.T) {
	b, _ := newFixture(t, "https://model.test", &fakeDocker{})
	defer b.Close()
	id := contextRun(t, b)
	private := "synthetic-private-summary"
	payload, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"message": modelMessage{Role: "assistant", Content: strings.Repeat(private, 1000)}}}})
	requests := 0
	b.cfg.HTTPClient = &http.Client{Transport: diagnosticTransport(func(*http.Request) (*http.Response, error) {
		requests++
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(payload)))}, nil
	})}
	before, _ := b.snapshot(id)
	_, err := b.prepareContext(context.Background(), id)
	if err == nil {
		t.Fatal("oversized summary accepted")
	}
	b.modelFailure(id, err)
	after, _ := b.snapshot(id)
	if requests != 1 || after.ModelCalls != before.ModelCalls+1 || !reflect.DeepEqual(before.Transcript, after.Transcript) || len(after.ContextCheckpoints) != 0 {
		t.Fatal("failed summary changed context, retried, or lost accounting")
	}
	if after.PendingStatus != "blocked" || after.Run.ModelFailurePhase != "response" || after.Run.ModelFailureCode != "context_summary_too_large" || !after.Run.RetryAt.IsZero() {
		t.Fatal("lost durable nonretryable summary diagnostic")
	}
	if strings.Contains(after.PendingSummary, private) {
		t.Fatal("model output leaked into persisted summary")
	}
}
