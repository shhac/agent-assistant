package core

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestDecisionDispositionPreservesWorkerAndAudit(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	project := newProject(t, s)
	ag := delegate(t, s, project, "")
	makeDecision := func() Decision {
		t.Helper()
		d, err := s.CreateDecision(ctx, DecisionInput{ProjectID: project.ID, AgentID: ag.ID, Title: "Old question", Context: "Outdated assumptions", Recommendation: "Option one", Choices: []string{"One", "Two"}})
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	selected := makeDecision()
	resolved, err := s.ResolveDecision(ctx, selected.ID, "One")
	if err != nil || resolved.Disposition != "choice" || resolved.Answer != "One" {
		t.Fatal(resolved, err)
	}
	custom := makeDecision()
	resolved, err = s.ResolveDecision(ctx, custom.ID, "Use the existing third option")
	if err != nil || resolved.Disposition != "custom" || resolved.Answer != "Use the existing third option" {
		t.Fatal(resolved, err)
	}
	obsolete := makeDecision()
	before, _ := s.Snapshot(ctx)
	for _, reason := range []string{" ", strings.Repeat("x", 4097)} {
		if _, err = s.DismissDecision(ctx, obsolete.ID, reason); err == nil {
			t.Fatal("accepted invalid reason")
		}
	}
	dismissed, err := s.DismissDecision(ctx, obsolete.ID, "Already fixed outside this discussion")
	if err != nil || dismissed.Status != "dismissed" || dismissed.Disposition != "dismissed" || dismissed.Answer != "" || dismissed.ResolvedAt == nil {
		t.Fatal(dismissed, err)
	}
	after, _ := s.Snapshot(ctx)
	if !reflect.DeepEqual(before.Agents, after.Agents) {
		t.Fatal("dismissal changed a worker")
	}
	found := false
	for _, e := range after.Activity {
		if e.Kind == "decision.dismissed" && strings.Contains(e.Summary, dismissed.ResolutionReason) {
			found = true
		}
	}
	if !found {
		t.Fatal("missing dismissal audit")
	}
	if _, err = s.ResolveDecision(ctx, obsolete.ID, "One"); !errors.Is(err, ErrConflict) {
		t.Fatal("overwrote dismissal", err)
	}
	if _, err = s.DismissDecision(ctx, selected.ID, "obsolete"); !errors.Is(err, ErrConflict) {
		t.Fatal("overwrote answer", err)
	}
}
