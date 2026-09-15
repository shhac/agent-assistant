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

	"github.com/shhac/agent-assistant/internal/engine"
	"github.com/shhac/agent-assistant/internal/integrations/worker"
)

type DependencyMount struct {
	Source string
	Target string
}

type Config struct {
	Dependencies []DependencyMount `json:"-"`

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
	MaxTurns        int
	MaxOutputTokens int
	MaxConcurrent   int
	HTTPClient      *http.Client
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
	PendingStatus  string              `json:"pending_status,omitempty"`
	PendingSummary string              `json:"pending_summary,omitempty"`
	Run            worker.Run          `json:"run"`
	Request        worker.StartRequest `json:"request"`
	Container      string              `json:"container"`
	WorkDir        string              `json:"work_dir"`
	Baseline       map[string][]byte   `json:"baseline"`
	Messages       []string            `json:"messages"`
	Transcript     []modelMessage      `json:"transcript"`
	Commands       []commandRecord     `json:"commands"`
	ModelCalls     int                 `json:"model_calls"`
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
