package workerbroker

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shhac/agent-assistant/internal/engine"
	"github.com/shhac/agent-assistant/internal/integrations/worker"
)

// toolProvider answers every request with one tool call, plus optional usage,
// so a test can drive as many turns as it likes without a real model.
func toolProvider(t *testing.T, calls *atomic.Int64, input, output int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		call := toolCall{ID: "call", Type: "function"}
		call.Function.Name = "run_command"
		call.Function.Arguments = `{"command":"synthetic-check"}`
		body := map[string]any{"choices": []any{map[string]any{"message": modelMessage{Role: "assistant", ToolCalls: []toolCall{call}}, "finish_reason": "tool_calls"}}}
		if input > 0 || output > 0 {
			body["usage"] = map[string]int{"prompt_tokens": input, "completion_tokens": output, "total_tokens": input + output}
		}
		_ = json.NewEncoder(w).Encode(body)
	}))
}

func startedRun(t *testing.T, b *Broker) string {
	t.Helper()
	response := request(t, b, "/runs", "dispatch-one", startRequest())
	if response.Code != 201 {
		t.Fatal(response.Code, response.Body.String())
	}
	var run worker.Run
	if err := json.Unmarshal(response.Body.Bytes(), &run); err != nil {
		t.Fatal(err)
	}
	return run.ID
}

func awaitRun(t *testing.T, b *Broker, id string, accept func(storedRun) bool) storedRun {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		r, err := b.snapshot(id)
		if err == nil && accept(r) {
			return r
		}
		time.Sleep(5 * time.Millisecond)
	}
	r, _ := b.snapshot(id)
	t.Fatalf("worker never reached the expected state: status=%q pending=%q calls=%d", r.Run.Status, r.PendingStatus, r.ModelCalls)
	return storedRun{}
}

// The former ceiling was 16 cumulative calls, and it counted successful
// thinking. Useful work must be able to run past it.
func TestWorkerRunsPastTheFormerCallCeiling(t *testing.T) {
	var calls atomic.Int64
	provider := toolProvider(t, &calls, 10, 5)
	defer provider.Close()
	b, _ := newFixture(t, provider.URL, &fakeDocker{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- b.Run(ctx) }()
	defer func() { cancel(); <-done; _ = b.Close() }()
	id := startedRun(t, b)
	r := awaitRun(t, b, id, func(r storedRun) bool { return r.ModelCalls >= 40 })
	if r.Run.Status != "running" {
		t.Fatalf("long assignment stopped at %d calls: %s", r.ModelCalls, r.Run.Summary)
	}
	// One call may be reserved and not yet settled when the snapshot is taken,
	// which is exactly the reservation that protects a crash from losing it.
	settled := r.UsageInputTokens / 10
	if r.UsageOutputTokens != settled*5 || settled < int64(r.ModelCalls)-1 || settled > int64(r.ModelCalls) {
		t.Fatalf("usage ledger disagrees with calls: %d in / %d out over %d calls", r.UsageInputTokens, r.UsageOutputTokens, r.ModelCalls)
	}
	if r.UsageUnknownCalls != 0 {
		t.Fatal("measured calls recorded as unknown", r.UsageUnknownCalls)
	}
}

// An exhausted saved assignment from before this change must continue on an
// ordinary resume, keeping its counter and its transcript.
func TestExhaustedSavedAssignmentResumesWithoutResettingHistory(t *testing.T) {
	var calls atomic.Int64
	provider := toolProvider(t, &calls, 4, 2)
	defer provider.Close()
	b, _ := newFixture(t, provider.URL, &fakeDocker{})
	// Establish the saved state of an assignment stopped by the old ceiling
	// before the runtime can dispatch it, so the resume is what starts work.
	id := startedRun(t, b)
	saved := []modelMessage{{Role: "user", Content: "Owner direction from the original assignment"}}
	if err := b.update(id, func(r *storedRun) error {
		r.Run.Status = "blocked"
		r.Run.Summary = "Worker exhausted its cumulative model-call allowance"
		r.ModelCalls = 16
		r.Transcript = append([]modelMessage(nil), saved...)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- b.Run(ctx) }()
	defer func() { cancel(); <-done; _ = b.Close() }()
	if code := request(t, b, "/runs/"+id+"/resume", "resume-one", map[string]string{"instruction": "Continue"}).Code; code != 200 {
		t.Fatal("exhausted assignment refused resume", code)
	}
	r := awaitRun(t, b, id, func(r storedRun) bool { return r.ModelCalls > 16 })
	if r.ModelCalls <= 16 {
		t.Fatal("history was reset instead of continued", r.ModelCalls)
	}
	if len(r.Transcript) == 0 || r.Transcript[0].Content != saved[0].Content {
		t.Fatal("resume lost the saved transcript")
	}
}

// Every request passes the injected gate, including the summary a compaction
// needs, and a refusal spends nothing.
func TestAdmissionCoversEveryRequestAndRefusalSpendsNothing(t *testing.T) {
	var calls atomic.Int64
	provider := toolProvider(t, &calls, 1, 1)
	defer provider.Close()
	b, _ := newFixture(t, provider.URL, &fakeDocker{})
	defer b.Close()
	var admitted atomic.Int64
	b.cfg.Admit = func(context.Context) error {
		admitted.Add(1)
		return &worker.HoldError{Hold: worker.ResourceHold{Kind: worker.HoldSubscriptionQuota, Reason: "Account allowance consumed; resets soon", ResetsAt: time.Now().Add(time.Hour).UTC()}}
	}
	id := contextRun(t, b)
	before, _ := b.snapshot(id)
	if _, err := b.completeForRun(context.Background(), id, nil); err == nil {
		t.Fatal("held request proceeded")
	}
	after, _ := b.snapshot(id)
	if admitted.Load() != 1 || calls.Load() != 0 {
		t.Fatalf("gate ran %d times and provider saw %d requests", admitted.Load(), calls.Load())
	}
	if after.ModelCalls != before.ModelCalls || after.PendingUsage != nil || after.UsageUnknownCalls != 0 {
		t.Fatal("refused request was accounted for")
	}
	if !reflect.DeepEqual(after.Transcript, before.Transcript) {
		t.Fatal("refused request altered the conversation")
	}
	if after.Run.ResourceHold == nil || after.Run.ResourceHold.OwnerAction || after.Run.ResourceHold.ResetsAt.IsZero() {
		t.Fatalf("quota hold did not record a self-clearing wait: %+v", after.Run.ResourceHold)
	}
	if after.Run.ProviderFailures != 0 || after.Run.ProviderFailureKind != "" || !after.Run.RetryAt.IsZero() {
		t.Fatal("hold masqueraded as a provider failure")
	}
}

// A held run finishes its attempt as a wait, keeping the owner's queued message
// for the continuation rather than answering it with a refusal.
func TestResourceHoldPreservesQueuedOwnerDirection(t *testing.T) {
	var calls atomic.Int64
	provider := toolProvider(t, &calls, 1, 1)
	defer provider.Close()
	b, _ := newFixture(t, provider.URL, &fakeDocker{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- b.Run(ctx) }()
	defer func() { cancel(); <-done; _ = b.Close() }()
	var held atomic.Bool
	held.Store(true)
	b.cfg.Admit = func(context.Context) error {
		if held.Load() {
			return &worker.HoldError{Hold: worker.ResourceHold{Kind: worker.HoldSubscriptionQuota, Reason: "Account allowance consumed"}}
		}
		return nil
	}
	id := startedRun(t, b)
	r := awaitRun(t, b, id, func(r storedRun) bool { return r.Run.Status == "usage_wait" })
	if r.Run.ResourceHold == nil || r.Run.Summary == "" {
		t.Fatal("hold published without a reason")
	}
	if code := request(t, b, "/runs/"+id+"/messages", "direction-one", map[string]string{"message": "Prefer the smaller change"}).Code; code != 409 {
		t.Fatal("a held worker accepted a message instead of requiring resume", code)
	}
	held.Store(false)
	if code := request(t, b, "/runs/"+id+"/resume", "resume-one", map[string]string{"instruction": "Prefer the smaller change"}).Code; code != 200 {
		t.Fatal("resume refused after the hold cleared", code)
	}
	r = awaitRun(t, b, id, func(r storedRun) bool { return r.ModelCalls > 0 })
	if r.Run.ResourceHold != nil {
		t.Fatal("stale hold survived a continued run")
	}
	found := false
	for _, m := range r.Transcript {
		if m.Role == "user" && m.Content == "Prefer the smaller change" {
			found = true
		}
	}
	if !found {
		t.Fatal("owner direction was lost across the hold")
	}
}

// A configured token budget stops the next request, and raising it lets the
// same assignment continue with its saved context.
func TestTokenBudgetStopsAndExtendsWithoutLosingContext(t *testing.T) {
	var calls atomic.Int64
	provider := toolProvider(t, &calls, 100, 50)
	defer provider.Close()
	b, _ := newFixture(t, provider.URL, &fakeDocker{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- b.Run(ctx) }()
	defer func() { cancel(); <-done; _ = b.Close() }()
	var budget atomic.Int64
	budget.Store(1000)
	b.cfg.TokenBudget = budget.Load
	id := startedRun(t, b)
	r := awaitRun(t, b, id, func(r storedRun) bool { return r.Run.Status == "usage_wait" })
	if r.Run.ResourceHold.Kind != worker.HoldTokenBudget || !r.Run.ResourceHold.OwnerAction {
		t.Fatalf("budget hold is not an owner decision: %+v", r.Run.ResourceHold)
	}
	used := r.UsageInputTokens + r.UsageOutputTokens
	if used < 1000 {
		t.Fatal("stopped before the budget was reached", used)
	}
	transcript := len(r.Transcript)
	if transcript == 0 {
		t.Fatal("budget hold discarded the conversation")
	}
	budget.Store(100000)
	if code := request(t, b, "/runs/"+id+"/resume", "resume-one", map[string]string{"instruction": "Continue"}).Code; code != 200 {
		t.Fatal("raised budget did not allow an explicit resume", code)
	}
	after := awaitRun(t, b, id, func(r storedRun) bool { return r.UsageInputTokens+r.UsageOutputTokens > used })
	if len(after.Transcript) < transcript {
		t.Fatal("continuation lost saved context")
	}
}

// Missing usage is not free work. With a budget configured, an assignment whose
// consumption cannot be established waits for an owner decision, and raising
// the budget does not measure what the provider never reported.
func TestUnknownUsageHoldsUntilTheOwnerDecides(t *testing.T) {
	var calls atomic.Int64
	provider := toolProvider(t, &calls, 0, 0)
	defer provider.Close()
	b, _ := newFixture(t, provider.URL, &fakeDocker{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- b.Run(ctx) }()
	defer func() { cancel(); <-done; _ = b.Close() }()
	var budget atomic.Int64
	budget.Store(100000)
	b.cfg.TokenBudget = budget.Load
	id := startedRun(t, b)
	r := awaitRun(t, b, id, func(r storedRun) bool { return r.Run.Status == "usage_wait" })
	if r.Run.ResourceHold.Kind != worker.HoldUsageUnknown || r.UsageUnknownCalls == 0 {
		t.Fatalf("unmeasured call was treated as free: %+v unknown=%d", r.Run.ResourceHold, r.UsageUnknownCalls)
	}
	if r.UsageInputTokens != 0 || r.UsageOutputTokens != 0 {
		t.Fatal("unknown usage was invented as a number")
	}
	budget.Store(10_000_000)
	if _, err := b.completeForRun(context.Background(), id, nil); err == nil {
		t.Fatal("raising the budget erased the uncertainty")
	}
	budget.Store(0)
	held, _ := b.snapshot(id)
	if hold := budgetHold(held, 0); hold != nil {
		t.Fatal("disabling the budget did not release the hold")
	}
}

// Nothing is spent, and nothing new becomes unknown, when a preflight fails
// before the request leaves.
func TestFailedPreflightAddsNoUnknownConsumption(t *testing.T) {
	b, _ := newFixture(t, "https://model.test", &fakeDocker{})
	defer b.Close()
	b.cfg.Engine = "codex"
	b.cfg.CodexBin = "/nonexistent/synthetic-codex"
	b.cfg.TokenBudget = func() int64 { return 100000 }
	id := contextRun(t, b)
	before, _ := b.snapshot(id)
	if _, err := b.completeForRun(context.Background(), id, nil); err == nil {
		t.Fatal("missing CLI produced a completion")
	}
	after, _ := b.snapshot(id)
	if after.UsageUnknownCalls != before.UsageUnknownCalls || after.ModelCalls != before.ModelCalls || after.PendingUsage != nil {
		t.Fatalf("preflight failure was accounted as consumption: unknown=%d calls=%d pending=%+v", after.UsageUnknownCalls, after.ModelCalls, after.PendingUsage)
	}
}

// A reservation that outlives its process is consumption that may have
// happened. Restart must convert it into uncertainty, and history recorded
// before the ledger existed must be converted exactly once.
func TestRestartConvertsStaleReservationsAndPreLedgerHistory(t *testing.T) {
	for _, tc := range []struct {
		name    string
		run     storedRun
		unknown int
	}{
		{"stale reservation", storedRun{UsageLedger: true, ModelCalls: 3, UsageInputTokens: 30, PendingUsage: &pendingUsage{RequestID: "abandoned", Stage: stageTurn}}, 1},
		{"pre-ledger history", storedRun{ModelCalls: 16}, 16},
		{"pre-ledger with reservation", storedRun{ModelCalls: 2, PendingUsage: &pendingUsage{RequestID: "abandoned"}}, 3},
		{"already reconciled", storedRun{UsageLedger: true, ModelCalls: 5, UsageInputTokens: 50}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			run := tc.run
			reconcileUsage(&run, 1000)
			if run.UsageUnknownCalls != tc.unknown || run.PendingUsage != nil || !run.UsageLedger {
				t.Fatalf("unknown=%d pending=%+v ledger=%v", run.UsageUnknownCalls, run.PendingUsage, run.UsageLedger)
			}
			if run.Run.Usage.TokenBudget != 1000 || run.Run.Usage.UnknownCalls != tc.unknown {
				t.Fatalf("published ledger disagrees: %+v", run.Run.Usage)
			}
			// Reconciling twice must not multiply the uncertainty.
			reconcileUsage(&run, 1000)
			if run.UsageUnknownCalls != tc.unknown {
				t.Fatal("second reconciliation double-counted", run.UsageUnknownCalls)
			}
		})
	}
}

// A settlement belongs to exactly one reservation. Anything else is a late or
// repeated report and must change nothing.
func TestUsageSettlesOncePerReservation(t *testing.T) {
	b, _ := newFixture(t, "https://model.test", &fakeDocker{})
	defer b.Close()
	id := contextRun(t, b)
	request := ""
	if err := b.reserveWorkerModelCall(context.Background(), id, stageTurn, &request); err != nil {
		t.Fatal(err)
	}
	if request == "" {
		t.Fatal("admission produced no reservation")
	}
	b.settleUsage(id, request, engineUsage(11, 7))
	b.settleUsage(id, request, engineUsage(11, 7))
	b.settleUsage(id, "a-different-request", engineUsage(500, 500))
	r, _ := b.snapshot(id)
	if r.UsageInputTokens != 11 || r.UsageOutputTokens != 7 || r.UsageUnknownCalls != 0 || r.PendingUsage != nil {
		t.Fatalf("ledger after repeated settlement: %+v", r.Run.Usage)
	}
}

// Owner pause and stop outrank a resource hold: an owner decision already made
// must not be overwritten by a wait discovered afterwards.
func TestOwnerControlsOutrankResourceHolds(t *testing.T) {
	for _, control := range []string{"paused", "cancelled"} {
		t.Run(control, func(t *testing.T) {
			b, _ := newFixture(t, "https://model.test", &fakeDocker{})
			defer b.Close()
			id := contextRun(t, b)
			if err := b.update(id, func(r *storedRun) error { r.PendingStatus = control; return nil }); err != nil {
				t.Fatal(err)
			}
			b.holdOnResources(id, worker.ResourceHold{Kind: worker.HoldSubscriptionQuota, Reason: "Account allowance consumed"})
			r, _ := b.snapshot(id)
			if r.PendingStatus != control || r.Run.ResourceHold != nil {
				t.Fatalf("resource hold overwrote an owner control: %q", r.PendingStatus)
			}
		})
	}
}

func engineUsage(input, output int) engine.Usage {
	return engine.Usage{InputTokens: input, OutputTokens: output, TotalTokens: input + output, Known: true}
}

// A worker running a long build-and-test loop must not be stopped because it
// has run a certain number of commands. Per-command time and output bounds are
// what contain a command; a lifetime count is not a resource limit.
func TestCommandsAreNotCappedByCount(t *testing.T) {
	b, _ := newFixture(t, "https://model.test", &fakeDocker{})
	defer b.Close()
	id := contextRun(t, b)
	r, _ := b.snapshot(id)
	r.Container = "agent-assistant-" + id
	for i := 0; i < 128; i++ {
		if err := b.update(id, func(run *storedRun) error {
			run.Commands = append(run.Commands, commandRecord{Command: "synthetic", Success: true})
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	var call toolCall
	call.ID, call.Type, call.Function.Name = "command-129", "function", "run_command"
	call.Function.Arguments = `{"command":"go test ./..."}`
	value, finished, err := b.tool(context.Background(), id, r, call)
	if err != nil || finished {
		t.Fatalf("command 129 was refused: %v", err)
	}
	record, ok := value.(commandRecord)
	if !ok || record.Command != "go test ./..." {
		t.Fatalf("command 129 did not run: %+v", value)
	}
	after, _ := b.snapshot(id)
	if len(after.Commands) != 129 {
		t.Fatalf("command 129 was not recorded: %d", len(after.Commands))
	}
}

// A settlement that did not persist leaves consumption unrecorded. Continuing
// to act on that reply would spend against a ledger the broker cannot vouch
// for, so the failure has to reach the caller instead of the proposals.
func TestUnpersistedSettlementStopsTheReplyAndHoldsTheNextRequest(t *testing.T) {
	var calls atomic.Int64
	b, _ := newFixture(t, "https://model.test", &fakeDocker{})
	defer b.Close()
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		// The reservation has persisted by now; make the settlement that follows
		// unable to write, which is what a durability failure looks like here.
		if err := os.Chmod(b.cfg.StateDir, 0500); err != nil {
			t.Error(err)
		}
		call := toolCall{ID: "call", Type: "function"}
		call.Function.Name = "finish"
		call.Function.Arguments = `{"summary":"Claiming completion"}`
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": modelMessage{Role: "assistant", ToolCalls: []toolCall{call}}, "finish_reason": "tool_calls"}}})
	}))
	defer provider.Close()
	defer os.Chmod(b.cfg.StateDir, 0700)
	b.cfg.ModelEndpoint = provider.URL
	id := contextRun(t, b)

	reply, err := b.completeForRun(context.Background(), id, nil)
	if err == nil {
		t.Fatal("an unrecorded settlement returned a usable reply")
	}
	if len(reply.ToolCalls) != 0 {
		t.Fatalf("proposals escaped an unrecorded settlement: %+v", reply.ToolCalls)
	}
	if err := os.Chmod(b.cfg.StateDir, 0700); err != nil {
		t.Fatal(err)
	}
	held, _ := b.snapshot(id)
	if held.PendingUsage == nil {
		t.Fatal("the unsettled reservation was discarded rather than retained")
	}

	// The next request is refused while that reservation is open, budget or not.
	request := ""
	err = b.reserveWorkerModelCall(context.Background(), id, stageTurn, &request)
	if !errors.Is(err, worker.ErrResourceHold) || request != "" {
		t.Fatalf("another request was authorized over an open reservation: %v", err)
	}
	after, _ := b.snapshot(id)
	if after.Run.ResourceHold == nil || after.Run.ResourceHold.Kind != worker.HoldUsageUnknown || !after.Run.ResourceHold.OwnerAction {
		t.Fatalf("an unresolved reservation was not surfaced for the owner: %+v", after.Run.ResourceHold)
	}
	if calls.Load() != 1 {
		t.Fatalf("the provider was contacted again: %d", calls.Load())
	}
}

// A figure that is negative, incomplete or too large to add is not a
// measurement. Recording it as a number would make it indistinguishable from a
// real one later.
func TestUnusableUsageIsRecordedAsUnknownNotRepaired(t *testing.T) {
	for _, tc := range []struct {
		name  string
		usage engine.Usage
		seed  int64
	}{
		{"unknown", engine.Usage{InputTokens: 5, OutputTokens: 5}, 0},
		{"negative input", engine.Usage{InputTokens: -5, OutputTokens: 5, Known: true}, 0},
		{"negative output", engine.Usage{InputTokens: 5, OutputTokens: -5, Known: true}, 0},
		{"overflowing total", engine.Usage{InputTokens: math.MaxInt64 / 2, OutputTokens: 1, Known: true}, math.MaxInt64 - 10},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, _ := newFixture(t, "https://model.test", &fakeDocker{})
			defer b.Close()
			id := contextRun(t, b)
			if err := b.update(id, func(r *storedRun) error { r.UsageInputTokens = tc.seed; return nil }); err != nil {
				t.Fatal(err)
			}
			request := ""
			if err := b.reserveWorkerModelCall(context.Background(), id, stageTurn, &request); err != nil {
				t.Fatal(err)
			}
			if err := b.settleUsage(id, request, tc.usage); err != nil {
				t.Fatal(err)
			}
			r, _ := b.snapshot(id)
			if r.UsageUnknownCalls != 1 {
				t.Fatalf("unusable usage was accepted: unknown=%d", r.UsageUnknownCalls)
			}
			if r.UsageInputTokens != tc.seed || r.UsageOutputTokens != 0 {
				t.Fatalf("unusable usage changed the total: %d/%d", r.UsageInputTokens, r.UsageOutputTokens)
			}
			if r.PendingUsage != nil {
				t.Fatal("the reservation was left open after an unknown settlement")
			}
		})
	}
}
