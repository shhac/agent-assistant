// Package core owns durable coordination records and deterministic authority checks.
package core

import "time"

type Assistant struct {
	Name        string `json:"name"`
	Personality string `json:"personality"`
}
type Project struct {
	Directories        []string  `json:"directories"`
	ScratchDirectory   string    `json:"scratch_directory"`
	ContractDefined    bool      `json:"contract_defined"`
	SourceDescription  string    `json:"source_description,omitempty"`
	ID                 string    `json:"id"`
	Title              string    `json:"title"`
	Description        string    `json:"description"`
	AcceptanceCriteria string    `json:"acceptance_criteria"`
	Status             string    `json:"status"`
	SourceID           string    `json:"source_id,omitempty"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// Agent is a durable, scoped assignment owned by the daemon, independent of
// the assistant model turn that commissioned it. Role describes responsibility.
// ParentID retains the stored/wire name for its authority and escalation link;
// it is not subprocess ownership or a restriction on peer communication.
type Agent struct {
	WorkItemID          string    `json:"work_item_id,omitempty"`
	LastProgressAt      time.Time `json:"last_progress_at,omitempty"`
	ProgressFingerprint string    `json:"progress_fingerprint,omitempty"`
	BrokerUpdatedAt     time.Time `json:"broker_updated_at,omitempty"`
	ResumeKey           string    `json:"resume_key,omitempty"`
	ID                  string    `json:"id"`
	ProjectID           string    `json:"project_id"`
	ParentID            string    `json:"parent_id,omitempty"`
	ProfileID           string    `json:"profile_id"`
	Name                string    `json:"name"`
	Role                string    `json:"role"`
	Status              string    `json:"status"`
	Task                string    `json:"task"`
	AcceptanceCriteria  string    `json:"acceptance_criteria"`
	Capabilities        []string  `json:"capabilities"`
	ExternalID          string    `json:"external_id,omitempty"`
	DispatchKey         string    `json:"dispatch_key"`
	Depth               int       `json:"depth"`
	Recoveries          int       `json:"recoveries"`
	LastUpdate          time.Time `json:"last_update"`
	NextCheckIn         time.Time `json:"next_check_in"`
	Summary             string    `json:"summary"`
	Evidence            []string  `json:"evidence"`
}
type Decision struct {
	WorkItemID     string     `json:"work_item_id,omitempty"`
	ID             string     `json:"id"`
	ProjectID      string     `json:"project_id,omitempty"`
	AgentID        string     `json:"agent_id,omitempty"`
	Title          string     `json:"title"`
	Context        string     `json:"context"`
	Recommendation string     `json:"recommendation"`
	Choices        []string   `json:"choices"`
	Status         string     `json:"status"`
	Answer         string     `json:"answer,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	ResolvedAt     *time.Time `json:"resolved_at,omitempty"`
}
type Message struct {
	ID        string    `json:"id"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}
type Memory struct {
	ID        string    `json:"id"`
	Key       string    `json:"key"`
	Content   string    `json:"content"`
	UpdatedAt time.Time `json:"updated_at"`
}
type Activity struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id,omitempty"`
	Kind      string    `json:"kind"`
	Summary   string    `json:"summary"`
	CreatedAt time.Time `json:"created_at"`
}
type Integration struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}
type PendingOperation struct {
	ID        string `json:"id"`
	Summary   string `json:"summary"`
	ProjectID string `json:"project_id,omitempty"`
}

type Snapshot struct {
	WorkItems         []WorkItem         `json:"work_items"`
	Steering          []SteeringMessage  `json:"steering"`
	SteeringReceipts  []SteeringReceipt  `json:"steering_receipts"`
	ChatTurns         []ChatTurn         `json:"-"`
	PendingOperations []PendingOperation `json:"pending_operations"`
	Events            map[string]bool    `json:"-"`
	Assistant         Assistant          `json:"assistant"`
	Projects          []Project          `json:"projects"`
	Agents            []Agent            `json:"agents"`
	Decisions         []Decision         `json:"decisions"`
	Messages          []Message          `json:"messages"`
	Memories          []Memory           `json:"memories"`
	Activity          []Activity         `json:"activity"`
	Integrations      []Integration      `json:"integrations"`
	Paused            bool               `json:"paused"`
	ModelCalls        map[string]int     `json:"-"`
}
type ProjectInput struct {
	Directories        []string `json:"directories"`
	Title              string   `json:"title"`
	Description        string   `json:"description"`
	AcceptanceCriteria string   `json:"acceptance_criteria"`
	SourceID           string   `json:"source_id,omitempty"`
}

// DelegateInput requests a daemon-owned assignment. ParentID selects the
// responsible coordinator and constrains delegated capabilities.
type DelegateInput struct {
	WorkItemID         string   `json:"work_item_id,omitempty"`
	ProjectID          string   `json:"project_id"`
	ParentID           string   `json:"parent_id,omitempty"`
	ProfileID          string   `json:"profile_id"`
	Name               string   `json:"name"`
	Role               string   `json:"role"`
	Task               string   `json:"task"`
	AcceptanceCriteria string   `json:"acceptance_criteria"`
	Capabilities       []string `json:"capabilities"`
}
type AgentUpdate struct {
	UpdatedAt  time.Time `json:"updated_at,omitempty"`
	Status     string    `json:"status"`
	Summary    string    `json:"summary"`
	Evidence   []string  `json:"evidence"`
	ExternalID string    `json:"external_id,omitempty"`
}
type DecisionInput struct {
	WorkItemID     string   `json:"work_item_id,omitempty"`
	ProjectID      string   `json:"project_id,omitempty"`
	AgentID        string   `json:"agent_id,omitempty"`
	Title          string   `json:"title"`
	Context        string   `json:"context"`
	Recommendation string   `json:"recommendation"`
	Choices        []string `json:"choices"`
}
