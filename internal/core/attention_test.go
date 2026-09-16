package core

import (
	"testing"
	"time"
)

func attentionFor(t *testing.T, v Snapshot, projectID string) ProjectAttention {
	t.Helper()
	for _, a := range DeriveAttention(v) {
		if a.ProjectID == projectID {
			return a
		}
	}
	t.Fatalf("no attention derived for %q", projectID)
	return ProjectAttention{}
}

func attentionSnapshot(agents []Agent, items []WorkItem) Snapshot {
	return Snapshot{
		Projects:  []Project{{ID: "p1", Title: "Garden planner", Status: "active"}},
		Agents:    agents,
		WorkItems: items,
	}
}

// The condition the owner reported: nothing is waiting on their judgment, yet
// a worker has stopped. Health must not be inferred from the decision queue.
func TestAttentionReportsBlockedWorkWithNoOpenDecisions(t *testing.T) {
	v := attentionSnapshot(
		[]Agent{{ID: "a1", ProjectID: "p1", WorkItemID: "w1", Name: "Suggestions worker", Status: "blocked", Summary: "The attempt stopped without a classified provider error.", ProviderFailureKind: "unknown"}},
		[]WorkItem{{ID: "w1", ProjectID: "p1", Title: "Suggestions", Status: "blocked", StatusReason: "Execution is blocked"}},
	)
	got := attentionFor(t, v, "p1")
	if got.Execution != "blocked" {
		t.Fatalf("execution = %q, want blocked", got.Execution)
	}
	if got.OpenDecisions != 0 {
		t.Fatalf("open decisions = %d, want 0", got.OpenDecisions)
	}
	if got.NextAction != "owner" {
		t.Fatalf("next action = %q, want owner", got.NextAction)
	}
	if got.Recovery != "held" {
		t.Fatalf("recovery = %q, want held", got.Recovery)
	}
	if got.AgentID != "a1" || got.WorkItemID != "w1" {
		t.Fatalf("attention does not point at the affected work: %+v", got)
	}
}

// A work item reports its liveliest attempt, so a project whose outcome reads
// "active" can still hold a stopped assignment. That assignment is the reason
// the owner opened the dashboard.
func TestAttentionSeesBlockedAgentBesideRunningSibling(t *testing.T) {
	v := attentionSnapshot(
		[]Agent{
			{ID: "a1", ProjectID: "p1", WorkItemID: "w1", Name: "Runner", Status: "running", Summary: "Working"},
			{ID: "a2", ProjectID: "p1", WorkItemID: "w1", Name: "Suggestions worker", Status: "blocked", Summary: "Execution is blocked"},
		},
		[]WorkItem{{ID: "w1", ProjectID: "p1", Status: "active", StatusReason: "Work is running"}},
	)
	got := attentionFor(t, v, "p1")
	if got.Execution != "blocked" {
		t.Fatalf("a running sibling masked the blocked worker: execution = %q", got.Execution)
	}
	if got.AgentName != "Suggestions worker" {
		t.Fatalf("attention names the wrong worker: %q", got.AgentName)
	}
}

// Reason text is copied from what produced the state so the dashboard and the
// work item cannot describe the same condition differently.
func TestAttentionQuotesRecordedReasonVerbatim(t *testing.T) {
	v := attentionSnapshot(nil, []WorkItem{{ID: "w1", ProjectID: "p1", Status: "blocked", StatusReason: "Waiting for a decision: Which review depth?"}})
	if got := attentionFor(t, v, "p1"); got.Reason != "Waiting for a decision: Which review depth?" {
		t.Fatalf("reason = %q, want the recorded status reason", got.Reason)
	}
}

func TestAttentionReportsScheduledRecoveryAsTheWorkersTurn(t *testing.T) {
	retry := time.Date(2026, 9, 16, 17, 5, 0, 0, time.UTC)
	v := attentionSnapshot(
		[]Agent{{ID: "a1", ProjectID: "p1", Name: "Worker", Status: "retry_wait", Summary: "Model provider temporarily unavailable", RetryAt: retry}},
		nil,
	)
	got := attentionFor(t, v, "p1")
	if got.NextAction != "worker" || got.Recovery != "scheduled" {
		t.Fatalf("scheduled retry misattributed: %+v", got)
	}
	if got.RecoveryAt == nil || !got.RecoveryAt.Equal(retry) {
		t.Fatalf("recovery time = %v, want %v", got.RecoveryAt, retry)
	}
}

// Closing a question is not permission to execute, and an uninspected
// interruption is the owner's turn whatever the workers are doing.
func TestAttentionCountsOwnerQueuesIndependentlyOfExecution(t *testing.T) {
	v := attentionSnapshot(
		[]Agent{{ID: "a1", ProjectID: "p1", Name: "Worker", Status: "running", Summary: "Working"}},
		nil,
	)
	v.Decisions = []Decision{
		{ID: "d1", ProjectID: "p1", Status: "open"},
		{ID: "d2", ProjectID: "p1", Status: "dismissed"},
	}
	v.PendingOperations = []PendingOperation{{ID: "op1", ProjectID: "p1"}}
	got := attentionFor(t, v, "p1")
	if got.OpenDecisions != 1 {
		t.Fatalf("open decisions = %d, want 1 (a dismissal is not open)", got.OpenDecisions)
	}
	if got.PendingOperations != 1 {
		t.Fatalf("pending operations = %d, want 1", got.PendingOperations)
	}
	if got.NextAction != "owner" {
		t.Fatalf("an open question is the owner's turn, got %q", got.NextAction)
	}
}

func TestAttentionOmitsFinishedAndUnstartedProjects(t *testing.T) {
	v := Snapshot{
		Projects: []Project{
			{ID: "p1", Status: "completed"},
			{ID: "p2", Status: "active"},
		},
		WorkItems: []WorkItem{
			{ID: "w1", ProjectID: "p1", Status: "blocked", StatusReason: "stale"},
			{ID: "w2", ProjectID: "p2", Status: "accepted"},
			{ID: "w3", ProjectID: "p2", Status: "legacy_completed"},
		},
	}
	for _, a := range DeriveAttention(v) {
		if a.ProjectID == "p1" {
			t.Fatal("a completed project must not report attention")
		}
		if a.ProjectID == "p2" {
			t.Fatalf("closed work must not report attention: %+v", a)
		}
	}
}

// Ordering decides what the owner sees first without scrolling.
func TestAttentionOrdersMostHeldUpFirst(t *testing.T) {
	v := Snapshot{
		Projects: []Project{
			{ID: "calm", Status: "active"},
			{ID: "stuck", Status: "active"},
			{ID: "reviewing", Status: "active"},
		},
		WorkItems: []WorkItem{
			{ID: "w1", ProjectID: "calm", Status: "active", StatusReason: "Work is running"},
			{ID: "w2", ProjectID: "stuck", Status: "blocked", StatusReason: "Execution is blocked"},
			{ID: "w3", ProjectID: "reviewing", Status: "review", StatusReason: "Execution finished; acceptance review is required"},
		},
	}
	got := DeriveAttention(v)
	if len(got) != 3 {
		t.Fatalf("expected every open project, got %d", len(got))
	}
	if got[0].ProjectID != "stuck" || got[1].ProjectID != "reviewing" || got[2].ProjectID != "calm" {
		t.Fatalf("attention order = %q, %q, %q", got[0].ProjectID, got[1].ProjectID, got[2].ProjectID)
	}
}

func TestAttentionIsDeterministic(t *testing.T) {
	v := attentionSnapshot(
		[]Agent{
			{ID: "a1", ProjectID: "p1", Name: "One", Status: "waiting", Summary: "waiting"},
			{ID: "a2", ProjectID: "p1", Name: "Two", Status: "waiting", Summary: "waiting"},
		},
		nil,
	)
	first := DeriveAttention(v)
	for i := 0; i < 5; i++ {
		next := DeriveAttention(v)
		if len(next) != len(first) || next[0] != first[0] {
			t.Fatal("derived attention changed between identical snapshots")
		}
	}
}

// The demonstration workspace has to exercise the states the dashboard claims
// to report, including the case where nothing waits on the owner's judgment
// and a worker has still stopped. Otherwise the health, failure and queue
// views are unreachable in preview mode.
func TestDemoWorkspaceCoversTheStatesTheDashboardReports(t *testing.T) {
	s, _ := fixture(t)
	if err := s.SeedDemo(testContext); err != nil {
		t.Fatal(err)
	}
	v, err := s.Snapshot(testContext)
	if err != nil {
		t.Fatal(err)
	}
	statuses := map[string]bool{}
	for _, w := range v.WorkItems {
		statuses[w.Status] = true
	}
	for _, want := range []string{"active", "blocked", "review", "queued"} {
		if !statuses[want] {
			t.Fatalf("demo workspace has no %q outcome; states present: %v", want, statuses)
		}
	}

	attention := DeriveAttention(v)
	var stopped *ProjectAttention
	for i := range attention {
		if attention[i].Execution == "blocked" {
			stopped = &attention[i]
		}
	}
	if stopped == nil {
		t.Fatal("demo workspace reports no blocked work")
	}
	if stopped.OpenDecisions != 0 {
		t.Fatalf("the blocked demo project should have no open decision, got %d", stopped.OpenDecisions)
	}
	if stopped.NextAction != "owner" {
		t.Fatalf("blocked demo work is not attributed to the owner: %+v", stopped)
	}
	for _, a := range v.Agents {
		if a.ID == "demo-blocked-worker" && a.ModelFailureEvidence == "" {
			t.Fatal("the blocked demo worker records no failure evidence to display")
		}
	}
}

// The dashboard decides what appears in "work needing attention" by asking
// whether a recovery posture was reported. That only stays equivalent to the
// held-up states if every one of them reports a posture and nothing else does.
func TestRecoveryPostureMarksExactlyTheHeldUpStates(t *testing.T) {
	heldUp := map[string]bool{"blocked": true, "interrupted": true, "reconciling": true, "retry_wait": true}
	for _, state := range []string{
		"blocked", "interrupted", "reconciling", "retry_wait",
		"paused", "pause_requested", "stop_requested",
		"running", "active", "waiting", "queued", "ready", "review", "dispatching", "resuming",
	} {
		reported := attentionRecovery(state) != ""
		if reported != heldUp[state] {
			t.Fatalf("state %q reports recovery %v, want %v", state, reported, heldUp[state])
		}
	}
}

// A control the owner already requested is their turn to resume, but it is not
// work that has stopped unexpectedly. Recording the distinction keeps the
// daemon and the dashboard from drifting on it silently.
func TestOwnerRequestedControlsAreTheOwnersTurnWithoutBeingHeldUp(t *testing.T) {
	for _, state := range []string{"paused", "pause_requested", "stop_requested"} {
		if got := attentionNextAction(state); got != "owner" {
			t.Fatalf("next action for %q = %q, want owner", state, got)
		}
		if attentionRecovery(state) != "" {
			t.Fatalf("%q reports a recovery posture; the dashboard would list it as not moving", state)
		}
	}
}
