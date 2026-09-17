// Package worker talks only to owner-approved execution brokers. It does not
// spawn shells or grant a broker capabilities beyond its configured profile.
package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type Config struct {
	Endpoint     string
	APIKeyEnv    string
	AuthToken    string `json:"-"` // Internal managed-broker credential, never persisted.
	Capabilities []string
	HTTPClient   *http.Client
	Timeout      time.Duration
}
type Client struct {
	cfg     Config
	http    *http.Client
	allowed map[string]bool
}

// Peer is a daemon-provided address book entry, not a grant of authority.
type Peer struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Role   string `json:"role"`
	Task   string `json:"task"`
	Status string `json:"status"`
}

type StartRequest struct {
	DelegationCapabilities []string  `json:"delegation_capabilities,omitempty"`
	DispatchKey            string    `json:"dispatch_key"`
	AgentID                string    `json:"agent_id"`
	ProjectID              string    `json:"project_id"`
	ParentID               string    `json:"parent_id,omitempty"`
	Role                   string    `json:"role"`
	Task                   string    `json:"task"`
	AcceptanceCriteria     string    `json:"acceptance_criteria"`
	Capabilities           []string  `json:"capabilities"`
	Prohibitions           []string  `json:"prohibitions"`
	CheckInDeadline        time.Time `json:"check_in_deadline"`
}
type Decision struct {
	RequestID      string   `json:"request_id"`
	Question       string   `json:"question"`
	Recommendation string   `json:"recommendation"`
	Options        []string `json:"options"`
	Why            string   `json:"why"`
	Evidence       []string `json:"evidence"`
}
type DelegationRequest struct {
	RequestID          string   `json:"request_id"`
	WorkerProfile      string   `json:"worker_profile"`
	Role               string   `json:"role"`
	Task               string   `json:"task"`
	AcceptanceCriteria string   `json:"acceptance_criteria"`
	Capabilities       []string `json:"capabilities"`
}

type Instruction struct {
	RequestID     string `json:"request_id"`
	TargetAgentID string `json:"target_agent_id"`
	Message       string `json:"message"`
}

// PeerMessage requests a daemon-routed exchange of information. It does not
// carry sender-supplied identity or coordinator authority.
type PeerMessage struct {
	RequestID     string `json:"request_id"`
	TargetAgentID string `json:"target_agent_id"`
	Message       string `json:"message"`
}

// ResourceHold explains why a broker stopped asking for inference while its
// work is preserved. It is deliberately distinct from a provider failure: no
// request was rejected, nothing is being retried, and no recovery allowance is
// spent. OwnerAction separates a wait that clears by itself, such as a known
// subscription reset, from one that needs the owner to change policy.
type ResourceHold struct {
	Kind        string    `json:"kind"`
	Reason      string    `json:"reason"`
	OwnerAction bool      `json:"owner_action,omitempty"`
	NextCheckAt time.Time `json:"next_check_at,omitempty"`
	ResetsAt    time.Time `json:"resets_at,omitempty"`
}

// Hold kinds. The first two describe a measurement problem that can resolve on
// its own, so a daemon may look again; the last two are decisions only the
// owner can make, about a configured budget or about consumption that was never
// established.
const (
	HoldSubscriptionQuota    = "subscription_quota"
	HoldTelemetryUnavailable = "telemetry_unavailable"
	HoldTokenBudget          = "token_budget"
	HoldUsageUnknown         = "usage_unknown"
)

// Recheckable reports whether a daemon may look again by itself. A hold of an
// unrecognized kind is not: an external broker's vocabulary is not permission
// to restart its work on a schedule this daemon invented.
func (h ResourceHold) Recheckable() bool {
	if h.OwnerAction {
		return false
	}
	return h.Kind == HoldSubscriptionQuota || h.Kind == HoldTelemetryUnavailable
}

// ErrResourceHold identifies a refused-before-billing decision anywhere in a
// wrapped error chain. Completion transports wrap admission errors in their own
// opaque type, so identity has to survive without unwrapping to the concrete
// hold; the broker already recorded the details before refusing.
var ErrResourceHold = errors.New("worker inference held by a resource policy")

// HoldError refuses one inference before it is made. It is returned by the
// admission callback the daemon injects into a broker, so both in-process and
// standalone brokers describe a hold identically.
type HoldError struct{ Hold ResourceHold }

func (e *HoldError) Error() string { return e.Hold.Reason }
func (e *HoldError) Unwrap() error { return ErrResourceHold }

// Usage is what a worker has actually consumed, as reported by the provider.
// UnknownCalls counts invocations whose consumption could not be established,
// including history recorded before this ledger existed; they are never
// counted as zero.
type Usage struct {
	InputTokens  int64 `json:"input_tokens,omitempty"`
	OutputTokens int64 `json:"output_tokens,omitempty"`
	UnknownCalls int   `json:"unknown_calls,omitempty"`
	TokenBudget  int64 `json:"token_budget,omitempty"`
}

type Run struct {
	ContextCompactions       int                `json:"context_compactions,omitempty"`
	ContextBytes             int                `json:"context_bytes,omitempty"`
	Usage                    Usage              `json:"usage,omitzero"`
	ResourceHold             *ResourceHold      `json:"resource_hold,omitempty"`
	RetryAt                  time.Time          `json:"retry_at,omitempty"`
	ProviderFailures         int                `json:"provider_failures,omitempty"`
	ProviderFailureKind      string             `json:"provider_failure_kind,omitempty"`
	ModelFailureEngine       string             `json:"model_failure_engine,omitempty"`
	ModelFailurePhase        string             `json:"model_failure_phase,omitempty"`
	ModelFailureCode         string             `json:"model_failure_code,omitempty"`
	ModelFailureEvidence     string             `json:"model_failure_evidence,omitempty"`
	ModelExitCode            *int               `json:"model_exit_code,omitempty"`
	ControlCapabilities      []string           `json:"control_capabilities,omitempty"`
	PauseRequested           bool               `json:"pause_requested,omitempty"`
	StopRequested            bool               `json:"stop_requested,omitempty"`
	SteeringAcknowledgements []string           `json:"steering_acknowledgements,omitempty"`
	Message                  *PeerMessage       `json:"message,omitempty"`
	Instruction              *Instruction       `json:"instruction,omitempty"`
	Delegation               *DelegationRequest `json:"delegation,omitempty"`
	ID                       string             `json:"id"`
	DispatchKey              string             `json:"dispatch_key"`
	Status                   string             `json:"status"`
	Summary                  string             `json:"summary"`
	Evidence                 []string           `json:"evidence"`
	UpdatedAt                time.Time          `json:"updated_at"`
	Decision                 *Decision          `json:"decision,omitempty"`
}

var ErrNotFound = errors.New("worker run not found")
var ErrUncertain = errors.New("worker operation outcome is uncertain; reconcile using the dispatch key before repeating")

func New(cfg Config) (*Client, error) {
	u, err := url.Parse(cfg.Endpoint)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("invalid worker endpoint")
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost" || u.Hostname() == "::1")) {
		return nil, errors.New("worker endpoint requires HTTPS except on loopback")
	}
	allowed := map[string]bool{}
	for _, cap := range cfg.Capabilities {
		switch cap {
		case "coordinate", "implement", "review", "research":
			allowed[cap] = true
		default:
			return nil, errors.New("worker profile contains a forbidden capability")
		}
	}
	if len(allowed) == 0 {
		return nil, errors.New("worker profile must declare approved capabilities")
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.Timeout < 0 {
		return nil, errors.New("worker timeout must be positive")
	}
	h := cfg.HTTPClient
	if h == nil {
		h = &http.Client{Timeout: cfg.Timeout}
	}
	copyClient := *h
	copyClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	cfg.Endpoint = strings.TrimRight(cfg.Endpoint, "/")
	return &Client{cfg: cfg, http: &copyClient, allowed: allowed}, nil
}
func (c *Client) Start(ctx context.Context, in StartRequest) (Run, error) {
	if in.DispatchKey == "" || in.AgentID == "" || in.ProjectID == "" || strings.TrimSpace(in.Task) == "" || strings.TrimSpace(in.AcceptanceCriteria) == "" {
		return Run{}, errors.New("worker start requires durable identity, task and acceptance criteria")
	}
	if in.Role != "worker" && in.Role != "manager" {
		return Run{}, errors.New("worker role must be worker or manager")
	}
	if len(in.Capabilities) == 0 {
		return Run{}, errors.New("worker start requires scoped capabilities")
	}
	for _, cap := range in.Capabilities {
		if !c.allowed[cap] {
			return Run{}, errors.New("worker start exceeds approved profile capabilities")
		}

	}
	if in.Role == "manager" {
		if !c.allowed["coordinate"] {
			return Run{}, errors.New("manager profile must permit coordination")
		}
		in.DelegationCapabilities = append([]string{}, in.Capabilities...)
		in.Capabilities = []string{"coordinate"}
	} else {
		in.DelegationCapabilities = nil
	}
	// Callers cannot override the immutable descendant prohibitions.
	in.Prohibitions = []string{"deployment", "production_data_access", "purchases"}
	result, err := c.call(ctx, http.MethodPost, "/runs", in.DispatchKey, in)
	if err == nil && result.DispatchKey != in.DispatchKey {
		return Run{}, ErrUncertain
	}
	return result, err
}
func (c *Client) Get(ctx context.Context, id string) (Run, error) {
	if id == "" {
		return Run{}, errors.New("worker run ID is required")
	}
	result, err := c.call(ctx, http.MethodGet, "/runs/"+url.PathEscape(id), "", nil)
	if err == nil && result.ID != id {
		return Run{}, errors.New("worker returned a mismatched run ID")
	}
	return result, err
}
func (c *Client) Find(ctx context.Context, dispatchKey string) (Run, error) {
	if dispatchKey == "" {
		return Run{}, errors.New("dispatch key is required")
	}
	result, err := c.call(ctx, http.MethodGet, "/runs?dispatch_key="+url.QueryEscape(dispatchKey), "", nil)
	if err == nil && result.DispatchKey != dispatchKey {
		return Run{}, errors.New("worker returned a mismatched dispatch key")
	}
	return result, err
}
func (c *Client) Resume(ctx context.Context, id, key, instruction string) (Run, error) {
	if id == "" || key == "" {
		return Run{}, errors.New("resume requires run ID and stable operation key")
	}
	return c.call(ctx, http.MethodPost, "/runs/"+url.PathEscape(id)+"/resume", key, map[string]string{"instruction": instruction})
}
func (c *Client) Send(ctx context.Context, id, key, message string) (Run, error) {
	if id == "" || key == "" || strings.TrimSpace(message) == "" {
		return Run{}, errors.New("worker message requires run ID, stable operation key and content")
	}
	return c.call(ctx, http.MethodPost, "/runs/"+url.PathEscape(id)+"/messages", key, map[string]string{"message": message})
}

func (c *Client) Pause(ctx context.Context, id, key string) (Run, error) {
	if id == "" || key == "" {
		return Run{}, errors.New("pause requires run ID and stable operation key")
	}
	return c.call(ctx, http.MethodPost, "/runs/"+url.PathEscape(id)+"/pause", key, struct{}{})
}
func (c *Client) Cancel(ctx context.Context, id, key string) (Run, error) {
	if id == "" || key == "" {
		return Run{}, errors.New("cancel requires run ID and stable operation key")
	}
	return c.call(ctx, http.MethodPost, "/runs/"+url.PathEscape(id)+"/cancel", key, struct{}{})
}
func (c *Client) call(ctx context.Context, method, path, key string, in any) (Run, error) {
	var result Run
	var body io.Reader
	if in != nil {
		raw, err := json.Marshal(in)
		if err != nil {
			return result, errors.New("cannot encode worker request")
		}
		body = bytes.NewReader(raw)
	}
	callCtx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(callCtx, method, c.cfg.Endpoint+path, body)
	if err != nil {
		return result, errors.New("cannot create worker request")
	}
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	if c.cfg.AuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.cfg.AuthToken)
	} else if c.cfg.APIKeyEnv != "" {
		token := os.Getenv(c.cfg.APIKeyEnv)
		if token == "" {
			return result, fmt.Errorf("worker credential environment variable %s is not set", c.cfg.APIKeyEnv)
		}
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		if method != http.MethodGet {
			return result, ErrUncertain
		}
		return result, errors.New("worker read failed or timed out")
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 && method == http.MethodGet {
		return result, ErrNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if method != http.MethodGet && (resp.StatusCode >= 500 || resp.StatusCode == 408) {
			return result, ErrUncertain
		}
		if method != http.MethodGet {
			if confirmedRejection(resp) {
				return result, &RejectionError{StatusCode: resp.StatusCode}
			}
			return result, ErrUncertain
		}
		return result, fmt.Errorf("worker returned HTTP %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024+1))
	if err != nil || len(raw) > 1024*1024 || json.Unmarshal(raw, &result) != nil {
		if method != http.MethodGet {
			return Run{}, ErrUncertain
		}
		return Run{}, errors.New("worker returned invalid or oversized JSON")
	}
	if result.ID == "" {
		if method != http.MethodGet {
			return Run{}, ErrUncertain
		}
		return Run{}, errors.New("worker returned no run ID")
	}
	switch result.Status {
	case "queued", "running", "waiting", "blocked", "interrupted", "completed", "failed", "cancelled", "paused", "retry_wait", "usage_wait":
	default:
		if method != http.MethodGet {
			return Run{}, ErrUncertain
		}
		return Run{}, errors.New("worker returned unknown run status")
	}
	return result, nil
}
