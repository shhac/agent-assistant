package core

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/shhac/agent-assistant/internal/config"
	"strings"
	"sync"
	"time"
)

var ErrNotFound = errors.New("not found")
var ErrConflict = errors.New("state conflict")

// Service deliberately exposes no execution, shell, production or purchase capability.
type Service struct {
	store *Store
	mu    sync.RWMutex
	cfg   config.Config
	now   func() time.Time
}

func NewService(store *Store, cfg config.Config) *Service {
	return &Service{store: store, cfg: cfg, now: time.Now}
}
func (s *Service) UpdateConfig(cfg config.Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	s.cfg = cfg
	s.mu.Unlock()
	return nil
}
func (s *Service) configuration() config.Config { s.mu.RLock(); defer s.mu.RUnlock(); return s.cfg }
func (s *Service) GetProfile(id string) (config.Worker, error) {
	for _, w := range s.configuration().Workers {
		if w.ID == id {
			return w, nil
		}
	}
	return config.Worker{}, fmt.Errorf("worker profile %q: %w", id, ErrNotFound)
}
func (s *Service) Snapshot(ctx context.Context) (Snapshot, error) {
	v, err := s.store.Snapshot(ctx)
	cfg := s.configuration()
	v.Assistant = Assistant{Name: cfg.Assistant.Name, Personality: cfg.Assistant.Personality}
	return v, err
}
func uid() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}
func record(v *Snapshot, now time.Time, project, kind, summary string) {
	v.Activity = append(v.Activity, Activity{ID: uid(), ProjectID: project, Kind: kind, Summary: summary, CreatedAt: now})
}
func project(v *Snapshot, id string) *Project {
	for i := range v.Projects {
		if v.Projects[i].ID == id {
			return &v.Projects[i]
		}
	}
	return nil
}
func agent(v *Snapshot, id string) *Agent {
	for i := range v.Agents {
		if v.Agents[i].ID == id {
			return &v.Agents[i]
		}
	}
	return nil
}
func terminal(status string) bool { return status == "completed" || status == "cancelled" }
func required(fields ...string) bool {
	for _, f := range fields {
		if strings.TrimSpace(f) == "" {
			return false
		}
	}
	return true
}
func (s *Service) CreateProject(ctx context.Context, in ProjectInput) (Project, error) {
	if !required(in.Title, in.AcceptanceCriteria) {
		return Project{}, errors.New("title and acceptance criteria are required")
	}
	out := Project{ID: uid(), Title: in.Title, Description: in.Description, AcceptanceCriteria: in.AcceptanceCriteria, Status: "ready", SourceID: in.SourceID, UpdatedAt: s.now().UTC()}
	err := s.store.update(ctx, func(v *Snapshot) error {
		if in.SourceID != "" {
			for i := range v.Projects {
				p := &v.Projects[i]
				if p.SourceID == in.SourceID {
					p.Title = in.Title
					p.Description = in.Description
					p.AcceptanceCriteria = in.AcceptanceCriteria
					p.UpdatedAt = s.now().UTC()
					out = *p
					return nil
				}
			}
		}
		v.Projects = append(v.Projects, out)
		record(v, out.UpdatedAt, out.ID, "project.created", out.Title)
		return nil
	})
	return out, err
}
func (s *Service) Delegate(ctx context.Context, in DelegateInput) (Agent, error) {
	if !required(in.ProjectID, in.ProfileID, in.Task, in.AcceptanceCriteria) {
		return Agent{}, errors.New("project, worker profile, task and acceptance criteria are required")
	}
	if in.Role != "manager" && in.Role != "worker" && in.Role != "reviewer" && in.Role != "researcher" {
		return Agent{}, errors.New("role must be manager, worker, reviewer or researcher")
	}
	profile, err := s.GetProfile(in.ProfileID)
	if err != nil {
		return Agent{}, err
	}
	if len(in.Capabilities) == 0 {
		in.Capabilities = append([]string(nil), profile.Capabilities...)
	}
	for _, cap := range in.Capabilities {
		if !contains(profile.Capabilities, cap) {
			return Agent{}, fmt.Errorf("worker profile does not grant %q", cap)
		}
	}
	if len(in.Capabilities) == 0 {
		return Agent{}, errors.New("worker must have at least one scoped capability")
	}
	if in.Role == "manager" && !contains(in.Capabilities, "coordinate") {
		return Agent{}, errors.New("manager role requires coordinate capability")
	}
	now := s.now().UTC()
	cfg := s.configuration()
	out := Agent{ID: uid(), ProjectID: in.ProjectID, ParentID: in.ParentID, ProfileID: in.ProfileID, Name: in.Name, Role: in.Role, Status: "queued", Task: in.Task, AcceptanceCriteria: in.AcceptanceCriteria, Capabilities: in.Capabilities, DispatchKey: uid(), Depth: 1, LastUpdate: now, NextCheckIn: now.Add(time.Duration(cfg.Limits.CheckInMinutes) * time.Minute), Summary: "Waiting for an approved worker runtime", Evidence: []string{}}
	if out.Name == "" {
		out.Name = profile.Name
	}
	if out.Name == "" {
		out.Name = profile.ID
	}
	err = s.store.update(ctx, func(v *Snapshot) error {
		if v.Paused {
			return errors.New("coordination is paused")
		}
		p := project(v, in.ProjectID)
		if p == nil {
			return fmt.Errorf("project: %w", ErrNotFound)
		}
		if p.Status == "completed" {
			return errors.New("completed project cannot receive new work")
		}
		if in.ParentID != "" {
			parent := agent(v, in.ParentID)
			if parent == nil {
				return fmt.Errorf("parent: %w", ErrNotFound)
			}
			if parent.ProjectID != in.ProjectID || terminal(parent.Status) {
				return errors.New("parent must be active in the same project")
			}
			if !contains(parent.Capabilities, "coordinate") {
				return errors.New("parent lacks coordination authority")
			}
			for _, cap := range in.Capabilities {
				if !contains(parent.Capabilities, cap) {
					return fmt.Errorf("child capability %q exceeds parent authority", cap)
				}
			}
			out.Depth = parent.Depth + 1
		}
		if out.Depth > cfg.Limits.MaxDepth {
			return errors.New("delegation depth limit reached")
		}
		p.Status = "active"
		p.UpdatedAt = now
		v.Agents = append(v.Agents, out)
		record(v, now, in.ProjectID, "agent.queued", out.Name+": "+in.Task)
		return nil
	})
	return out, err
}
func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

// BeginDispatch commits intent before the caller contacts a worker. Recovery must
// reconcile dispatching/reconciling records by DispatchKey, never blindly repeat.
func (s *Service) BeginDispatch(ctx context.Context, id string) (Agent, error) {
	var out Agent
	err := s.store.update(ctx, func(v *Snapshot) error {
		a := agent(v, id)
		if a == nil {
			return ErrNotFound
		}
		if v.Paused {
			return errors.New("coordination is paused")
		}
		if a.Status != "queued" {
			return fmt.Errorf("dispatch requires queued state: %w", ErrConflict)
		}
		if _, err := s.GetProfile(a.ProfileID); err != nil {
			return err
		}
		if executingCount(v) >= s.configuration().Limits.MaxAgents {
			return errors.New("agent execution capacity reached")
		}
		a.Status = "dispatching"
		a.LastUpdate = s.now().UTC()
		a.NextCheckIn = a.LastUpdate.Add(time.Duration(s.configuration().Limits.CheckInMinutes) * time.Minute)
		out = *a
		record(v, a.LastUpdate, a.ProjectID, "agent.dispatching", a.Name)
		return nil
	})
	return out, err
}
func (s *Service) MarkDispatched(ctx context.Context, id, externalID string) error {
	if !required(externalID) {
		return errors.New("external worker ID is required")
	}
	return s.store.update(ctx, func(v *Snapshot) error {
		a := agent(v, id)
		if a == nil {
			return ErrNotFound
		}
		if a.Status == "running" && a.ExternalID == externalID {
			return nil
		}
		if a.Status != "dispatching" && a.Status != "reconciling" && a.Status != "resuming" {
			return ErrConflict
		}
		a.Status = "running"
		a.ExternalID = externalID
		a.LastUpdate = s.now().UTC()
		a.NextCheckIn = a.LastUpdate.Add(time.Duration(s.configuration().Limits.CheckInMinutes) * time.Minute)
		a.Summary = "Worker acknowledged the assignment"
		record(v, a.LastUpdate, a.ProjectID, "agent.started", a.Name)
		return nil
	})
}
func (s *Service) MarkUncertain(ctx context.Context, id, reason string) error {
	return s.store.update(ctx, func(v *Snapshot) error {
		a := agent(v, id)
		if a == nil {
			return ErrNotFound
		}
		if terminal(a.Status) {
			return ErrConflict
		}
		if a.Status == "reconciling" && a.Summary == reason {
			return nil
		}
		a.Status = "reconciling"
		a.Summary = reason
		record(v, s.now().UTC(), a.ProjectID, "agent.reconciling", a.Name+": "+reason)
		return nil
	})
}
func (s *Service) UpdateAgent(ctx context.Context, id string, in AgentUpdate) (Agent, error) {
	if !contains([]string{"running", "waiting", "blocked", "interrupted", "completed", "cancelled"}, in.Status) {
		return Agent{}, errors.New("invalid worker update status")
	}
	if !required(in.Summary) {
		return Agent{}, errors.New("worker update requires a summary")
	}
	if in.Status == "completed" {
		if len(in.Evidence) == 0 {
			return Agent{}, errors.New("completion requires acceptance evidence")
		}
		for _, e := range in.Evidence {
			if !required(e) {
				return Agent{}, errors.New("completion evidence must be nonempty")
			}
		}
	}
	var out Agent
	err := s.store.update(ctx, func(v *Snapshot) error {
		a := agent(v, id)
		if a == nil {
			return ErrNotFound
		}
		if in.ExternalID != "" && a.ExternalID != "" && in.ExternalID != a.ExternalID {
			return errors.New("worker update identity does not match")
		}
		if terminal(a.Status) {
			return fmt.Errorf("worker is already terminal: %w", ErrConflict)
		}
		if a.Status == "queued" {
			return errors.New("unstarted work cannot report progress")
		}
		if in.Status == "completed" {
			for _, child := range v.Agents {
				if child.ParentID == id && !terminal(child.Status) {
					return errors.New("manager cannot complete with unfinished descendants")
				}
			}
		}
		if !in.UpdatedAt.IsZero() {
			if in.UpdatedAt.After(s.now().Add(time.Minute)) {
				return errors.New("worker timestamp is in the future")
			}
			if !a.BrokerUpdatedAt.IsZero() && in.UpdatedAt.Before(a.BrokerUpdatedAt) {
				return errors.New("worker report is older than the recorded report")
			}
			a.BrokerUpdatedAt = in.UpdatedAt.UTC()
		}
		a.Status = in.Status
		a.Summary = in.Summary
		a.Evidence = append([]string{}, in.Evidence...)
		if a.ExternalID == "" {
			a.ExternalID = in.ExternalID
		}
		a.LastUpdate = s.now().UTC()
		if !in.UpdatedAt.IsZero() {
			a.LastUpdate = in.UpdatedAt.UTC()
		}
		a.NextCheckIn = a.LastUpdate.Add(time.Duration(s.configuration().Limits.CheckInMinutes) * time.Minute)
		out = *a
		record(v, a.LastUpdate, a.ProjectID, "agent."+in.Status, a.Name+": "+in.Summary)
		if p := project(v, a.ProjectID); p != nil {
			p.UpdatedAt = a.LastUpdate
		}
		return nil
	})
	return out, err
}

// RetryAgent is only available after confirmed interruption. Silence and an
// ambiguous network response never authorize a duplicate external worker.
func (s *Service) RetryAgent(ctx context.Context, id string) (Agent, error) {
	return s.PrepareResume(ctx, id)
}

// PrepareResume persists one resume attempt before the adapter sends it. The
// original external session and launch key survive; ResumeKey deduplicates this attempt.
func (s *Service) PrepareResume(ctx context.Context, id string) (Agent, error) {
	var out Agent
	cfg := s.configuration()
	err := s.store.update(ctx, func(v *Snapshot) error {
		a := agent(v, id)
		if a == nil {
			return ErrNotFound
		}
		if v.Paused {
			return errors.New("coordination is paused")
		}
		if a.Status != "interrupted" {
			return errors.New("recovery requires confirmed interruption")
		}
		if a.Recoveries >= cfg.Limits.MaxRecoveries {
			return errors.New("recovery allowance exhausted; owner decision required")
		}
		for _, child := range v.Agents {
			if child.ParentID == id && !terminal(child.Status) {
				return errors.New("reconcile descendants before retrying their manager")
			}
		}
		if a.ExternalID == "" {
			return errors.New("cannot resume without an external session; reconcile first")
		}
		if executingCount(v) >= cfg.Limits.MaxAgents {
			return errors.New("agent execution capacity reached")
		}
		a.Recoveries++
		a.Status = "resuming"
		a.ResumeKey = fmt.Sprintf("%s:resume:%d", a.DispatchKey, a.Recoveries)
		a.LastUpdate = s.now().UTC()
		a.Summary = "Confirmed interruption; resume requested"
		out = *a
		record(v, a.LastUpdate, a.ProjectID, "agent.recovery", a.Name)
		return nil
	})
	return out, err
}
func (s *Service) CreateDecision(ctx context.Context, in DecisionInput) (Decision, error) {
	if !required(in.Title, in.Context, in.Recommendation) || len(in.Choices) < 2 {
		return Decision{}, errors.New("decision requires title, context, recommendation and at least two choices")
	}
	for _, c := range in.Choices {
		if !required(c) {
			return Decision{}, errors.New("decision choices cannot be blank")
		}
	}
	out := Decision{ID: uid(), ProjectID: in.ProjectID, AgentID: in.AgentID, Title: in.Title, Context: in.Context, Recommendation: in.Recommendation, Choices: in.Choices, Status: "open", CreatedAt: s.now().UTC()}
	err := s.store.update(ctx, func(v *Snapshot) error {
		if in.ProjectID != "" && project(v, in.ProjectID) == nil {
			return ErrNotFound
		}
		if in.AgentID != "" {
			a := agent(v, in.AgentID)
			if a == nil || a.ProjectID != in.ProjectID {
				return errors.New("decision agent must belong to its project")
			}
		}
		v.Decisions = append(v.Decisions, out)
		record(v, out.CreatedAt, in.ProjectID, "decision.opened", in.Title)
		return nil
	})
	return out, err
}
func (s *Service) ResolveDecision(ctx context.Context, id, answer string) (Decision, error) {
	if !required(answer) {
		return Decision{}, errors.New("answer is required")
	}
	var out Decision
	err := s.store.update(ctx, func(v *Snapshot) error {
		for i := range v.Decisions {
			d := &v.Decisions[i]
			if d.ID != id {
				continue
			}
			if d.Status != "open" {
				return fmt.Errorf("decision already resolved: %w", ErrConflict)
			}
			now := s.now().UTC()
			d.Status = "resolved"
			d.Answer = answer
			d.ResolvedAt = &now
			out = *d
			record(v, now, d.ProjectID, "decision.resolved", d.Title+": "+answer)
			return nil
		}
		return ErrNotFound
	})
	return out, err
}
func (s *Service) Remember(ctx context.Context, key, value string) (Memory, error) {
	if !required(key, value) {
		return Memory{}, errors.New("memory key and content are required")
	}
	out := Memory{ID: uid(), Key: key, Content: value, UpdatedAt: s.now().UTC()}
	err := s.store.update(ctx, func(v *Snapshot) error {
		for i, m := range v.Memories {
			if m.Key == key {
				out.ID = m.ID
				v.Memories[i] = out
				record(v, out.UpdatedAt, "", "memory.updated", key)
				return nil
			}
		}
		v.Memories = append(v.Memories, out)
		record(v, out.UpdatedAt, "", "memory.created", key)
		return nil
	})
	return out, err
}
func (s *Service) Forget(ctx context.Context, id string) error {
	return s.store.update(ctx, func(v *Snapshot) error {
		for i, m := range v.Memories {
			if m.ID == id {
				v.Memories = append(v.Memories[:i], v.Memories[i+1:]...)
				record(v, s.now().UTC(), "", "memory.forgotten", m.Key)
				return nil
			}
		}
		return ErrNotFound
	})
}
func (s *Service) AddMessage(ctx context.Context, role, text string) (Message, error) {
	if !contains([]string{"user", "assistant", "system"}, role) || !required(text) {
		return Message{}, errors.New("message requires a supported role and content")
	}
	out := Message{ID: uid(), Role: role, Content: text, CreatedAt: s.now().UTC()}
	err := s.store.update(ctx, func(v *Snapshot) error { v.Messages = append(v.Messages, out); return nil })
	return out, err
}
func (s *Service) SetPaused(ctx context.Context, paused bool) error {
	return s.store.update(ctx, func(v *Snapshot) error {
		v.Paused = paused
		record(v, s.now().UTC(), "", "coordination.paused", fmt.Sprintf("Paused: %t", paused))
		return nil
	})
}

// CheckIn is a deterministic watchdog. It only marks missing workers for
// investigation, leaving diagnosis and recovery to the PA and runtime adapter.
func (s *Service) CheckIn(ctx context.Context) error {
	return s.store.update(ctx, func(v *Snapshot) error {
		now := s.now().UTC()
		for i := range v.Agents {
			a := &v.Agents[i]
			if (a.Status == "running" || a.Status == "dispatching" || a.Status == "resuming" || a.Status == "waiting") && !a.NextCheckIn.After(now) {
				a.Status = "reconciling"
				a.Summary = "Expected update missed; checking worker state before any retry"
				record(v, now, a.ProjectID, "agent.missed_check_in", a.Name)
			}
		}
		return nil
	})
}

// CompleteProject requires explicit evidence and every commissioned agent to
// finish first. A worker's optimistic status alone cannot close a project.
func (s *Service) CompleteProject(ctx context.Context, id string, evidence []string) error {
	if len(evidence) == 0 {
		return errors.New("project completion requires acceptance evidence")
	}
	for _, e := range evidence {
		if !required(e) {
			return errors.New("empty completion evidence")
		}
	}
	return s.store.update(ctx, func(v *Snapshot) error {
		p := project(v, id)
		if p == nil {
			return ErrNotFound
		}
		if p.Status == "completed" {
			return ErrConflict
		}
		for _, a := range v.Agents {
			if a.ProjectID == id && !terminal(a.Status) {
				return errors.New("project still has unfinished work")
			}
		}
		for _, d := range v.Decisions {
			if d.ProjectID == id && d.Status == "open" {
				return errors.New("project still has an unresolved decision")
			}
		}
		p.Status = "completed"
		p.UpdatedAt = s.now().UTC()
		record(v, p.UpdatedAt, id, "project.completed", strings.Join(evidence, "; "))
		return nil
	})
}

// ReserveModelCall is persisted before inference and is deliberately never
// refunded on ambiguous errors. The quota window is a UTC calendar day.
func (s *Service) ReserveModelCall(ctx context.Context, limit int) error {
	if limit < 1 {
		return errors.New("model call limit must be positive")
	}
	return s.store.update(ctx, func(v *Snapshot) error {
		day := s.now().UTC().Format("2006-01-02")
		if v.ModelCalls[day] >= limit {
			return errors.New("daily model call allowance exhausted")
		}
		v.ModelCalls[day]++
		for d := range v.ModelCalls {
			if d < day {
				delete(v.ModelCalls, d)
			}
		}
		return nil
	})
}

func executingCount(v *Snapshot) int {
	n := 0
	for _, a := range v.Agents {
		if contains([]string{"running", "dispatching", "resuming", "reconciling"}, a.Status) {
			n++
		}
	}
	return n
}
func (s *Service) RecordActivity(ctx context.Context, projectID, kind, summary string) error {
	return s.store.update(ctx, func(v *Snapshot) error { record(v, s.now().UTC(), projectID, kind, summary); return nil })
}
