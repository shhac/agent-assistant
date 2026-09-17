package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shhac/agent-assistant/internal/config"
	"github.com/shhac/agent-assistant/internal/core"
	"github.com/shhac/agent-assistant/internal/integrations/worker"
	"github.com/shhac/lib-agent-harness/session"
)

func quotaFixture(used float64) session.QuotaSnapshot {
	observation := session.Observation{Quality: session.Measured, ObservedAt: time.Now()}
	return session.QuotaSnapshot{Observation: observation, Complete: true, Windows: []session.QuotaWindow{{Observation: observation, ID: "codex/primary", Scope: "codex", UsedPercent: &used}}}
}
func TestQuotaDecision(t *testing.T) {
	now := time.Now()
	for _, tc := range []struct {
		name string
		used float64
		held bool
	}{{"below", 89.9, false}, {"exactly", 90, true}, {"over", 110, true}, {"zero", 0, false}} {
		t.Run(tc.name, func(t *testing.T) {
			held, known, _ := quotaDecision(quotaFixture(tc.used), config.Model{Engine: "codex"}, 90, now)
			if held != tc.held || !known {
				t.Fatalf("held=%v known=%v", held, known)
			}
		})
	}
	for _, tc := range []string{"absent", "stale", "invalidated", "nil percentage", "expired reset", "partial", "unrelated"} {
		t.Run(tc, func(t *testing.T) {
			q := quotaFixture(20)
			switch tc {
			case "absent":
				q = session.QuotaSnapshot{}
			case "stale":
				q.ObservedAt = now.Add(-3 * time.Minute)
			case "invalidated":
				q.Invalidated = true
			case "nil percentage":
				q.Windows[0].UsedPercent = nil
			case "expired reset":
				expired := now.Add(-time.Second)
				q.Windows[0].ResetsAt = &expired
			case "partial":
				q.Complete = false
			case "unrelated":
				q.Windows[0].Scope = "code-review"
			}
			held, known, _ := quotaDecision(q, config.Model{Engine: "codex"}, 90, now)
			if held || known {
				t.Fatalf("held=%v known=%v", held, known)
			}
		})
	}
	q := quotaFixture(10)
	high := quotaFixture(95).Windows[0]
	high.ID = "codex/secondary"
	q.Windows = append(q.Windows, high)
	if held, _, _ := quotaDecision(q, config.Model{Engine: "codex"}, 90, now); !held {
		t.Fatal("weekly allowance ignored")
	}
	q.Complete = false
	if held, _, _ := quotaDecision(q, config.Model{Engine: "codex"}, 90, now); !held {
		t.Fatal("known exhausted window lost in partial snapshot")
	}
}
func TestQuotaModelScopes(t *testing.T) {
	for _, tc := range []struct {
		engine, model, scope, id string
		applies                  bool
	}{
		{"codex", "gpt-6-astra", "codex", "codex/primary", true},
		{"codex", "gpt-6-astra", "gpt-6-astra", "model/primary", true},
		{"codex", "gpt-6-astra", "code-review", "review/primary", false},
		{"claude", "claude-opus-5", "five_hour", "five_hour", true},
		{"claude", "claude-opus-5", "seven_day_opus", "seven_day_opus", true},
		{"claude", "claude-opus-5", "seven_day_sonnet", "seven_day_sonnet", false},
		{"claude", "claude-opus-5", "Opus 5", "model:Opus 5", true},
		{"claude", "claude-opus-5", "Sonnet", "model:Sonnet", false},
	} {
		if got := quotaApplies(session.QuotaWindow{ID: tc.id, Scope: tc.scope}, config.Model{Engine: tc.engine, Model: tc.model}); got != tc.applies {
			t.Errorf("%+v got %v", tc, got)
		}
	}
}
func TestUsageCacheUsesEngineBinaryAndHomeNotModel(t *testing.T) {
	var options []session.Options
	meter := workerUsageMeter{inspect: func(_ context.Context, o session.Options) (session.Inspection, error) {
		options = append(options, o)
		return session.Inspection{Quota: quotaFixture(90)}, errors.New("account unavailable but quota succeeded")
	}}
	m := config.Default().WorkerModel
	for i := 0; i < 2; i++ {
		if !meter.read(context.Background(), m).Known() {
			t.Fatal("partial inspection discarded quota")
		}
	}
	m.Model = "different-model"
	meter.read(context.Background(), m)
	m.CodexHome = "/synthetic/another-home"
	meter.read(context.Background(), m)
	m.CodexBin = "another-codex"
	meter.read(context.Background(), m)
	m.Engine = "claude"
	m.ClaudeBin = "synthetic-claude"
	m.ClaudeHome = "/synthetic/claude-home"
	meter.read(context.Background(), m)
	if len(options) != 4 || options[3].Engine != session.Claude || options[3].Binary != m.ClaudeBin || options[3].Home != m.ClaudeHome {
		t.Fatalf("wrong identities: %+v", options)
	}
}

type quotaManaged struct{ client *worker.Client }

func (f quotaManaged) Prepare(context.Context, string, string, config.Model) (*worker.Client, error) {
	return f.client, nil
}
func (f quotaManaged) Client(context.Context, string, string, config.Model) (*worker.Client, error) {
	return f.client, nil
}
func (f quotaManaged) Close() error { return nil }

func usageRuntimeFixture(t *testing.T, handler http.HandlerFunc) (*App, core.Agent) {
	t.Helper()
	a, server := runtimeFixture(t, handler)
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	p, err := a.Core.CreateProject(context.Background(), core.ProjectInput{Title: "Usage protected", AcceptanceCriteria: "Verified output", Directories: []string{dir}})
	if err != nil {
		t.Fatal(err)
	}
	cfg := a.Config()
	cfg.Workers[0] = config.Worker{ID: "fake", Name: "Project worker", Managed: true, ProjectID: p.ID, Workspace: dir, Capabilities: []string{"implement"}}
	if err = a.UpdateConfig(cfg); err != nil {
		t.Fatal(err)
	}
	client, err := worker.New(worker.Config{Endpoint: server.URL, Capabilities: []string{"implement"}})
	if err != nil {
		t.Fatal(err)
	}
	a.managed = quotaManaged{client}
	ag, err := a.Core.Delegate(context.Background(), core.DelegateInput{ProjectID: p.ID, ProfileID: "fake", Role: "worker", Task: "Bounded task", AcceptanceCriteria: "Verified output"})
	if err != nil {
		t.Fatal(err)
	}
	a.workerUsage.inspect = func(context.Context, session.Options) (session.Inspection, error) {
		return session.Inspection{Quota: quotaFixture(90)}, nil
	}
	return a, ag
}
func clearQuotaCache(a *App) {
	a.workerUsage.mu.Lock()
	a.workerUsage.entries = nil
	a.workerUsage.mu.Unlock()
}

func TestQuotaHoldLeavesDispatchQueuedAndAutomaticallyRecovers(t *testing.T) {
	ctx := context.Background()
	starts := 0
	a, ag := usageRuntimeFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/runs" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		starts++
		var request worker.StartRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		writeRun(w, worker.Run{ID: "run", DispatchKey: request.DispatchKey, Status: "running", Summary: "Working", UpdatedAt: time.Now()})
	})
	if err := a.tick(ctx, false); err != nil {
		t.Fatal(err)
	}
	snap, _ := a.Snapshot(ctx)
	if starts != 0 || snap.Agents[0].Status != "queued" || snap.Agents[0].ExternalID != "" || snap.Agents[0].DispatchKey != ag.DispatchKey {
		t.Fatal("hold altered dispatch", snap.Agents)
	}
	visible := false
	for _, status := range snap.Integrations {
		if status.ID == "worker-usage:fake" && status.Status == "paused" && strings.Contains(status.Detail, "90.0%") {
			visible = true
		}
	}
	if !visible {
		t.Fatal("hold missing from dashboard/model state", snap.Integrations)
	}
	a.workerUsage.inspect = func(context.Context, session.Options) (session.Inspection, error) {
		return session.Inspection{Quota: quotaFixture(10)}, nil
	}
	clearQuotaCache(a)
	if err := a.tick(ctx, false); err != nil {
		t.Fatal(err)
	}
	if starts != 1 {
		t.Fatal("queued work did not restart", starts)
	}
}
func TestUsageDoesNotBlockObservationButHoldsResumeAndInstructions(t *testing.T) {
	ctx := context.Background()
	gets, posts := 0, 0
	a, ag := usageRuntimeFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			gets++
			writeRun(w, worker.Run{ID: "saved", Status: "interrupted", Summary: "Stopped", UpdatedAt: time.Now()})
			return
		}
		posts++
		t.Error("new worker work dispatched at quota limit")
	})
	if _, err := a.Core.BeginDispatch(ctx, ag.ID); err != nil {
		t.Fatal(err)
	}
	if err := a.Core.MarkDispatched(ctx, ag.ID, "saved"); err != nil {
		t.Fatal(err)
	}
	if err := a.tick(ctx, false); err != nil {
		t.Fatal(err)
	}
	snap, _ := a.Core.Snapshot(ctx)
	current := snap.Agents[0]
	if gets != 1 || posts != 0 || current.ResumeKey != "" || current.ExternalID != "saved" {
		t.Fatal("resume hold lost session or skipped reconciliation", current)
	}
	if _, err := a.SendAgent(ctx, current, "Continue"); err == nil {
		t.Fatal("manual instruction bypassed gate")
	}
	key := "retryable-instruction"
	err := a.once(ctx, key, func() error { err := a.sendInstruction(ctx, current, key, "Continue"); return err })
	if err == nil {
		t.Fatal("automatic instruction bypassed gate")
	}
	claimed, err := a.Core.ClaimEvent(ctx, key)
	if err != nil || !claimed {
		t.Fatal("quota hold consumed idempotency key", claimed, err)
	}
}
func TestUnavailableAndDisabledWorkerUsagePolicies(t *testing.T) {
	a, _ := usageRuntimeFixture(t, func(http.ResponseWriter, *http.Request) { t.Fatal("unexpected broker call") })
	a.workerUsage.inspect = func(context.Context, session.Options) (session.Inspection, error) {
		return session.Inspection{}, errors.New("unavailable")
	}
	ctx := context.Background()
	if err := a.workerUsageAllowed(ctx, "fake"); err != nil {
		t.Fatal("default unavailable should allow", err)
	}
	cfg := a.Config()
	cfg.Limits.WorkerUsage.OnUnavailable = "pause"
	if err := a.UpdateConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if err := a.workerUsageAllowed(ctx, "fake"); err == nil {
		t.Fatal("pause policy ignored")
	}
	cfg.Limits.WorkerUsage.CodexMaxUsedPercent = 0
	if err := a.UpdateConfig(cfg); err != nil {
		t.Fatal(err)
	}
	a.workerUsage.inspect = func(context.Context, session.Options) (session.Inspection, error) {
		t.Fatal("disabled limit inspected CLI")
		return session.Inspection{}, nil
	}
	clearQuotaCache(a)
	if err := a.workerUsageAllowed(ctx, "fake"); err != nil {
		t.Fatal("disabled limit blocked", err)
	}
	external, _ := runtimeFixture(t, func(http.ResponseWriter, *http.Request) { t.Fatal("unexpected broker call") })
	external.workerUsage.inspect = a.workerUsage.inspect
	cfg = external.Config()
	cfg.Limits.WorkerUsage.OnUnavailable = "pause"
	if err := external.UpdateConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if err := external.workerUsageAllowed(ctx, "fake"); err == nil {
		t.Fatal("external broker incorrectly attributed to local account")
	}
}

func TestWorkerUsageUsesProfileEngineAndThreshold(t *testing.T) {
	a, _ := usageRuntimeFixture(t, func(http.ResponseWriter, *http.Request) { t.Fatal("unexpected broker call") })
	cfg := a.Config()
	override := cfg.WorkerModel
	override.Engine = "claude"
	override.Model = "claude-opus-5"
	override.ClaudeHome = filepath.Join(t.TempDir(), "different-login")
	cfg.Workers[0].ModelProfile = &override
	cfg.Limits.WorkerUsage.ClaudeMaxUsedPercent = 75
	if err := a.UpdateConfig(cfg); err != nil {
		t.Fatal(err)
	}
	a.workerUsage.inspect = func(_ context.Context, o session.Options) (session.Inspection, error) {
		if o.Engine != session.Claude || o.Home != override.ClaudeHome {
			t.Fatalf("wrong worker login: %+v", o)
		}
		q := quotaFixture(75)
		q.Windows[0].ID = "seven_day"
		q.Windows[0].Scope = "seven_day"
		return session.Inspection{Quota: q}, nil
	}
	if err := a.workerUsageAllowed(context.Background(), "fake"); err == nil {
		t.Fatal("Claude-specific limit ignored")
	}
}

func TestWorkerUsageRefreshDoesNotRetainFailedTelemetry(t *testing.T) {
	calls := 0
	meter := workerUsageMeter{inspect: func(context.Context, session.Options) (session.Inspection, error) {
		calls++
		if calls == 1 {
			return session.Inspection{Quota: quotaFixture(5)}, nil
		}
		return session.Inspection{}, errors.New("failed refresh")
	}}
	model := config.Default().WorkerModel
	if !meter.read(context.Background(), model).Known() {
		t.Fatal("initial quota absent")
	}
	for key, entry := range meter.entries {
		entry.fetched = time.Now().Add(-2 * quotaCacheAge)
		meter.entries[key] = entry
	}
	if meter.read(context.Background(), model).Known() || calls != 2 {
		t.Fatal("failed refresh presented stale quota as current")
	}
}

func TestUsageHeldDelegationRetriesWithoutOrphaningOrDuplicatingChild(t *testing.T) {
	ctx := context.Background()
	messages := 0
	run := worker.Run{ID: "manager", Status: "waiting", Summary: "Waiting for child", UpdatedAt: time.Now(), Delegation: &worker.DelegationRequest{RequestID: "stable-child-request", WorkerProfile: "fake", Role: "worker", Task: "Bounded implementation", AcceptanceCriteria: "Tests pass", Capabilities: []string{"implement"}}}
	a, _ := runtimeFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/messages") {
			messages++
		}
		writeRun(w, run)
	})
	parent := commission(t, a, "")
	if _, err := a.Core.BeginDispatch(ctx, parent.ID); err != nil {
		t.Fatal(err)
	}
	if err := a.Core.MarkDispatched(ctx, parent.ID, run.ID); err != nil {
		t.Fatal(err)
	}
	cfg := a.Config()
	cfg.Limits.WorkerUsage.OnUnavailable = "pause"
	if err := a.UpdateConfig(cfg); err != nil {
		t.Fatal(err)
	}
	// An external manager has unavailable local telemetry. Pausing it must happen
	// before child creation, exactly like a measured exhausted subscription.
	if err := a.tick(ctx, false); err != nil {
		t.Fatal("quota hold reported as supervision failure", err)
	}
	snap, _ := a.Core.Snapshot(ctx)
	if len(snap.Agents) != 1 || messages != 0 {
		t.Fatal("delegation hold created child before admission")
	}
	cfg.Limits.WorkerUsage.OnUnavailable = "allow"
	if err := a.UpdateConfig(cfg); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		snap, _ = a.Core.Snapshot(ctx)
		current, _ := findAgent(snap, parent.ID)
		if err := a.superviseAgent(ctx, current, false); err != nil {
			t.Fatal(err)
		}
	}
	snap, _ = a.Core.Snapshot(ctx)
	if len(snap.Agents) != 2 || messages != 1 {
		t.Fatalf("delegation failed recovery or duplicated: agents=%d messages=%d", len(snap.Agents), messages)
	}
}
