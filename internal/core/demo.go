package core

import (
	"context"
	"errors"
	"time"
)

// SeedDemo installs fictional records only into an empty database. The caller
// must explicitly choose demonstration mode and disable all external adapters.
func (s *Service) SeedDemo(ctx context.Context) error {
	return s.store.update(ctx, func(v *Snapshot) error {
		if len(v.Projects) > 0 || len(v.Messages) > 0 || len(v.Agents) > 0 || len(v.Decisions) > 0 || len(v.Memories) > 0 {
			return errors.New("demo requires an empty database")
		}
		now := s.now().UTC()
		if err := s.seedKnowledgeHub(v, now); err != nil {
			return err
		}
		if err := s.seedReleaseChecklist(v, now); err != nil {
			return err
		}
		v.Paused = true
		return nil
	})
}

// seedKnowledgeHub is the original fixture: an outcome reviewed and waiting on
// one owner decision.
func (s *Service) seedKnowledgeHub(v *Snapshot, now time.Time) error {
	p := Project{ContractDefined: true, ID: "demo-project", Title: "Community knowledge hub", Description: "Make the team's useful answers easy to find.", AcceptanceCriteria: "A searchable prototype with accessible keyboard navigation and a review report.", Status: "active", UpdatedAt: now}
	if err := s.store.prepareProject(&p); err != nil {
		return err
	}
	v.Projects = append(v.Projects, p)
	v.Agents = append(v.Agents, Agent{ID: "demo-manager", ProjectID: p.ID, ProfileID: "demo", Name: "Project coordinator", Role: "manager", Status: "waiting", Task: "Coordinate the knowledge hub prototype", AcceptanceCriteria: p.AcceptanceCriteria, Capabilities: []string{"coordinate", "implement", "review"}, DispatchKey: "demo-manager", Depth: 1, LastUpdate: now.Add(-5 * time.Minute), NextCheckIn: now.Add(25 * time.Minute), Summary: "Prototype reviewed. Waiting for the owner's audience decision.", Evidence: []string{}}, Agent{ID: "demo-worker", ProjectID: p.ID, ParentID: "demo-manager", ProfileID: "demo", Name: "Prototype builder", Role: "worker", Status: "completed", Task: "Build the isolated prototype", AcceptanceCriteria: "Working search and keyboard navigation", Capabilities: []string{"implement"}, DispatchKey: "demo-worker", Depth: 2, LastUpdate: now.Add(-10 * time.Minute), NextCheckIn: now, Summary: "Prototype ready; acceptance checks recorded.", Evidence: []string{"Fictional demo: search and keyboard-navigation checks passed."}})
	v.Decisions = append(v.Decisions, Decision{ID: "demo-decision", ProjectID: p.ID, AgentID: "demo-manager", Title: "Who should the first prototype serve?", Context: "A focused audience lets the team validate search quality before widening scope.", Recommendation: "Start with the support team, then expand after feedback.", Choices: []string{"Support team first", "Whole organization"}, Status: "open", CreatedAt: now.Add(-3 * time.Minute)})
	v.Memories = append(v.Memories, Memory{ID: "demo-memory", Key: "decision_style", Content: "Bring a recommendation, the trade-off and supporting evidence.", UpdatedAt: now})
	v.Messages = append(v.Messages, Message{ID: "demo-message", Role: "assistant", Content: "The knowledge hub prototype is ready for review. I have one audience decision for you; the team can continue once it is resolved.", CreatedAt: now})
	record(v, now, p.ID, "demo.created", "Fictional demonstration; no external workers or services were contacted")
	return nil
}

// seedReleaseChecklist covers the execution states the dashboard has to report
// honestly: work running, work stopped while nothing waits on the owner's
// judgment, an outcome awaiting review and an outcome queued behind another.
// Without these the health, failure, queue and evidence views cannot be seen in
// demonstration mode at all.
func (s *Service) seedReleaseChecklist(v *Snapshot, now time.Time) error {
	q := Project{ContractDefined: true, ID: "demo-delivery", Title: "Release checklist", Description: "Get the release steps reliable and repeatable.", AcceptanceCriteria: "Each step is automated or explicitly owned, with evidence.", Status: "active", UpdatedAt: now}
	if err := s.store.prepareProject(&q); err != nil {
		return err
	}
	v.Projects = append(v.Projects, q)
	v.WorkItems = append(v.WorkItems,
		WorkItem{ID: "demo-running", ProjectID: q.ID, Title: "Automate the checklist", Objective: "Replace the manual steps with a checked script.", AcceptanceCriteria: "Every step runs unattended\nFailures stop the run", Status: "active", CreatedAt: now.Add(-2 * time.Hour), UpdatedAt: now.Add(-9 * time.Minute)},
		WorkItem{ID: "demo-blocked", ProjectID: q.ID, Title: "Suggest the next step", Objective: "Offer the owner a next step from the checklist state.", AcceptanceCriteria: "Suggestions are derived from recorded state only", Status: "blocked", CreatedAt: now.Add(-3 * time.Hour), UpdatedAt: now.Add(-41 * time.Minute)},
		WorkItem{ID: "demo-review", ProjectID: q.ID, Title: "Document the rollback", Objective: "Write down how to undo a release.", AcceptanceCriteria: "A reviewer can follow it without asking questions", Status: "review", CreatedAt: now.Add(-4 * time.Hour), UpdatedAt: now.Add(-20 * time.Minute)},
		WorkItem{ID: "demo-queued", ProjectID: q.ID, AfterWorkItemID: "demo-blocked", CommissionRequested: true, Title: "Attach evidence to each step", Objective: "Record what proves a step succeeded.", AcceptanceCriteria: "Each step names its evidence", Status: "queued", CreatedAt: now.Add(-1 * time.Hour), UpdatedAt: now.Add(-1 * time.Hour)},
	)
	v.Agents = append(v.Agents,
		Agent{ID: "demo-runner", ProjectID: q.ID, WorkItemID: "demo-running", ProfileID: "demo", Name: "Checklist builder", Role: "worker", Status: "running", Task: "Automate the checklist", AcceptanceCriteria: "Every step runs unattended", Capabilities: []string{"implement"}, DispatchKey: "demo-runner", Depth: 1, LastUpdate: now.Add(-9 * time.Minute), LastProgressAt: now.Add(-9 * time.Minute), NextCheckIn: now.Add(21 * time.Minute), Summary: "Fictional demo: converting the third step.", Evidence: []string{}},
		Agent{ID: "demo-blocked-worker", ProjectID: q.ID, WorkItemID: "demo-blocked", ProfileID: "demo", Name: "Suggestions worker", Role: "worker", Status: "blocked", Task: "Suggest the next step", AcceptanceCriteria: "Suggestions are derived from recorded state only", Capabilities: []string{"implement"}, DispatchKey: "demo-blocked-worker", Depth: 1, LastUpdate: now.Add(-41 * time.Minute), LastProgressAt: now.Add(-41 * time.Minute), NextCheckIn: now.Add(-11 * time.Minute), ProviderFailureKind: "unknown", ModelFailureEngine: "demo-engine", ModelFailureEvidence: "untyped_error", Recoveries: 1, Summary: "Fictional demo: the attempt stopped without a classified provider error. Inspect the preserved work before explicitly resuming.", Evidence: []string{"Broker evidence: 0 changed files; 3 recorded commands: 3 SUCCEEDED, 0 FAILED. Command success is an exit-status fact, not proof that acceptance criteria are met. No unrecorded check is verified.", "Changed paths (0 of 0 shown; 0 omitted): []", "Command 1 SUCCEEDED: demo check\ncaptured output: fictional output"}},
		Agent{ID: "demo-retry", ProjectID: q.ID, WorkItemID: "demo-review", ProfileID: "demo", Name: "Rollback writer", Role: "worker", Status: "completed", Task: "Document the rollback", AcceptanceCriteria: "A reviewer can follow it without asking questions", Capabilities: []string{"implement"}, DispatchKey: "demo-retry", Depth: 1, LastUpdate: now.Add(-20 * time.Minute), LastProgressAt: now.Add(-20 * time.Minute), NextCheckIn: now, Summary: "Fictional demo: rollback steps drafted and ready for review.", Evidence: []string{"Fictional demo: rollback walkthrough recorded."}},
	)
	record(v, now.Add(-41*time.Minute), q.ID, "agent.blocked", "Suggestions worker: the attempt stopped without a classified provider error")
	record(v, now.Add(-20*time.Minute), q.ID, "work_item.review_ready", "Document the rollback")
	record(v, now.Add(-9*time.Minute), q.ID, "agent.running", "Checklist builder: converting the third step")
	return nil
}
