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
		p := Project{ContractDefined: true, ID: "demo-project", Title: "Community knowledge hub", Description: "Make the team's useful answers easy to find.", AcceptanceCriteria: "A searchable prototype with accessible keyboard navigation and a review report.", Status: "active", UpdatedAt: now}
		v.Projects = append(v.Projects, p)
		v.Agents = append(v.Agents, Agent{ID: "demo-manager", ProjectID: p.ID, ProfileID: "demo", Name: "Project coordinator", Role: "manager", Status: "waiting", Task: "Coordinate the knowledge hub prototype", AcceptanceCriteria: p.AcceptanceCriteria, Capabilities: []string{"coordinate", "implement", "review"}, DispatchKey: "demo-manager", Depth: 1, LastUpdate: now.Add(-5 * time.Minute), NextCheckIn: now.Add(25 * time.Minute), Summary: "Prototype reviewed. Waiting for the owner's audience decision.", Evidence: []string{}}, Agent{ID: "demo-worker", ProjectID: p.ID, ParentID: "demo-manager", ProfileID: "demo", Name: "Prototype builder", Role: "worker", Status: "completed", Task: "Build the isolated prototype", AcceptanceCriteria: "Working search and keyboard navigation", Capabilities: []string{"implement"}, DispatchKey: "demo-worker", Depth: 2, LastUpdate: now.Add(-10 * time.Minute), NextCheckIn: now, Summary: "Prototype ready; acceptance checks recorded.", Evidence: []string{"Fictional demo: search and keyboard-navigation checks passed."}})
		v.Decisions = append(v.Decisions, Decision{ID: "demo-decision", ProjectID: p.ID, AgentID: "demo-manager", Title: "Who should the first prototype serve?", Context: "A focused audience lets the team validate search quality before widening scope.", Recommendation: "Start with the support team, then expand after feedback.", Choices: []string{"Support team first", "Whole organization"}, Status: "open", CreatedAt: now.Add(-3 * time.Minute)})
		v.Memories = append(v.Memories, Memory{ID: "demo-memory", Key: "decision_style", Content: "Bring a recommendation, the trade-off and supporting evidence.", UpdatedAt: now})
		v.Messages = append(v.Messages, Message{ID: "demo-message", Role: "assistant", Content: "The knowledge hub prototype is ready for review. I have one audience decision for you; the team can continue once it is resolved.", CreatedAt: now})
		record(v, now, p.ID, "demo.created", "Fictional demonstration; no external workers or services were contacted")
		v.Paused = true
		return nil
	})
}
