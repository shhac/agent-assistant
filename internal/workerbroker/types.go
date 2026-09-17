// Package workerbroker runs coding workers inside owner-supplied, offline Docker
// images. The PA process never receives these implementation tools.
package workerbroker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/shhac/agent-assistant/internal/diagnostics"
	"github.com/shhac/agent-assistant/internal/engine"
	"github.com/shhac/agent-assistant/internal/integrations/worker"
)

type DependencyMount struct {
	Source string
	Target string
}

type Config struct {
	Diagnostics  *diagnostics.Logger `json:"-"`
	Dependencies []DependencyMount   `json:"-"`

	StateDir        string
	Workspace       string
	ProjectID       string
	Image           string
	DockerSocket    string
	Engine          string
	Effort          string
	CodexBin        string
	CodexHome       string
	ClaudeBin       string
	ClaudeHome      string
	ModelEndpoint   string
	Model           string
	APIKeyEnv       string
	TokenEnv        string
	AuthToken       string `json:"-"` // In-process managed brokers never export credentials.
	MaxOutputTokens int
	MaxConcurrent   int
	// Admit runs before every billable inference, including context summaries
	// and recovery attempts. Returning a *worker.HoldError preserves the run's
	// work and waits; any other error stops the attempt. A nil callback permits
	// every request, which is what a broker with no configured policy does.
	Admit func(context.Context) error `json:"-"`
	// TokenBudget is read immediately before each request so a live policy
	// change applies without restarting the broker. Zero disables the budget.
	TokenBudget func() int64 `json:"-"`
	HTTPClient  *http.Client
	// Command is injectable for tests. Production uses exec.CommandContext with a
	// clean environment; it never invokes a host shell.
	Command Commander
}
type Commander interface {
	Run(context.Context, []string, []byte) ([]byte, error)
}
type CommandFunc func(context.Context, []string, []byte) ([]byte, error)

func (f CommandFunc) Run(ctx context.Context, args []string, in []byte) ([]byte, error) {
	return f(ctx, args, in)
}

type storedRun struct {
	// Transcript remains the complete durable archive. WorkingContext represents
	// only its first ContextThrough messages; later transcript entries append to it.
	WorkingContext     []modelMessage             `json:"working_context,omitempty"`
	ContextThrough     int                        `json:"context_through,omitempty"`
	ContextDigest      string                     `json:"context_digest,omitempty"`
	ContextCheckpoints []engine.ContextCheckpoint `json:"context_checkpoints,omitempty"`
	PendingMessage     *worker.PeerMessage        `json:"pending_message,omitempty"`
	PendingStatus      string                     `json:"pending_status,omitempty"`
	PendingSummary     string                     `json:"pending_summary,omitempty"`
	Run                worker.Run                 `json:"run"`
	Request            worker.StartRequest        `json:"request"`
	Container          string                     `json:"container"`
	WorkDir            string                     `json:"work_dir"`
	Baseline           map[string][]byte          `json:"baseline"`
	Messages           []string                   `json:"messages"`
	Transcript         []modelMessage             `json:"transcript"`
	Commands           []commandRecord            `json:"commands"`
	// ModelCalls is diagnostic history. It was once a work allowance; it is not
	// one now, and a resume never resets it.
	ModelCalls int `json:"model_calls"`
	// PendingUsage is written after admission and before the request leaves, so
	// a crash cannot turn a possibly-billed call into free work. It is settled
	// by request ID, which makes a late or duplicate settlement a no-op.
	PendingUsage *pendingUsage `json:"pending_usage,omitempty"`
	// UsageLedger marks that consumption has been accounted since this run
	// started recording it. A run persisted before the ledger existed converts
	// its historical calls into unknown consumption exactly once.
	UsageLedger       bool  `json:"usage_ledger,omitempty"`
	UsageInputTokens  int64 `json:"usage_input_tokens,omitempty"`
	UsageOutputTokens int64 `json:"usage_output_tokens,omitempty"`
	UsageUnknownCalls int   `json:"usage_unknown_calls,omitempty"`
	// FailingTurns counts consecutive turns in which every requested operation
	// failed. That is observable absence of progress, not a call ceiling.
	FailingTurns int `json:"failing_turns,omitempty"`
}
type pendingUsage struct {
	RequestID string    `json:"request_id"`
	Stage     string    `json:"stage"`
	StartedAt time.Time `json:"started_at"`
}
type commandRecord struct {
	Command string `json:"command"`
	Success bool   `json:"success"`
	Output  string `json:"output"`
}
type receipt struct {
	Digest string `json:"digest"`
	RunID  string `json:"run_id"`
}
type diskState struct {
	Runs     map[string]*storedRun `json:"runs"`
	Receipts map[string]receipt    `json:"receipts"`
}
type modelMessage = engine.Message
type toolCall = engine.ToolCall

var ErrConflict = errors.New("worker operation conflicts with its persisted state")
var errInterrupted = errors.New("worker interrupted before acceptance; inspect evidence before resuming")

func strict(raw []byte, out any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		return errors.New("invalid request JSON")
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return errors.New("request must contain one JSON object")
	}
	return nil
}

// Bytes reader is kept here only to share strict decoding across model tools and HTTP.
func now() time.Time { return time.Now().UTC() }
