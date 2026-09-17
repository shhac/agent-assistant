package app

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/shhac/agent-assistant/internal/core"
	"github.com/shhac/agent-assistant/internal/integrations/worker"
)

func usageWaitRun(kind string, ownerAction bool) worker.Run {
	return worker.Run{
		ID: "held-session", Status: "usage_wait", Summary: "Waiting for worker resources",
		UpdatedAt:           time.Now().UTC(),
		ControlCapabilities: []string{"pause", "resume", "stop"},
		ResourceHold:        &worker.ResourceHold{Kind: kind, Reason: "Waiting for worker resources", OwnerAction: ownerAction, ResetsAt: time.Now().Add(time.Hour).UTC()},
		Usage:               worker.Usage{InputTokens: 900, OutputTokens: 100, TokenBudget: 1000},
	}
}

// A quota wait clears by itself, so the daemon continues the existing session
// once headroom returns. Continuing is not a recovery and must not spend the
// recovery allowance a genuine interruption is entitled to.
func TestQuotaWaitContinuesAutomaticallyWithoutSpendingRecoveries(t *testing.T) {
	ctx := context.Background()
	resumes := 0
	run := usageWaitRun(worker.HoldSubscriptionQuota, false)
	a, _ := runtimeFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/resume") {
			resumes++
			run = worker.Run{ID: run.ID, DispatchKey: run.DispatchKey, Status: "running", Summary: "Continuing the existing assignment", UpdatedAt: time.Now().UTC(), ControlCapabilities: run.ControlCapabilities, Usage: run.Usage}
		}
		writeRun(w, run)
	})
	ag := commission(t, a, "")
	run.DispatchKey = ag.DispatchKey
	a.Core.BeginDispatch(ctx, ag.ID)
	a.Core.MarkDispatched(ctx, ag.ID, run.ID)
	if err := a.tick(ctx, false); err != nil {
		t.Fatal(err)
	}
	snapshot, _ := a.Core.Snapshot(ctx)
	current := snapshot.Agents[0]
	if resumes != 1 {
		t.Fatalf("quota wait did not continue automatically: %d resumes", resumes)
	}
	if current.Recoveries != 0 {
		t.Fatalf("resource wait spent a recovery allowance: %d", current.Recoveries)
	}
	if current.ProviderFailures != 0 || current.ProviderFailureKind != "" {
		t.Fatal("resource wait was recorded as a provider failure", current.ProviderFailureKind)
	}
}

// An owner-decision hold is the owner's move. The daemon must leave it alone
// and keep reporting it, however many times it looks.
func TestBudgetWaitNeedsAnExplicitOwnerResume(t *testing.T) {
	ctx := context.Background()
	posts := 0
	run := usageWaitRun(worker.HoldTokenBudget, true)
	a, _ := runtimeFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			posts++
			t.Error("budget hold was continued without an owner decision")
		}
		writeRun(w, run)
	})
	ag := commission(t, a, "")
	run.DispatchKey = ag.DispatchKey
	a.Core.BeginDispatch(ctx, ag.ID)
	a.Core.MarkDispatched(ctx, ag.ID, run.ID)
	for i := 0; i < 3; i++ {
		if err := a.tick(ctx, false); err != nil {
			t.Fatal(err)
		}
	}
	snapshot, _ := a.Core.Snapshot(ctx)
	current := snapshot.Agents[0]
	if posts != 0 || current.Status != "usage_wait" || current.ResumeKey != "" {
		t.Fatalf("budget hold did not wait for the owner: posts=%d %+v", posts, current)
	}
	if current.UsageInputTokens != 900 || current.UsageOutputTokens != 100 || current.TokenBudget != 1000 {
		t.Fatalf("ledger did not reach the daemon: %+v", current)
	}
	controls, err := a.AgentControls(ctx, current.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !controls.Resume || !strings.Contains(controls.Reason, "Resume") {
		t.Fatalf("owner has no way to continue: %+v", controls)
	}
	inspection, err := a.InspectAgent(ctx, current.ID)
	if err != nil {
		t.Fatal(err)
	}
	if inspection.Resources.WaitingOn != "token_budget" || inspection.Resources.UsedTokens != 1000 || !inspection.Resources.BudgetConfigured {
		t.Fatalf("assistant inspection hides the budget state: %+v", inspection.Resources)
	}
}

// Owner pause and stop, a global pause and a no-dispatch boot each outrank an
// automatic continuation.
func TestResourceContinuationYieldsToOwnerAndDaemonControls(t *testing.T) {
	for _, gate := range []string{"owner_pause", "owner_stop", "global_pause", "no_dispatch"} {
		t.Run(gate, func(t *testing.T) {
			ctx := context.Background()
			posts := 0
			run := usageWaitRun(worker.HoldSubscriptionQuota, false)
			a, _ := runtimeFixture(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "POST" {
					posts++
					t.Errorf("%s did not prevent continuation", gate)
				}
				writeRun(w, run)
			})
			ag := commission(t, a, "")
			run.DispatchKey = ag.DispatchKey
			a.Core.BeginDispatch(ctx, ag.ID)
			a.Core.MarkDispatched(ctx, ag.ID, run.ID)
			noDispatch := false
			switch gate {
			case "owner_pause":
				if _, _, err := a.Core.PrepareOwnerControl(ctx, ag.ID, "pause", "owner-pause"); err != nil {
					t.Fatal(err)
				}
			case "owner_stop":
				if _, _, err := a.Core.PrepareOwnerControl(ctx, ag.ID, "stop", "owner-stop"); err != nil {
					t.Fatal(err)
				}
			case "global_pause":
				if err := a.Core.SetPaused(ctx, true); err != nil {
					t.Fatal(err)
				}
			case "no_dispatch":
				noDispatch = true
			}
			for i := 0; i < 2; i++ {
				if err := a.tick(ctx, noDispatch); err != nil {
					t.Fatal(err)
				}
			}
			snapshot, _ := a.Core.Snapshot(ctx)
			if posts != 0 || snapshot.Agents[0].ResumeKey != "" {
				t.Fatalf("continuation escaped %s: %+v", gate, snapshot.Agents[0])
			}
		})
	}
}

// A resource wait is visible as waiting. Only a hold the owner has to act on
// puts the project in their queue.
func TestAttentionSeparatesOrdinaryWaitFromOwnerDecision(t *testing.T) {
	base := core.Snapshot{
		Projects: []core.Project{{ID: "p1", Title: "Project", Status: "active"}},
		Agents:   []core.Agent{{ID: "a1", ProjectID: "p1", Name: "Worker", Status: "usage_wait", Summary: "Waiting for the account allowance", ResourceHoldKind: worker.HoldSubscriptionQuota, ResourceHoldResetsAt: time.Now().Add(time.Hour)}},
	}
	rows := core.DeriveAttention(base)
	if len(rows) != 1 || rows[0].Execution != "usage_wait" {
		t.Fatalf("resource wait not reported: %+v", rows)
	}
	if rows[0].NextAction != "worker" || rows[0].Recovery != "scheduled" || rows[0].RecoveryAt == nil {
		t.Fatalf("a self-clearing wait asked the owner to act: %+v", rows[0])
	}
	base.Agents[0].ResourceHoldOwnerAction = true
	base.Agents[0].ResourceHoldKind = worker.HoldTokenBudget
	base.Agents[0].ResourceHoldResetsAt = time.Time{}
	rows = core.DeriveAttention(base)
	if rows[0].NextAction != "owner" || rows[0].Recovery != "held" || rows[0].RecoveryAt != nil {
		t.Fatalf("an owner decision was reported as an ordinary wait: %+v", rows[0])
	}
}
