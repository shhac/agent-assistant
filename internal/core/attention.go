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

// attentionIndex accumulates one row per open project while the snapshot is
// scanned, remembering how strong a claim has been made on each row so a later
// scan can only improve it.
type attentionIndex struct {
	rows map[string]*ProjectAttention
	rank map[string]int
}

func openProjects(v Snapshot) attentionIndex {
	index := attentionIndex{rows: map[string]*ProjectAttention{}, rank: map[string]int{}}
	for _, p := range v.Projects {
		if p.Status == "completed" || p.Status == "cancelled" || p.Status == "archived" {
			continue
		}
		index.rows[p.ID] = &ProjectAttention{ProjectID: p.ID}
	}
	return index
}

func (index attentionIndex) applyWorkItems(v Snapshot) {
	for _, w := range v.WorkItems {
		row := index.rows[w.ProjectID]
		if row == nil || workItemClosed(w) || w.Status == "cancelled" {
			continue
		}
		rank := attentionRank(w.Status)
		if rank <= index.rank[w.ProjectID] {
			continue
		}
		index.rank[w.ProjectID] = rank
		row.Execution, row.Reason, row.WorkItemID = w.Status, w.StatusReason, w.ID
		row.AgentID, row.AgentName = "", ""
	}
}

// applyAgents scans assignments in their own right. An outcome reports the
// state of its liveliest attempt, so a stopped assignment beside a running one
// is otherwise invisible.
func (index attentionIndex) applyAgents(v Snapshot) {
	for _, a := range v.Agents {
		row := index.rows[a.ProjectID]
		state := agentAttentionState(a.Status)
		if row == nil || state == "" {
			continue
		}
		if progress := attentionProgress(a); progress != nil {
			if row.LastProgressAt == nil || progress.After(*row.LastProgressAt) {
				row.LastProgressAt = progress
			}
		}
		if !assignmentClaims(attentionRank(state), index.rank[a.ProjectID], row.AgentID != "") {
			continue
		}
		index.rank[a.ProjectID] = attentionRank(state)
		row.Execution, row.Reason = state, a.Summary
		row.AgentID, row.AgentName = a.ID, a.Name
		if a.WorkItemID != "" {
			row.WorkItemID = a.WorkItemID
		}
		if !a.RetryAt.IsZero() && state == "retry_wait" {
			retry := a.RetryAt
			row.RecoveryAt = &retry
		}
	}
}

// assignmentClaims decides whether an assignment may take over a row. At equal
// rank it wins over the outcome that summarizes it, because it names the worker
// and carries its own recorded summary — but the first such assignment keeps
// the row, so repeated scans of one snapshot agree.
func assignmentClaims(rank, best int, alreadyClaimed bool) bool {
	if rank < best {
		return false
	}
	return rank > best || !alreadyClaimed
}

func (index attentionIndex) applyOwnerQueues(v Snapshot) {
	for _, d := range v.Decisions {
		if d.Status != "open" {
			continue
		}
		if row := index.rows[d.ProjectID]; row != nil {
			row.OpenDecisions++
		}
	}
	for _, op := range v.PendingOperations {
		if row := index.rows[op.ProjectID]; row != nil {
			row.PendingOperations++
		}
	}
}

// finalize turns the accumulated rows into the reported order, dropping
// projects with nothing recorded.
func (index attentionIndex) finalize() []ProjectAttention {
	out := make([]ProjectAttention, 0, len(index.rows))
	for _, row := range index.rows {
		row.NextAction = attentionNextAction(row.Execution)
		row.Recovery = attentionRecovery(row.Execution)
		// An open question or an uninspected interruption is the owner's turn
		// whatever the workers are doing.
		if row.OpenDecisions > 0 || row.PendingOperations > 0 {
			row.NextAction = "owner"
		}
		if row.Execution == "" && row.OpenDecisions == 0 && row.PendingOperations == 0 {
			continue
		}
		out = append(out, *row)
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

// DeriveAttention summarizes each project's execution health, open decisions
// and unacknowledged interrupted operations. Projects with nothing recorded are
// omitted, so an empty result means nothing is known to need attention — not
// that everything succeeded.
func DeriveAttention(v Snapshot) []ProjectAttention {
	index := openProjects(v)
	index.applyWorkItems(v)
	index.applyAgents(v)
	index.applyOwnerQueues(v)
	return index.finalize()
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
