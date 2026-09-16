package core

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func createItem(t *testing.T, s *Service, p Project, title string) WorkItem {
	t.Helper()
	w, err := s.CreateWorkItem(testContext, WorkItemInput{ProjectID: p.ID, Title: title, Objective: "Deliver the requested outcome", AcceptanceCriteria: "Reproducible evidence demonstrates the outcome"})
	if err != nil {
		t.Fatal(err)
	}
	return w
}
func assignItem(t *testing.T, s *Service, p Project, w WorkItem) Agent {
	t.Helper()
	a, err := s.Delegate(testContext, DelegateInput{ProjectID: p.ID, WorkItemID: w.ID, ProfileID: "test", Role: "worker", Task: "Implement the scoped outcome", AcceptanceCriteria: w.AcceptanceCriteria})
	if err != nil {
		t.Fatal(err)
	}
	return a
}
func finishItemAgent(t *testing.T, s *Service, a Agent) {
	t.Helper()
	start(t, s, a)
	if _, err := s.UpdateAgent(testContext, a.ID, AgentUpdate{Status: "completed", Summary: "Outcome verified", Evidence: []string{"artifact revision abc; checks passed"}}); err != nil {
		t.Fatal(err)
	}
}
func currentItem(t *testing.T, s *Service, id string) WorkItem {
	t.Helper()
	v, err := s.Snapshot(testContext)
	if err != nil {
		t.Fatal(err)
	}
	w := workItem(&v, id)
	if w == nil {
		t.Fatalf("missing item %s", id)
	}
	return *w
}
func acceptItem(t *testing.T, s *Service, w WorkItem) WorkItem {
	t.Helper()
	w = currentItem(t, s, w.ID)
	out, err := s.AcceptWorkItem(testContext, w.ID, w.ReviewRevision, []string{"Checks and artifact inspected"}, "assistant")
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestSuccessiveWorkItemsKeepProjectOpen(t *testing.T) {
	s, _ := fixture(t)
	p, err := s.CreateProject(testContext, ProjectInput{Title: "Long lived project"})
	if err != nil {
		t.Fatal(err)
	}
	w := createItem(t, s, p, "First outcome")
	a := assignItem(t, s, p, w)
	if a.WorkItemID != w.ID {
		t.Fatal("assignment lost work item")
	}
	finishItemAgent(t, s, a)
	if err := s.CompleteProject(testContext, p.ID, []string{"done"}); err == nil {
		t.Fatal("project completion bypassed work item acceptance")
	}
	if got := currentItem(t, s, w.ID); got.Status != "review" {
		t.Fatalf("status %s", got.Status)
	}
	w = acceptItem(t, s, w)
	v, _ := s.Snapshot(testContext)
	if project(&v, p.ID).Status == "completed" {
		t.Fatal("accepting an outcome completed its project")
	}
	if err := s.CompleteProject(testContext, p.ID, []string{"explicitly archive project"}); err != nil {
		t.Fatal(err)
	}
	next := createItem(t, s, p, "Second outcome")
	nextAgent := assignItem(t, s, p, next)
	if nextAgent.WorkItemID == a.WorkItemID {
		t.Fatal("separate outcomes share attempt scope")
	}
	if got := currentItem(t, s, w.ID); got.Status != "accepted" || got.Acceptance == nil {
		t.Fatal("new outcome rewrote previous acceptance")
	}
	v, _ = s.Snapshot(testContext)
	if project(&v, p.ID).Status != "active" {
		t.Fatal("new outcome did not reactivate completed project")
	}
}

func TestDelegationWorkItemSelectionAndAuthority(t *testing.T) {
	s, _ := fixture(t)
	p := newProject(t, s)
	one := createItem(t, s, p, "One")
	a := delegate(t, s, p, "")
	if a.WorkItemID != one.ID {
		t.Fatal("unambiguous item not selected")
	}
	two := createItem(t, s, p, "Two")
	in := DelegateInput{ProjectID: p.ID, ProfileID: "test", Role: "worker", Task: "Implement", AcceptanceCriteria: "Evidence"}
	if _, err := s.Delegate(testContext, in); err == nil {
		t.Fatal("ambiguous item guessed")
	}
	in.ParentID = a.ID
	in.WorkItemID = two.ID
	if _, err := s.Delegate(testContext, in); err == nil {
		t.Fatal("coordinator escaped work item scope")
	}
	in.WorkItemID = ""
	child, err := s.Delegate(testContext, in)
	if err != nil {
		t.Fatal(err)
	}
	if child.WorkItemID != a.WorkItemID {
		t.Fatal("child did not inherit item")
	}
	other := newProject(t, s)
	in.ParentID = ""
	in.ProjectID = other.ID
	in.WorkItemID = one.ID
	if _, err := s.Delegate(testContext, in); err == nil {
		t.Fatal("cross-project item accepted")
	}
}

func TestAcceptanceRevisionAndDecisionGates(t *testing.T) {
	s, _ := fixture(t)
	p := newProject(t, s)
	w := createItem(t, s, p, "Evidence bound")
	a := assignItem(t, s, p, w)
	start(t, s, a)
	s.UpdateAgent(testContext, a.ID, AgentUpdate{Status: "running", Summary: "artifact abc", Evidence: []string{"abc"}})
	before := currentItem(t, s, w.ID)
	// Poll-only metadata changes cannot stale a review; artifact/summary changes can.
	if err := s.store.update(testContext, func(v *Snapshot) error {
		a := agent(v, a.ID)
		a.LastUpdate = a.LastUpdate.Add(time.Hour)
		a.NextCheckIn = a.NextCheckIn.Add(time.Hour)
		a.BrokerUpdatedAt = a.LastUpdate
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if after := currentItem(t, s, w.ID); after.ReviewRevision != before.ReviewRevision {
		t.Fatal("poll timestamps changed revision")
	}
	if _, err := s.AcceptWorkItem(testContext, w.ID, before.ReviewRevision, []string{"review"}, "owner"); err == nil {
		t.Fatal("unfinished attempt accepted")
	}
	s.UpdateAgent(testContext, a.ID, AgentUpdate{Status: "completed", Summary: "artifact def", Evidence: []string{"def"}})
	if _, err := s.AcceptWorkItem(testContext, w.ID, before.ReviewRevision, []string{"old review"}, "owner"); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale revision: %v", err)
	}
	d, err := s.CreateDecision(testContext, DecisionInput{Title: "Global choice", Context: "Unresolved authority", Recommendation: "Wait", Choices: []string{"Wait", "Proceed"}})
	if err != nil {
		t.Fatal(err)
	}
	cur := currentItem(t, s, w.ID)
	if _, err := s.AcceptWorkItem(testContext, w.ID, cur.ReviewRevision, []string{"review"}, "owner"); err == nil {
		t.Fatal("global unresolved decision ignored")
	}
	if _, err = s.ResolveDecision(testContext, d.ID, "Proceed"); err != nil {
		t.Fatal(err)
	}
	accepted := acceptItem(t, s, w)
	if accepted.Acceptance.Revision != accepted.ReviewRevision {
		t.Fatal("acceptance does not bind current revision")
	}
	// New evidence invalidates acceptance rather than leaving a stale green status.
	if err = s.store.update(testContext, func(v *Snapshot) error { agent(v, a.ID).Evidence = []string{"artifact ghi"}; return nil }); err != nil {
		t.Fatal(err)
	}
	if got := currentItem(t, s, w.ID); got.Status != "review" || got.Acceptance.Revision == got.ReviewRevision {
		t.Fatal("changed evidence retained acceptance")
	}
}

func TestLegacyWorkItemMigrationIsStableAndDoesNotInventAcceptance(t *testing.T) {
	s, _ := fixture(t)
	p := newProject(t, s)
	a := delegate(t, s, p, "")
	finishItemAgent(t, s, a)
	if err := s.CompleteProject(testContext, p.ID, []string{"verified before migration"}); err != nil {
		t.Fatal(err)
	}
	v, err := s.Snapshot(testContext)
	if err != nil {
		t.Fatal(err)
	}
	v.WorkItems = nil
	v.Agents[0].WorkItemID = ""
	v.Decisions = append(v.Decisions, Decision{ID: "old-decision", ProjectID: p.ID, AgentID: a.ID, Status: "resolved"})
	raw, err := json.Marshal(diskState{Snapshot: v})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.store.db.Exec("UPDATE state SET payload=? WHERE id=1", string(raw)); err != nil {
		t.Fatal(err)
	}
	first, err := s.Snapshot(testContext)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Snapshot(testContext)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.WorkItems) != 1 || !reflect.DeepEqual(first.WorkItems, second.WorkItems) {
		t.Fatal("migration is not deterministic")
	}
	w := first.WorkItems[0]
	if w.Status != "legacy_completed" || w.Acceptance != nil {
		t.Fatal("historical completion invented fresh acceptance")
	}
	if first.Agents[0].WorkItemID != w.ID || first.Decisions[0].WorkItemID != w.ID {
		t.Fatal("legacy records not linked")
	}
	next := createItem(t, s, p, "New commission")
	assignItem(t, s, p, next)
	after, _ := s.Snapshot(testContext)
	if len(after.WorkItems) != 2 {
		t.Fatal("legacy outcome lost on later commission")
	}
}

func TestSteeringSurvivesReplacementAndRequiresAcknowledgement(t *testing.T) {
	s, _ := fixture(t)
	p := newProject(t, s)
	w := createItem(t, s, p, "Steered outcome")
	a := assignItem(t, s, p, w)
	start(t, s, a)
	if _, err := s.AddSteering(testContext, w.ID, " invalid-id ", "Direction"); err == nil {
		t.Fatal("stored a message ID the worker cannot acknowledge")
	}
	m, err := s.AddSteering(testContext, w.ID, "owner-direction-1", "Preserve the existing interface")
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.AddSteering(testContext, w.ID, m.ID, m.Content)
	if err != nil || !reflect.DeepEqual(m, again) {
		t.Fatalf("idempotent steering: %v", err)
	}
	if _, err = s.AddSteering(testContext, w.ID, m.ID, "Different content"); !errors.Is(err, ErrConflict) {
		t.Fatal("steering key reused")
	}
	if _, err = s.UpdateAgent(testContext, a.ID, AgentUpdate{Status: "cancelled", Summary: "Replaced after confirmed cancellation"}); err != nil {
		t.Fatal(err)
	}
	replacement := assignItem(t, s, p, w)
	start(t, s, replacement)
	messages, err := s.SteeringForAgent(testContext, replacement.ID)
	if err != nil || len(messages) != 1 || messages[0].ID != m.ID {
		t.Fatal("replacement lost steering")
	}
	if _, err = s.UpdateAgent(testContext, replacement.ID, AgentUpdate{Status: "completed", Summary: "Interface preserved", Evidence: []string{"Compatibility check passed"}}); err != nil {
		t.Fatal(err)
	}
	cur := currentItem(t, s, w.ID)
	if _, err = s.AcceptWorkItem(testContext, w.ID, cur.ReviewRevision, []string{"reviewed"}, "assistant"); err == nil {
		t.Fatal("unacknowledged steering accepted")
	}
	// Final reports and restart replay may arrive after completion was persisted.
	if err = s.AcknowledgeSteering(testContext, replacement.ID, []string{m.ID}); err != nil {
		t.Fatal(err)
	}
	if err = s.AcknowledgeSteering(testContext, replacement.ID, []string{m.ID}); err != nil {
		t.Fatal("receipt is not idempotent")
	}
	acceptItem(t, s, w)
	other := createItem(t, s, p, "Other scope")
	b := assignItem(t, s, p, other)
	start(t, s, b)
	if err = s.AcknowledgeSteering(testContext, b.ID, []string{m.ID}); err == nil {
		t.Fatal("cross-item steering receipt accepted")
	}
	if _, err = s.AddSteering(testContext, other.ID, m.ID, m.Content); !errors.Is(err, ErrConflict) {
		t.Fatal("cross-item message key accepted")
	}
	if err = s.AcknowledgeSteering(testContext, a.ID, []string{m.ID}); err == nil {
		t.Fatal("cancelled attempt added receipt")
	}
}

func TestDecisionInheritsWorkItemAndCannotCrossScope(t *testing.T) {
	s, _ := fixture(t)
	p := newProject(t, s)
	w := createItem(t, s, p, "First")
	other := createItem(t, s, p, "Second")
	a := assignItem(t, s, p, w)
	in := DecisionInput{ProjectID: p.ID, AgentID: a.ID, Title: "Choose", Context: "Need a decision", Recommendation: "A", Choices: []string{"A", "B"}}
	d, err := s.CreateDecision(testContext, in)
	if err != nil || d.WorkItemID != w.ID {
		t.Fatalf("decision scope: %+v %v", d, err)
	}
	in.WorkItemID = other.ID
	if _, err = s.CreateDecision(testContext, in); err == nil {
		t.Fatal("decision escaped agent work item")
	}
}

func TestLaterProjectDecisionsDoNotReopenAcceptedHistory(t *testing.T) {
	s, _ := fixture(t)
	p := newProject(t, s)
	w := createItem(t, s, p, "First outcome")
	a := assignItem(t, s, p, w)
	finishItemAgent(t, s, a)
	accepted := acceptItem(t, s, w)
	next := createItem(t, s, p, "Next outcome")
	for _, in := range []DecisionInput{
		{Title: "Global choice", Context: "Future authority", Recommendation: "A", Choices: []string{"A", "B"}},
		{ProjectID: p.ID, Title: "Future project choice", Context: "Choose next direction", Recommendation: "A", Choices: []string{"A", "B"}},
		{ProjectID: p.ID, WorkItemID: next.ID, Title: "Next outcome choice", Context: "Next contract", Recommendation: "A", Choices: []string{"A", "B"}},
	} {
		d, err := s.CreateDecision(testContext, in)
		if err != nil {
			t.Fatal(err)
		}
		if got := currentItem(t, s, w.ID); got.Status != "accepted" || got.ReviewRevision != accepted.ReviewRevision {
			t.Fatal("later decision reopened accepted history")
		}
		if _, err = s.ResolveDecision(testContext, d.ID, "A"); err != nil {
			t.Fatal(err)
		}
		if got := currentItem(t, s, w.ID); got.Status != "accepted" || got.ReviewRevision != accepted.ReviewRevision {
			t.Fatal("later answer reopened accepted history")
		}
	}
}

func TestSteeringBudgetIncludesJSONEscaping(t *testing.T) {
	s, _ := fixture(t)
	p := newProject(t, s)
	w := createItem(t, s, p, "Bounded direction")
	// Each control byte is six bytes in the encoded payload.
	if _, err := s.AddSteering(testContext, w.ID, "escaped", strings.Repeat("\x01", 5000)); err == nil {
		t.Fatal("JSON escaping bypassed steering budget")
	}
	if _, err := s.AddSteering(testContext, w.ID, "first", strings.Repeat("x", 13000)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddSteering(testContext, w.ID, "second", strings.Repeat("y", 13000)); err == nil {
		t.Fatal("aggregate steering budget bypassed")
	}
	v, err := s.Snapshot(testContext)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Steering) != 1 {
		t.Fatal("rejected steering mutated state")
	}
}
