package core

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// WorkItem preserves an outcome's contract independently of the sessions used
// to deliver it. A project can contain successive or concurrent work items.
type WorkItem struct {
	AfterWorkItemID     string            `json:"after_work_item_id,omitempty"`
	CommissionRequested bool              `json:"commission_requested,omitempty"`
	StatusReason        string            `json:"status_reason,omitempty"`
	ID                  string            `json:"id"`
	ProjectID           string            `json:"project_id"`
	Title               string            `json:"title"`
	Objective           string            `json:"objective"`
	AcceptanceCriteria  string            `json:"acceptance_criteria"`
	Status              string            `json:"status"`
	CreatedAt           time.Time         `json:"created_at"`
	UpdatedAt           time.Time         `json:"updated_at"`
	ReviewRevision      string            `json:"review_revision"`
	Acceptance          *AcceptanceRecord `json:"acceptance,omitempty"`
	// Legacy permits the old explicit CompleteProject operation to accept the
	// compatibility item. Newly created items require revision-bound acceptance.
	Legacy bool `json:"legacy,omitempty"`
}
type AcceptanceRecord struct {
	Revision   string    `json:"revision"`
	Evidence   []string  `json:"evidence"`
	Reviewer   string    `json:"reviewer"`
	AcceptedAt time.Time `json:"accepted_at"`
}
type WorkItemInput struct {
	AfterWorkItemID    string `json:"after_work_item_id,omitempty"`
	ProjectID          string `json:"project_id"`
	Title              string `json:"title"`
	Objective          string `json:"objective"`
	AcceptanceCriteria string `json:"acceptance_criteria"`
}

func workItem(v *Snapshot, id string) *WorkItem {
	for i := range v.WorkItems {
		if v.WorkItems[i].ID == id {
			return &v.WorkItems[i]
		}
	}
	return nil
}
func workItemClosed(w WorkItem) bool { return w.Status == "accepted" || w.Status == "legacy_completed" }

// CreateWorkItem records a contract without permission to commission it later.
func (s *Service) CreateWorkItem(ctx context.Context, in WorkItemInput) (WorkItem, error) {
	return s.createWorkItem(ctx, in, false)
}

func (s *Service) createWorkItem(ctx context.Context, in WorkItemInput, commissionRequested bool) (WorkItem, error) {
	if !required(in.ProjectID, in.Title, in.Objective, in.AcceptanceCriteria) {
		return WorkItem{}, errors.New("project, title, objective and measurable acceptance criteria are required")
	}
	if len(in.Title) > 500 || len(in.Objective) > 32000 || len(in.AcceptanceCriteria) > 32000 {
		return WorkItem{}, errors.New("work item contract is too long")
	}
	now := s.now().UTC()
	out := WorkItem{AfterWorkItemID: in.AfterWorkItemID, CommissionRequested: commissionRequested, ID: uid(), ProjectID: in.ProjectID, Title: strings.TrimSpace(in.Title), Objective: strings.TrimSpace(in.Objective), AcceptanceCriteria: strings.TrimSpace(in.AcceptanceCriteria), Status: "ready", CreatedAt: now, UpdatedAt: now}
	err := s.store.update(ctx, func(v *Snapshot) error {
		p := project(v, in.ProjectID)
		if p == nil {
			return ErrNotFound
		}
		if err := validateWorkItemDependency(v, &out); err != nil {
			return err
		}
		v.WorkItems = append(v.WorkItems, out)
		if p.Status == "completed" {
			p.Status = "active"
		}
		p.UpdatedAt = now
		refreshWorkItems(v)
		out = *workItem(v, out.ID)
		kind := "work_item.created"
		if commissionRequested {
			kind = "work_item.queued"
		}
		record(v, now, p.ID, kind, out.Title)
		return nil
	})
	return out, err
}

func resolveDelegationWorkItem(v *Snapshot, p *Project, id string, now time.Time) (*WorkItem, error) {
	if id != "" {
		w := workItem(v, id)
		if w == nil || w.ProjectID != p.ID {
			return nil, errors.New("work item must belong to the project")
		}
		if workItemClosed(*w) {
			return nil, errors.New("accepted work cannot receive new assignments; create another work item")
		}
		if err := workItemExecutionReady(v, w); err != nil {
			return nil, err
		}
		return w, nil
	}
	var open *WorkItem
	hasItems := false
	for i := range v.WorkItems {
		w := &v.WorkItems[i]
		if w.ProjectID != p.ID {
			continue
		}
		hasItems = true
		if workItemClosed(*w) {
			continue
		}
		if open != nil {
			return nil, errors.New("multiple open work items; choose the work item for this assignment")
		}
		open = w
	}
	if open != nil {
		if err := workItemExecutionReady(v, open); err != nil {
			return nil, err
		}
		return open, nil
	}
	if hasItems || p.Status == "completed" {
		return nil, errors.New("create a new work item before commissioning another outcome")
	}
	if !p.ContractDefined {
		return nil, errors.New("define a work item with measurable acceptance criteria before commissioning work")
	}
	w := legacyWorkItem(*p, now)
	v.WorkItems = append(v.WorkItems, w)
	return &v.WorkItems[len(v.WorkItems)-1], nil
}

func legacyWorkItem(p Project, now time.Time) WorkItem {
	objective := p.Description
	if !required(objective) {
		objective = p.Title
	}
	// The deterministic ID makes migration stable across restarts and retries.
	sum := sha256.Sum256([]byte("legacy-work-item:" + p.ID))
	return WorkItem{ID: fmt.Sprintf("legacy-%x", sum[:12]), ProjectID: p.ID, Title: p.Title, Objective: objective, AcceptanceCriteria: p.AcceptanceCriteria, Status: "active", CreatedAt: now, UpdatedAt: now, Legacy: true}
}

func migrateWorkItems(v *Snapshot) {
	for _, p := range v.Projects {
		var orphaned bool
		for _, a := range v.Agents {
			if a.ProjectID == p.ID && a.WorkItemID == "" {
				orphaned = true
				break
			}
		}
		if !orphaned {
			continue
		}
		w := legacyWorkItem(p, p.UpdatedAt)
		if p.Status == "completed" {
			w.Status = "legacy_completed"
		}
		if workItem(v, w.ID) == nil {
			v.WorkItems = append(v.WorkItems, w)
		}
		for i := range v.Agents {
			a := &v.Agents[i]
			if a.ProjectID == p.ID && a.WorkItemID == "" {
				a.WorkItemID = w.ID
			}
		}
		for i := range v.Decisions {
			d := &v.Decisions[i]
			if d.ProjectID == p.ID && d.WorkItemID == "" {
				d.WorkItemID = w.ID
			}
		}
	}
}

// Revision binds acceptance to the contract, all execution attempts, evidence,
// item-specific decisions and steering. Later project/global decisions gate new
// acceptance but cannot rewrite already accepted history. Poll timestamps do not enter
// this digest: reading unchanged progress must not invalidate a review.
func workItemRevision(v *Snapshot, w *WorkItem) string {
	type attempt struct {
		ID, ParentID, ProfileID, Task, AcceptanceCriteria, Status, Summary, ProgressFingerprint, ExternalID string
		Evidence, Capabilities                                                                              []string
	}
	type decision struct {
		ID, AgentID, Title, Context, Recommendation, Status, Answer string
		Choices                                                     []string
	}
	type steering struct{ ID, Content string }
	type receipt struct{ MessageID, AgentID string }
	payload := struct {
		ID, ProjectID, Title, Objective, AcceptanceCriteria string
		AfterWorkItemID                                     string `json:",omitempty"`
		Attempts                                            []attempt
		Decisions                                           []decision
		Steering                                            []steering
		Receipts                                            []receipt
	}{ID: w.ID, ProjectID: w.ProjectID, Title: w.Title, Objective: w.Objective, AcceptanceCriteria: w.AcceptanceCriteria, AfterWorkItemID: w.AfterWorkItemID}
	for _, a := range v.Agents {
		if a.WorkItemID == w.ID {
			payload.Attempts = append(payload.Attempts, attempt{a.ID, a.ParentID, a.ProfileID, a.Task, a.AcceptanceCriteria, a.Status, a.Summary, a.ProgressFingerprint, a.ExternalID, a.Evidence, a.Capabilities})
		}
	}
	for _, d := range v.Decisions {
		if d.WorkItemID == w.ID {
			payload.Decisions = append(payload.Decisions, decision{d.ID, d.AgentID, d.Title, d.Context, d.Recommendation, d.Status, d.Answer, d.Choices})
		}
	}
	messageIDs := map[string]bool{}
	for _, m := range v.Steering {
		if m.WorkItemID == w.ID {
			payload.Steering = append(payload.Steering, steering{m.ID, m.Content})
			messageIDs[m.ID] = true
		}
	}
	for _, r := range v.SteeringReceipts {
		if messageIDs[r.MessageID] {
			payload.Receipts = append(payload.Receipts, receipt{r.MessageID, r.AgentID})
		}
	}
	raw, _ := json.Marshal(payload)
	return fmt.Sprintf("%x", sha256.Sum256(raw))
}
func decisionAffectsWorkItem(d Decision, w WorkItem) bool {
	return d.WorkItemID == w.ID || (d.WorkItemID == "" && (d.ProjectID == "" || d.ProjectID == w.ProjectID))
}
func refreshWorkItems(v *Snapshot) {
	for i := range v.WorkItems {
		w := &v.WorkItems[i]
		w.ReviewRevision = workItemRevision(v, w)
		w.StatusReason = ""
		if w.Status == "legacy_completed" {
			continue
		}
		if w.Acceptance != nil && w.Acceptance.Revision == w.ReviewRevision {
			w.Status = "accepted"
			continue
		}
		w.Status, w.StatusReason = deriveWorkItemStatus(v, w)
	}
}

func (s *Service) AcceptWorkItem(ctx context.Context, id, revision string, evidence []string, reviewer string) (WorkItem, error) {
	if !required(id, revision, reviewer) || len(evidence) == 0 {
		return WorkItem{}, errors.New("acceptance requires work item, revision, reviewer and evidence")
	}
	if len(evidence) > 100 || len(reviewer) > 500 {
		return WorkItem{}, errors.New("acceptance record is too large")
	}
	for _, e := range evidence {
		if !required(e) || len(e) > 16000 {
			return WorkItem{}, errors.New("acceptance evidence must be nonempty and bounded")
		}
	}
	var out WorkItem
	err := s.store.update(ctx, func(v *Snapshot) error {
		w := workItem(v, id)
		if w == nil {
			return ErrNotFound
		}
		actual := workItemRevision(v, w)
		if actual != revision {
			return fmt.Errorf("work changed since review; inspect the current revision: %w", ErrConflict)
		}
		if w.Status == "accepted" && w.Acceptance != nil && w.Acceptance.Revision == revision {
			out = *w
			return nil
		}
		if w.Status == "legacy_completed" {
			return errors.New("historical project completion cannot be retroactively verified")
		}
		if err := workItemAcceptanceReady(v, w); err != nil {
			return err
		}
		now := s.now().UTC()
		w.Acceptance = &AcceptanceRecord{Revision: revision, Evidence: append([]string{}, evidence...), Reviewer: reviewer, AcceptedAt: now}
		w.Status = "accepted"
		w.StatusReason = ""
		w.ReviewRevision = revision
		w.UpdatedAt = now
		out = *w
		record(v, now, w.ProjectID, "work_item.accepted", w.Title+": "+strings.Join(evidence, "; "))
		return nil
	})
	return out, err
}
func workItemAcceptanceReady(v *Snapshot, w *WorkItem) error {
	if err := workItemExecutionReady(v, w); err != nil {
		return err
	}
	completed := map[string]bool{}
	for _, a := range v.Agents {
		if a.WorkItemID != w.ID {
			continue
		}
		if !terminal(a.Status) {
			return errors.New("work item still has unfinished execution attempts")
		}
		if a.Status == "completed" {
			if len(a.Evidence) == 0 {
				return errors.New("completed attempt has no evidence")
			}
			completed[a.ID] = true
		}
	}
	if len(completed) == 0 {
		return errors.New("acceptance requires evidence from completed commissioned work")
	}
	for _, d := range v.Decisions {
		if d.Status == "open" && decisionAffectsWorkItem(d, *w) {
			return errors.New("work item has an unresolved decision")
		}
	}
	for _, m := range v.Steering {
		if m.WorkItemID != w.ID {
			continue
		}
		acknowledged := false
		for _, r := range v.SteeringReceipts {
			if r.MessageID == m.ID && completed[r.AgentID] {
				acknowledged = true
				break
			}
		}
		if !acknowledged {
			return errors.New("steering must be acknowledged by a completed execution attempt before acceptance")
		}
	}
	return nil
}
