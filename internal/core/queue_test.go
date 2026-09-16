package core

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func queueItem(t *testing.T, s *Service, p Project, after WorkItem) WorkItem {
	t.Helper()
	w, err := s.QueueWorkItem(testContext, WorkItemInput{ProjectID: p.ID, AfterWorkItemID: after.ID, Title: "Next outcome", Objective: "Do the second task after the first", AcceptanceCriteria: "Second outcome demonstrated"})
	if err != nil {
		t.Fatal(err)
	}
	return w
}
func TestQueueRequiresExplicitCommissionRequestAndAcceptance(t *testing.T) {
	s, _ := fixture(t)
	p := newProject(t, s)
	first := createItem(t, s, p, "First outcome")
	next := queueItem(t, s, p, first)
	if next.Status != "queued" || !next.CommissionRequested || next.AfterWorkItemID != first.ID {
		t.Fatalf("not durably queued: %+v", next)
	}
	// Ordinary intake may specify an ordering constraint, but that does not grant
	// permission to commission work automatically after its prerequisite.
	draft, err := s.CreateWorkItem(testContext, WorkItemInput{ProjectID: p.ID, AfterWorkItemID: first.ID, Title: "Possible later work", Objective: "A draft", AcceptanceCriteria: "Review before commissioning"})
	if err != nil {
		t.Fatal(err)
	}
	if draft.CommissionRequested {
		t.Fatal("intake invented commissioning permission")
	}
	if ready, err := s.ReadyQueuedWorkItems(testContext); err != nil || len(ready) != 0 {
		t.Fatalf("premature queued commissioning: %+v %v", ready, err)
	}
	in := DelegateInput{ProjectID: p.ID, WorkItemID: next.ID, ProfileID: "test", Role: "worker", Task: "Second task", AcceptanceCriteria: next.AcceptanceCriteria}
	if _, err = s.Delegate(testContext, in); err == nil {
		t.Fatal("unfinished predecessor did not block delegation")
	}
	a := assignItem(t, s, p, first)
	finishItemAgent(t, s, a)
	if ready, _ := s.ReadyQueuedWorkItems(testContext); len(ready) != 0 {
		t.Fatal("completion without acceptance released queue")
	}
	acceptItem(t, s, first)
	ready, err := s.ReadyQueuedWorkItems(testContext)
	if err != nil || len(ready) != 1 || ready[0].ID != next.ID {
		t.Fatalf("authorized queue not released: %+v %v", ready, err)
	}
	if ready[0].Status != "ready" {
		t.Fatal("released queue status not ready")
	}
	b, err := s.Delegate(testContext, in)
	if err != nil {
		t.Fatal(err)
	}
	if ready, _ := s.ReadyQueuedWorkItems(testContext); len(ready) != 0 {
		t.Fatal("existing attempt allowed duplicate commissioning")
	}
	start(t, s, b)
	if _, err = s.UpdateAgent(testContext, b.ID, AgentUpdate{Status: "interrupted", Summary: "CLI exited unexpectedly"}); err != nil {
		t.Fatal(err)
	}
	if ready, _ := s.ReadyQueuedWorkItems(testContext); len(ready) != 0 {
		t.Fatal("interruption authorized a fresh duplicate attempt")
	}
}

func TestQueueSurvivesReloadAndHonoursPauseAndDecisions(t *testing.T) {
	s, _ := fixture(t)
	p := newProject(t, s)
	first := createItem(t, s, p, "First")
	a := assignItem(t, s, p, first)
	finishItemAgent(t, s, a)
	acceptItem(t, s, first)
	next := queueItem(t, s, p, first)
	// Round-trip through the actual durable blob rather than keeping service state.
	before, err := s.store.Snapshot(testContext)
	if err != nil {
		t.Fatal(err)
	}
	var payload string
	if err = s.store.db.QueryRow("SELECT payload FROM state WHERE id=1").Scan(&payload); err != nil {
		t.Fatal(err)
	}
	var saved diskState
	if err = json.Unmarshal([]byte(payload), &saved); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before.WorkItems, saved.Snapshot.WorkItems) {
		t.Fatal("queue fields lost in durable storage")
	}
	if err = s.SetPaused(testContext, true); err != nil {
		t.Fatal(err)
	}
	if ready, _ := s.ReadyQueuedWorkItems(testContext); len(ready) != 0 {
		t.Fatal("pause did not hold queue")
	}
	if got := currentItem(t, s, next.ID); got.Status != "paused" {
		t.Fatalf("paused queue labelled %s", got.Status)
	}
	s.SetPaused(testContext, false)
	d, err := s.CreateDecision(testContext, DecisionInput{ProjectID: p.ID, WorkItemID: next.ID, Title: "Confirm scope", Context: "Unresolved input", Recommendation: "Wait", Choices: []string{"Wait", "Proceed"}})
	if err != nil {
		t.Fatal(err)
	}
	if ready, _ := s.ReadyQueuedWorkItems(testContext); len(ready) != 0 {
		t.Fatal("unresolved decision did not hold queue")
	}
	if got := currentItem(t, s, next.ID); got.Status != "blocked" {
		t.Fatalf("blocked queue labelled %s", got.Status)
	}
	if _, err = s.ResolveDecision(testContext, d.ID, "Proceed"); err != nil {
		t.Fatal(err)
	}
	if ready, _ := s.ReadyQueuedWorkItems(testContext); len(ready) != 1 || ready[0].ID != next.ID {
		t.Fatal("decision resolution did not release queue")
	}
}

func TestQueueRejectsUnknownCrossProjectAndCyclicPredecessors(t *testing.T) {
	s, _ := fixture(t)
	p := newProject(t, s)
	first := createItem(t, s, p, "First")
	other := newProject(t, s)
	foreign := createItem(t, s, other, "Other scope")
	in := WorkItemInput{ProjectID: p.ID, Title: "Queued", Objective: "Outcome", AcceptanceCriteria: "Evidence"}
	for _, id := range []string{"", "missing", foreign.ID} {
		in.AfterWorkItemID = id
		if _, err := s.QueueWorkItem(testContext, in); err == nil {
			t.Fatalf("accepted predecessor %q", id)
		}
	}
	if err := s.store.update(testContext, func(v *Snapshot) error { workItem(v, first.ID).AfterWorkItemID = first.ID; return nil }); err != nil {
		t.Fatal(err)
	}
	in.AfterWorkItemID = first.ID
	if _, err := s.QueueWorkItem(testContext, in); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("cyclic predecessor accepted: %v", err)
	}
}

func TestDispatchRechecksPredecessorAcceptance(t *testing.T) {
	s, _ := fixture(t)
	p := newProject(t, s)
	first := createItem(t, s, p, "First")
	a := assignItem(t, s, p, first)
	finishItemAgent(t, s, a)
	acceptItem(t, s, first)
	next := queueItem(t, s, p, first)
	b := assignItem(t, s, p, next)
	// If the accepted artifact changes between assignment and launch, dispatch
	// must require a fresh acceptance instead of trusting the earlier check.
	if err := s.store.update(testContext, func(v *Snapshot) error { agent(v, a.ID).Evidence = []string{"different artifact revision"}; return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := s.BeginDispatch(testContext, b.ID); err == nil {
		t.Fatal("dispatch ignored invalidated prerequisite acceptance")
	}
}

func TestWorkItemStatusDescribesInterruptedAndWaitingExecution(t *testing.T) {
	s, _ := fixture(t)
	p := newProject(t, s)
	w := createItem(t, s, p, "Honest status")
	a := assignItem(t, s, p, w)
	if got := currentItem(t, s, w.ID); got.Status != "waiting" {
		t.Fatalf("unstarted assignment labelled %s", got.Status)
	}
	start(t, s, a)
	if got := currentItem(t, s, w.ID); got.Status != "active" {
		t.Fatalf("running assignment labelled %s", got.Status)
	}
	for _, status := range []string{"interrupted", "blocked", "waiting"} {
		summary := "Worker reported " + status
		if _, err := s.UpdateAgent(testContext, a.ID, AgentUpdate{Status: status, Summary: summary}); err != nil {
			t.Fatal(err)
		}
		got := currentItem(t, s, w.ID)
		if got.Status != status || got.StatusReason != summary {
			t.Fatalf("untruthful outcome state: %+v", got)
		}
	}
	if _, err := s.UpdateAgent(testContext, a.ID, AgentUpdate{Status: "cancelled", Summary: "Stopped by owner"}); err != nil {
		t.Fatal(err)
	}
	if got := currentItem(t, s, w.ID); got.Status != "cancelled" {
		t.Fatalf("stopped outcome labelled %s", got.Status)
	}
}

func TestRunningAttemptTakesPrecedenceOverInterruptedSibling(t *testing.T) {
	s, _ := fixture(t)
	p := newProject(t, s)
	w := createItem(t, s, p, "Shared outcome")
	a := assignItem(t, s, p, w)
	start(t, s, a)
	if _, err := s.UpdateAgent(testContext, a.ID, AgentUpdate{Status: "interrupted", Summary: "One attempt interrupted"}); err != nil {
		t.Fatal(err)
	}
	b := assignItem(t, s, p, w)
	start(t, s, b)
	if got := currentItem(t, s, w.ID); got.Status != "active" {
		t.Fatalf("active work hidden by interrupted sibling: %+v", got)
	}
}

func TestCancelQueuedWorkRetainsDraftAndNeverStartsOnAcceptance(t *testing.T) {
	s, _ := fixture(t)
	p := newProject(t, s)
	first := createItem(t, s, p, "First")
	next := queueItem(t, s, p, first)
	draft, err := s.CancelQueuedWorkItem(testContext, next.ID)
	if err != nil {
		t.Fatal(err)
	}
	if draft.CommissionRequested || draft.AfterWorkItemID != first.ID || draft.Objective != next.Objective || draft.Status == "queued" {
		t.Fatalf("withdrawn request not retained as draft: %+v", draft)
	}
	snapshot, _ := s.Snapshot(testContext)
	activityCount := len(snapshot.Activity)
	again, err := s.CancelQueuedWorkItem(testContext, next.ID)
	if err != nil || !reflect.DeepEqual(draft, again) {
		t.Fatalf("withdrawal not idempotent: %+v %v", again, err)
	}
	snapshot, _ = s.Snapshot(testContext)
	if len(snapshot.Activity) != activityCount {
		t.Fatal("duplicate withdrawal emitted activity")
	}
	a := assignItem(t, s, p, first)
	finishItemAgent(t, s, a)
	accepted := acceptItem(t, s, first)
	if ready, _ := s.ReadyQueuedWorkItems(testContext); len(ready) != 0 {
		t.Fatal("withdrawn request auto-commissionable after predecessor acceptance")
	}
	if got := currentItem(t, s, first.ID); got.Acceptance.Revision != accepted.Acceptance.Revision || got.Status != "accepted" {
		t.Fatal("withdrawal invalidated unrelated acceptance")
	}
	// The draft remains manually commissionable when the owner later asks.
	b := assignItem(t, s, p, draft)
	if _, err = s.CancelQueuedWorkItem(testContext, draft.ID); err == nil {
		t.Fatal("queue withdrawal claimed to stop an existing assignment")
	}
	if got := currentItem(t, s, draft.ID); got.CommissionRequested {
		t.Fatal("manual assignment invented persisted auto-commission request")
	}
	if b.WorkItemID != draft.ID {
		t.Fatal("manual assignment lost draft identity")
	}
}

func TestQueuedCommissionRechecksWithdrawnPermissionAtomically(t *testing.T) {
	s, _ := fixture(t)
	p := newProject(t, s)
	first := createItem(t, s, p, "First")
	a := assignItem(t, s, p, first)
	finishItemAgent(t, s, a)
	acceptItem(t, s, first)
	next := queueItem(t, s, p, first)
	ready, err := s.ReadyQueuedWorkItems(testContext)
	if err != nil || len(ready) != 1 {
		t.Fatalf("queue should be ready: %v", err)
	}
	if _, err = s.CancelQueuedWorkItem(testContext, next.ID); err != nil {
		t.Fatal(err)
	}
	in := DelegateInput{ProjectID: p.ID, WorkItemID: next.ID, ProfileID: "test", Role: "worker", Task: "A model action based on the old queue snapshot", AcceptanceCriteria: next.AcceptanceCriteria, RequireCommissionRequest: true}
	if _, err = s.Delegate(testContext, in); err == nil {
		t.Fatal("stale queue action ignored withdrawn commissioning permission")
	}
	snapshot, _ := s.Snapshot(testContext)
	for _, attempt := range snapshot.Agents {
		if attempt.WorkItemID == next.ID {
			t.Fatal("rejected queue action created an attempt")
		}
	}
	// A later explicit owner request can still commission the preserved draft.
	in.RequireCommissionRequest = false
	if _, err = s.Delegate(testContext, in); err != nil {
		t.Fatal(err)
	}
}
