package core

import (
	"sort"
	"time"
)

// ProjectAttention restates what is already recorded about a project's
// execution so the owner does not have to open each project to find it. It is
// derived on read and never stored: nothing here is new policy, a new state, or
// a diagnosis. Reason text is copied verbatim from the work item or the worker
// that produced it, so the same condition is never described two ways.
type ProjectAttention struct {
	RecoveryAt        *time.Time `json:"recovery_at,omitempty"`
	LastProgressAt    *time.Time `json:"last_progress_at,omitempty"`
	ProjectID         string     `json:"project_id"`
	WorkItemID        string     `json:"work_item_id,omitempty"`
	AgentID           string     `json:"agent_id,omitempty"`
	AgentName         string     `json:"agent_name,omitempty"`
	Execution         string     `json:"execution"`
	Reason            string     `json:"reason,omitempty"`
	NextAction        string     `json:"next_action"`
	Recovery          string     `json:"recovery,omitempty"`
	OpenDecisions     int        `json:"open_decisions"`
	PendingOperations int        `json:"pending_operations"`
}

// attentionRank orders recorded states by how much they hold up the outcome. A
// live attempt does not cancel out a stopped sibling: a project with one
// running and one blocked assignment still needs the blocker looked at, which
// is why this ranks execution states rather than taking the first live one.
func attentionRank(state string) int {
	switch state {
	case "blocked":
		return 100
	case "interrupted":
		return 90
	case "reconciling":
		return 80
	case "review":
		return 70
	case "paused", "pause_requested", "stop_requested":
		return 60
	case "retry_wait":
		return 50
	case "waiting":
		return 30
	case "queued", "ready", "dispatching":
		return 20
	case "active", "running", "resuming":
		return 10
	}
	return 0
}

func attentionNextAction(state string) string {
	switch state {
	case "blocked", "interrupted", "review", "paused", "pause_requested", "stop_requested":
		return "owner"
	case "reconciling":
		return "assistant"
	case "retry_wait", "running", "active", "resuming", "dispatching":
		return "worker"
	}
	return "assistant"
}

func attentionRecovery(state string) string {
	switch state {
	case "retry_wait":
		return "scheduled"
	case "reconciling":
		return "checking"
	case "blocked", "interrupted":
		return "held"
	}
	return ""
}

// agentAttentionState normalizes a worker status into the shared vocabulary.
func agentAttentionState(status string) string {
	if terminal(status) {
		return ""
	}
	return status
}

// DeriveAttention summarizes each project's execution health, open decisions
// and unacknowledged interrupted operations. Projects with nothing recorded are
// omitted, so an empty result means nothing is known to need attention — not
// that everything succeeded.
func DeriveAttention(v Snapshot) []ProjectAttention {
	byProject := map[string]*ProjectAttention{}
	for _, p := range v.Projects {
		if p.Status == "completed" || p.Status == "cancelled" || p.Status == "archived" {
			continue
		}
		byProject[p.ID] = &ProjectAttention{ProjectID: p.ID}
	}

	best := map[string]int{}
	for _, w := range v.WorkItems {
		out := byProject[w.ProjectID]
		if out == nil || workItemClosed(w) || w.Status == "cancelled" {
			continue
		}
		rank := attentionRank(w.Status)
		if rank <= best[w.ProjectID] {
			continue
		}
		best[w.ProjectID] = rank
		out.Execution, out.Reason, out.WorkItemID = w.Status, w.StatusReason, w.ID
		out.AgentID, out.AgentName = "", ""
	}

	// Scanned separately and allowed to win: a work item reports the state of
	// its liveliest attempt, so a stopped assignment is otherwise invisible
	// beside a running one on the same outcome.
	for _, a := range v.Agents {
		out := byProject[a.ProjectID]
		state := agentAttentionState(a.Status)
		if out == nil || state == "" {
			continue
		}
		if progress := attentionProgress(a); progress != nil {
			if out.LastProgressAt == nil || progress.After(*out.LastProgressAt) {
				out.LastProgressAt = progress
			}
		}
		// At equal rank the assignment wins over the outcome that summarizes
		// it: it names the worker and carries its own recorded summary. The
		// first such assignment claims the entry so the result is stable.
		rank := attentionRank(state)
		if rank < best[a.ProjectID] || (rank == best[a.ProjectID] && out.AgentID != "") {
			continue
		}
		best[a.ProjectID] = rank
		out.Execution, out.Reason = state, a.Summary
		out.AgentID, out.AgentName = a.ID, a.Name
		if a.WorkItemID != "" {
			out.WorkItemID = a.WorkItemID
		}
		if !a.RetryAt.IsZero() && state == "retry_wait" {
			retry := a.RetryAt
			out.RecoveryAt = &retry
		}
	}

	for _, d := range v.Decisions {
		if d.Status != "open" {
			continue
		}
		if out := byProject[d.ProjectID]; out != nil {
			out.OpenDecisions++
		}
	}
	for _, op := range v.PendingOperations {
		if out := byProject[op.ProjectID]; out != nil {
			out.PendingOperations++
		}
	}

	out := make([]ProjectAttention, 0, len(byProject))
	for _, a := range byProject {
		a.NextAction = attentionNextAction(a.Execution)
		a.Recovery = attentionRecovery(a.Execution)
		// An open question or an uninspected interruption is the owner's turn
		// whatever the workers are doing.
		if a.OpenDecisions > 0 || a.PendingOperations > 0 {
			a.NextAction = "owner"
		}
		if a.Execution == "" && a.OpenDecisions == 0 && a.PendingOperations == 0 {
			continue
		}
		out = append(out, *a)
	}
	sort.Slice(out, func(i, j int) bool {
		li, lj := attentionRank(out[i].Execution), attentionRank(out[j].Execution)
		if li != lj {
			return li > lj
		}
		if out[i].OpenDecisions != out[j].OpenDecisions {
			return out[i].OpenDecisions > out[j].OpenDecisions
		}
		return out[i].ProjectID < out[j].ProjectID
	})
	return out
}

func attentionProgress(a Agent) *time.Time {
	candidate := a.LastProgressAt
	if candidate.IsZero() {
		candidate = a.LastUpdate
	}
	if candidate.IsZero() {
		return nil
	}
	return &candidate
}
