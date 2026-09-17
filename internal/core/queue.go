package core

import (
	"context"
	"errors"
	"fmt"
)

// QueueWorkItem saves the owner's request to commission this outcome after its
// predecessor is accepted. Merely creating a contract never grants this intent.
// The PA still selects assignments within its authority; no worker is launched here.
func (s *Service) QueueWorkItem(ctx context.Context, in WorkItemInput) (WorkItem, error) {
	if !required(in.AfterWorkItemID) {
		return WorkItem{}, errors.New("queued work requires a preceding work item")
	}
	return s.createWorkItem(ctx, in, true)
}

// CancelQueuedWorkItem withdraws only the saved permission to commission an
// unstarted outcome. Its contract and ordering remain available as a draft.
// Once an assignment exists, the explicit worker controls own stopping it.
func (s *Service) CancelQueuedWorkItem(ctx context.Context, id string) (WorkItem, error) {
	var out WorkItem
	err := s.store.update(ctx, func(v *Snapshot) error {
		w := workItem(v, id)
		if w == nil {
			return ErrNotFound
		}
		for _, a := range v.Agents {
			if a.WorkItemID == id {
				return fmt.Errorf("work has an assignment; use worker controls instead: %w", ErrConflict)
			}
		}
		if w.CommissionRequested {
			w.CommissionRequested = false
			w.UpdatedAt = s.now().UTC()
			record(v, w.UpdatedAt, w.ProjectID, "work_item.unqueued", w.Title+": commissioning request withdrawn")
		}
		refreshWorkItems(v)
		out = *workItem(v, id)
		return nil
	})
	return out, err
}

// ReadyQueuedWorkItems returns durable, explicitly requested outcomes whose
// prerequisites have completed. Existing attempts are never repeated, including
// cancelled or interrupted attempts: those require separate recovery decisions.
func (s *Service) ReadyQueuedWorkItems(ctx context.Context) ([]WorkItem, error) {
	v, err := s.store.Snapshot(ctx)
	if err != nil {
		return nil, err
	}
	out := []WorkItem{}
	if v.Paused {
		return out, nil
	}
	for _, w := range v.WorkItems {
		if !w.CommissionRequested || workItemClosed(w) || workItemExecutionReady(&v, &w) != nil {
			continue
		}
		hasAttempt := false
		for _, a := range v.Agents {
			if a.WorkItemID == w.ID {
				hasAttempt = true
				break
			}
		}
		if hasAttempt {
			continue
		}
		blocked := false
		for _, d := range v.Decisions {
			if d.Status == "open" && decisionAffectsWorkItem(d, w) {
				blocked = true
				break
			}
		}
		if !blocked {
			out = append(out, w)
		}
	}
	return out, nil
}

func validateWorkItemDependency(v *Snapshot, w *WorkItem) error {
	seen := map[string]bool{w.ID: true}
	for id := w.AfterWorkItemID; id != ""; {
		if seen[id] {
			return errors.New("work item ordering cannot contain a cycle")
		}
		seen[id] = true
		predecessor := workItem(v, id)
		if predecessor == nil {
			return fmt.Errorf("preceding work item: %w", ErrNotFound)
		}
		if predecessor.ProjectID != w.ProjectID {
			return errors.New("preceding work item must belong to the same project")
		}
		id = predecessor.AfterWorkItemID
	}
	return nil
}

// Every execution entry point checks this guard, including resumed sessions.
// Missing or malformed dependencies fail closed rather than appearing ready.
func workItemExecutionReady(v *Snapshot, w *WorkItem) error {
	if err := validateWorkItemDependency(v, w); err != nil {
		return err
	}
	for id := w.AfterWorkItemID; id != ""; {
		predecessor := workItem(v, id)
		accepted := predecessor.Status == "legacy_completed" || (predecessor.Acceptance != nil && predecessor.Acceptance.Revision == workItemRevision(v, predecessor))
		if !accepted {
			return fmt.Errorf("waiting for acceptance of %q", predecessor.Title)
		}
		id = predecessor.AfterWorkItemID
	}
	return nil
}

// deriveWorkItemStatus summarizes actual execution evidence. A waiting outcome
// never looks active merely because it was recorded. A live attempt takes
// precedence over a stopped sibling; the dashboard can show each attempt too.
func deriveWorkItemStatus(v *Snapshot, w *WorkItem) (string, string) {
	if err := workItemExecutionReady(v, w); err != nil {
		if !w.CommissionRequested {
			hasAttempt := false
			for _, a := range v.Agents {
				if a.WorkItemID == w.ID {
					hasAttempt = true
					break
				}
			}
			if !hasAttempt {
				return "waiting", "Not queued; " + err.Error()
			}
		}
		return "queued", err.Error()
	}
	statuses := map[string]int{}
	total, completed, finished := 0, 0, 0
	reason := map[string]string{}
	for _, a := range v.Agents {
		if a.WorkItemID != w.ID {
			continue
		}
		total++
		statuses[a.Status]++
		if reason[a.Status] == "" {
			reason[a.Status] = a.Summary
		}
		if terminal(a.Status) {
			finished++
		}
		if a.Status == "completed" {
			completed++
		}
	}
	if total == 0 {
		if v.Paused && w.CommissionRequested {
			return "paused", "New work is paused; this outcome remains queued"
		}
		for _, d := range v.Decisions {
			if d.Status == "open" && decisionAffectsWorkItem(d, *w) {
				return "blocked", "Waiting for a decision: " + d.Title
			}
		}
		if w.CommissionRequested {
			return "ready", "Preceding work accepted; waiting for the assistant to commission this outcome"
		}
		return "ready", "Ready to commission when requested"
	}
	if finished == total {
		if completed > 0 {
			return "review", "Execution finished; acceptance review is required"
		}
		return "cancelled", "All execution attempts were stopped; this outcome has not been completed"
	}
	if statuses["pause_requested"] > 0 {
		return "active", "Pausing execution; waiting for the worker to confirm"
	}
	if statuses["stop_requested"] > 0 {
		return "active", "Stopping execution; waiting for the worker to confirm"
	}
	if statuses["running"]+statuses["dispatching"]+statuses["resuming"] > 0 {
		return "active", "Work is running"
	}
	if statuses["retry_wait"] > 0 {
		return "waiting", nonemptyStatusReason(reason["retry_wait"], "Waiting for the model provider; retry is scheduled")
	}
	if statuses["usage_wait"] > 0 {
		return "waiting", nonemptyStatusReason(reason["usage_wait"], "Waiting for worker resources to become available")
	}
	if statuses["reconciling"] > 0 {
		return "waiting", "Checking worker state before any retry"
	}
	if statuses["paused"] > 0 {
		return "paused", "Execution is paused by the owner"
	}
	if statuses["interrupted"] > 0 {
		return "interrupted", nonemptyStatusReason(reason["interrupted"], "Execution was interrupted; recovery needs attention")
	}
	if statuses["blocked"] > 0 {
		return "blocked", nonemptyStatusReason(reason["blocked"], "Execution is blocked")
	}
	if v.Paused {
		return "paused", "New work is paused"
	}
	if statuses["queued"] > 0 {
		return "waiting", "An assignment is queued; execution has not started"
	}
	if statuses["waiting"] > 0 {
		return "waiting", nonemptyStatusReason(reason["waiting"], "Waiting for the next coordination action")
	}
	return "waiting", "Waiting for an execution update"
}
func nonemptyStatusReason(value, fallback string) string {
	if required(value) {
		return value
	}
	return fallback
}
