// Package diagnostics emits bounded, secret-safe daemon errors on stderr.
package diagnostics

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/shhac/lib-agent-harness/completion"
	"github.com/shhac/lib-agent-harness/session"
	output "github.com/shhac/lib-agent-output"
)

// Event contains correlation metadata, never prompts, tool arguments or output.
type Event struct {
	output.Error
	Time         time.Time `json:"time"`
	Component    string    `json:"component"`
	Stage        string    `json:"stage"`
	ProjectID    string    `json:"project_id,omitempty"`
	RunID        string    `json:"run_id,omitempty"`
	Engine       string    `json:"engine,omitempty"`
	Kind         string    `json:"kind,omitempty"`
	Phase        string    `json:"phase,omitempty"`
	Code         string    `json:"code,omitempty"`
	ExitCode     *int      `json:"exit_code,omitempty"`
	ErrorTypes   []string  `json:"error_types,omitempty"`
	ModelCalls   int       `json:"model_calls,omitempty"`
	ContextBytes int       `json:"context_bytes,omitempty"`
	// Detail carries library-sanitized harness output: bounded, control-stripped
	// and credential-redacted by the harness before it reaches here. It is the
	// only field that may contain anything a subprocess wrote, it never reaches
	// a user interface, and nothing else may be interpolated into it.
	Detail  string     `json:"detail,omitempty"`
	RetryAt *time.Time `json:"retry_at,omitempty"`
}

type Logger struct {
	mu     sync.Mutex
	writer *output.NDJSONWriter
}

func New(w io.Writer) *Logger { return &Logger{writer: output.NewNDJSONWriter(w)} }

var Default = New(os.Stderr)

// Failure logs only structural error facts and explicitly safe diagnostics.
// Arbitrary errors and subprocess stderr can echo credentials or task content;
// their text is deliberately never interpolated into a terminal event.
func (l *Logger) Failure(event Event, err error) {
	if err == nil {
		return
	}
	if l == nil {
		l = Default
	}
	event.Time = time.Now().UTC()
	event.Error = *output.New("Operation failed; inspect the stage and diagnostic code", output.FixableByHuman).WithHint("Inspect preserved worker evidence before resuming; this diagnostic does not retry work.")
	if event.Code == "" {
		// Only a default. A caller that already knows the code — a harness
		// diagnostic, say — has better information than this does.
		event.Code = "untyped_error"
	}
	var failure *completion.RequestError
	if facts, native := session.ErrorFacts(err); native {
		// A coding harness classifies its own failures, from a fixed vocabulary.
		// Discarding that and reporting "untyped_error" was losing the one thing
		// an operator could act on — an expired login reached them as nothing at
		// all. Its Error() is built from constants, so it is safe to carry.
		event.Message = err.Error()
		event.Kind, event.Phase, event.Code = facts.Kind, facts.Phase, facts.Code
		if facts.Engine != "" {
			event.Engine = facts.Engine
		}
		event.ExitCode = facts.ExitCode
	} else if errors.As(err, &failure) {
		event.Message = failure.Error()
		event.Kind, event.Phase, event.Code = string(failure.Kind), string(failure.Phase), failure.Code
		if failure.Engine != "" {
			event.Engine = failure.Engine
		}
		event.ExitCode = failure.ExitCode
		if failure.Retryable() && event.RetryAt != nil {
			event.FixableBy = output.FixableByRetry
			event.Hint = "The daemon has scheduled recovery at retry_at; no manual retry is needed."
		}
	} else {
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			event.Code = "deadline_exceeded"
		case errors.Is(err, context.Canceled):
			event.Code = "cancelled"
		case errors.Is(err, os.ErrPermission):
			event.Code = "permission_denied"
		case errors.Is(err, os.ErrNotExist), errors.Is(err, exec.ErrNotFound):
			event.Code = "not_found"
		}
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			code := exit.ExitCode()
			event.ExitCode = &code
		}
	}
	var safe interface{ SafeDiagnostic() string }
	if errors.As(err, &safe) {
		event.Message = safe.SafeDiagnostic()
	}
	// Types identify a lost wrapper boundary without copying its sensitive text.
	for cause, depth := err, 0; cause != nil && depth < 8; cause, depth = errors.Unwrap(cause), depth+1 {
		event.ErrorTypes = append(event.ErrorTypes, fmt.Sprintf("%T", cause))
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	_ = l.writer.WriteItem(event)
}
