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
	// ContextCompactions counts the coding session's own compactions. There is no
	// byte figure beside it: the daemon does not rewrite a worker's conversation,
	// and how full that conversation is belongs to the worker's own report rather
	// than to an application budget.
	ContextCompactions int `json:"context_compactions,omitempty"`
	// ContextUsedPercent is conversation occupancy, never consumption. Quality
	// separates a provider measurement from a local estimate; an empty quality
	// means occupancy was not established.
	ContextUsedPercent   *float64 `json:"context_used_percent,omitempty"`
	ContextQuality       string   `json:"context_quality,omitempty"`
	SessionEngine        string   `json:"session_engine,omitempty"`
	SessionResumed       bool     `json:"session_resumed,omitempty"`
	ObservedInputTokens  int64    `json:"observed_input_tokens,omitempty"`
	ObservedOutputTokens int64    `json:"observed_output_tokens,omitempty"`
	// Work is the worker's own recent activity: which tools it called, how its
	// turns ended, what was refused. It is how an owner can see a long-running
	// worker doing something rather than only its latest one-line summary.
	Work []AgentWork `json:"work,omitempty"`
	// Resource-hold fields describe work waiting on a budget or an account
	// allowance. OwnerAction separates a wait that clears by itself from one
	// that needs a policy decision. None of this is a failure classification.
	ResourceHoldKind        string    `json:"resource_hold_kind,omitempty"`
	ResourceHoldOwnerAction bool      `json:"resource_hold_owner_action,omitempty"`
	ResourceHoldResetsAt    time.Time `json:"resource_hold_resets_at,omitempty"`
	UsageInputTokens        int64     `json:"usage_input_tokens,omitempty"`
	UsageOutputTokens       int64     `json:"usage_output_tokens,omitempty"`
	UsageUnknownCalls       int       `json:"usage_unknown_calls,omitempty"`
	TokenBudget             int64     `json:"token_budget,omitempty"`
	RetryAt                 time.Time `json:"retry_at,omitempty"`
	ProviderFailures        int       `json:"provider_failures,omitempty"`
	ProviderFailureKind     string    `json:"provider_failure_kind,omitempty"`
	ModelFailureEngine      string    `json:"model_failure_engine,omitempty"`
	ModelFailurePhase       string    `json:"model_failure_phase,omitempty"`
	ModelFailureCode        string    `json:"model_failure_code,omitempty"`
	ModelFailureEvidence    string    `json:"model_failure_evidence,omitempty"`
	ModelExitCode           *int      `json:"model_exit_code,omitempty"`
	OwnerControl            string    `json:"owner_control,omitempty"`
	ControlKey              string    `json:"control_key,omitempty"`
	ControlCapabilities     []string  `json:"control_capabilities,omitempty"`
	WorkItemID              string    `json:"work_item_id,omitempty"`
	LastProgressAt          time.Time `json:"last_progress_at,omitempty"`
	ProgressFingerprint     string    `json:"progress_fingerprint,omitempty"`
	BrokerUpdatedAt         time.Time `json:"broker_updated_at,omitempty"`
	ResumeKey               string    `json:"resume_key,omitempty"`
	ID                      string    `json:"id"`
	ProjectID               string    `json:"project_id"`
	ParentID                string    `json:"parent_id,omitempty"`
	ProfileID               string    `json:"profile_id"`
	Name                    string    `json:"name"`
	Role                    string    `json:"role"`
	Status                  string    `json:"status"`
	Task                    string    `json:"task"`
	AcceptanceCriteria      string    `json:"acceptance_criteria"`
	Capabilities            []string  `json:"capabilities"`
	ExternalID              string    `json:"external_id,omitempty"`
	DispatchKey             string    `json:"dispatch_key"`
	Depth                   int       `json:"depth"`
	Recoveries              int       `json:"recoveries"`
	LastUpdate              time.Time `json:"last_update"`
	NextCheckIn             time.Time `json:"next_check_in"`
	Summary                 string    `json:"summary"`
	Evidence                []string  `json:"evidence"`
}
type Decision struct {
	Disposition      string     `json:"disposition,omitempty"`
	ResolutionReason string     `json:"resolution_reason,omitempty"`
	WorkItemID       string     `json:"work_item_id,omitempty"`
	ID               string     `json:"id"`
	ProjectID        string     `json:"project_id,omitempty"`
	AgentID          string     `json:"agent_id,omitempty"`
	Title            string     `json:"title"`
	Context          string     `json:"context"`
	Recommendation   string     `json:"recommendation"`
	Choices          []string   `json:"choices"`
	Status           string     `json:"status"`
	Answer           string     `json:"answer,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	ResolvedAt       *time.Time `json:"resolved_at,omitempty"`
}
type Message struct {
	ID        string    `json:"id"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

// Memory separates a durable preference from a time-sensitive observation so a
// stale fact is not read as a standing instruction. Kind and Source are empty
// for anything recorded before they existed; that is reported as uncategorized
// rather than guessed at. Correcting a memory supersedes it and keeps the
// original, so the record of what was believed is never silently rewritten.
type Memory struct {
	ID           string    `json:"id"`
	Key          string    `json:"key"`
	Content      string    `json:"content"`
	Kind         string    `json:"kind,omitempty"`
	Source       string    `json:"source,omitempty"`
	Supersedes   string    `json:"supersedes,omitempty"`
	SupersededAt time.Time `json:"superseded_at,omitempty"`
	UpdatedAt    time.Time `json:"updated_at"`
}
type Activity struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id,omitempty"`
	Kind      string    `json:"kind"`
	Summary   string    `json:"summary"`
	CreatedAt time.Time `json:"created_at"`
}

// AgentWork is one observation of what a worker did, as the daemon recorded it.
// Detail is bounded and sanitized by the daemon; nothing here is a claim that
// the work was correct, and none of it is the evidence acceptance rests on.
//
// Truncated marks the oldest retained entry when earlier ones were dropped, so
// a long assignment's feed does not read as if it began where the record does.
type AgentWork struct {
	At        time.Time `json:"at"`
	Kind      string    `json:"kind"`
	Tool      string    `json:"tool,omitempty"`
	Status    string    `json:"status,omitempty"`
	Detail    string    `json:"detail,omitempty"`
	Truncated bool      `json:"truncated,omitempty"`
}
type Integration struct {
	ID string `json:"id"`
	// ProjectID links a connection to the project it serves, where one does.
	// The dashboard shows a capacity hold beside the work it holds up, and
	// that relationship is the daemon's to state rather than the browser's to
	// reassemble from an identifier convention.
	ProjectID string `json:"project_id,omitempty"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	Detail    string `json:"detail,omitempty"`
}
type PendingOperation struct {
	ID        string `json:"id"`
	Summary   string `json:"summary"`
	ProjectID string `json:"project_id,omitempty"`
}

type Snapshot struct {
	ChatCheckpoint       ChatCheckpoint           `json:"-"`
	ConversationDropped  map[string]bool          `json:"-"`
	AgentConversation    []AgentConversationEntry `json:"-"`
	ConversationSequence int64                    `json:"-"`
	WorkItems            []WorkItem               `json:"work_items"`
	Steering             []SteeringMessage        `json:"steering"`
	SteeringReceipts     []SteeringReceipt        `json:"steering_receipts"`
	ChatTurns            []ChatTurn               `json:"-"`
	ChatHold             *ChatHold                `json:"-"`
	ChatQueueRevision    int                      `json:"-"`
	PendingOperations    []PendingOperation       `json:"pending_operations"`
	Events               map[string]bool          `json:"-"`
	Assistant            Assistant                `json:"assistant"`
	Projects             []Project                `json:"projects"`
	Agents               []Agent                  `json:"agents"`
	Decisions            []Decision               `json:"decisions"`
	Messages             []Message                `json:"messages"`
	Memories             []Memory                 `json:"memories"`
	Activity             []Activity               `json:"activity"`
	Integrations         []Integration            `json:"integrations"`
	Paused               bool                     `json:"paused"`
	ModelCalls           map[string]int           `json:"-"`
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
	// RequireCommissionRequest is set by the daemon queue dispatcher, never by model JSON.
	RequireCommissionRequest bool     `json:"-"`
	WorkItemID               string   `json:"work_item_id,omitempty"`
	ProjectID                string   `json:"project_id"`
	ParentID                 string   `json:"parent_id,omitempty"`
	ProfileID                string   `json:"profile_id"`
	Name                     string   `json:"name"`
	Role                     string   `json:"role"`
	Task                     string   `json:"task"`
	AcceptanceCriteria       string   `json:"acceptance_criteria"`
	Capabilities             []string `json:"capabilities"`
}
type AgentUpdate struct {
	ContextCompactions      int       `json:"context_compactions,omitempty"`
	ContextUsedPercent      *float64  `json:"context_used_percent,omitempty"`
	ContextQuality          string    `json:"context_quality,omitempty"`
	SessionEngine           string    `json:"session_engine,omitempty"`
	SessionResumed          bool      `json:"session_resumed,omitempty"`
	ObservedInputTokens     int64     `json:"observed_input_tokens,omitempty"`
	ObservedOutputTokens    int64     `json:"observed_output_tokens,omitempty"`
	ResourceHoldKind        string    `json:"resource_hold_kind,omitempty"`
	ResourceHoldOwnerAction bool      `json:"resource_hold_owner_action,omitempty"`
	ResourceHoldResetsAt    time.Time `json:"resource_hold_resets_at,omitempty"`
	UsageInputTokens        int64     `json:"usage_input_tokens,omitempty"`
	UsageOutputTokens       int64     `json:"usage_output_tokens,omitempty"`
	UsageUnknownCalls       int       `json:"usage_unknown_calls,omitempty"`
	TokenBudget             int64     `json:"token_budget,omitempty"`
	RetryAt                 time.Time `json:"retry_at,omitempty"`
	ProviderFailures        int       `json:"provider_failures,omitempty"`
	ProviderFailureKind     string    `json:"provider_failure_kind,omitempty"`
	ModelFailureEngine      string    `json:"model_failure_engine,omitempty"`
	ModelFailurePhase       string    `json:"model_failure_phase,omitempty"`
	ModelFailureCode        string    `json:"model_failure_code,omitempty"`
	ModelFailureEvidence    string    `json:"model_failure_evidence,omitempty"`
	ModelExitCode           *int      `json:"model_exit_code,omitempty"`
	UpdatedAt               time.Time `json:"updated_at,omitempty"`
	Status                  string    `json:"status"`
	Summary                 string    `json:"summary"`
	Evidence                []string  `json:"evidence"`
	ExternalID              string    `json:"external_id,omitempty"`
	// Work is what the worker was observed doing. It is a readable record, not
	// the evidence acceptance rests on, and it is bounded by the daemon before
	// it gets here.
	Work []AgentWork `json:"work,omitempty"`
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
