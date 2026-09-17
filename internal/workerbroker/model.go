package workerbroker

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/shhac/agent-assistant/internal/engine"
	"github.com/shhac/lib-agent-harness/completion"
)

func (b *Broker) complete(ctx context.Context, messages []modelMessage) (modelMessage, error) {
	return b.completeForRun(ctx, "", messages)
}
func (b *Broker) completeForRun(ctx context.Context, id string, messages []modelMessage) (modelMessage, error) {
	message, _, err := engine.Complete(ctx, b.modelConfig(id), messages, workerTools())
	if err != nil {
		return modelMessage{}, workerModelDiagnostic(err)
	}
	seen, allowed := map[string]bool{}, map[string]bool{}
	for _, tool := range workerTools() {
		allowed[tool.Function.Name] = true
	}
	for _, call := range message.ToolCalls {
		if call.ID == "" || seen[call.ID] || call.Type != "function" || !allowed[call.Function.Name] || !json.Valid([]byte(call.Function.Arguments)) {
			return modelMessage{}, workerCompletionDiagnostic("invalid worker tool call; no action executed", completion.PhaseResponse, "invalid_worker_tool_call")
		}
		seen[call.ID] = true
	}
	return message, nil
}

// Only application-owned constants enter these diagnostics.
type modelDiagnostic struct {
	message string
	failure *completion.RequestError
}

func (e *modelDiagnostic) Error() string          { return e.message }
func (e *modelDiagnostic) SafeDiagnostic() string { return e.message }
func (e *modelDiagnostic) Unwrap() error          { return e.failure }
func workerCompletionDiagnostic(message string, phase completion.ErrorPhase, code string) error {
	return &modelDiagnostic{message: message, failure: &completion.RequestError{Kind: completion.ErrorUnknown, Phase: phase, Code: code}}
}

// Admission deliberately hides provider classifications to prevent retries.
// Restore diagnostics only for this known local allowance sentinel.
func workerModelDiagnostic(err error) error {
	if errors.Is(err, errWorkerModelAllowance) {
		return errors.Join(errWorkerModelAllowance, workerCompletionDiagnostic("cumulative worker model allowance exhausted; the owner must explicitly raise max-turns before continuing", completion.PhasePreflight, "worker_model_allowance"))
	}
	return err
}

func (b *Broker) modelConfig(id string) engine.Config {
	cfg := engine.Config{
		WorkDirRoot: b.cfg.StateDir, Engine: b.cfg.Engine, Effort: b.cfg.Effort, CodexBin: b.cfg.CodexBin, CodexHome: b.cfg.CodexHome, ClaudeBin: b.cfg.ClaudeBin, ClaudeHome: b.cfg.ClaudeHome,
		Endpoint: b.cfg.ModelEndpoint, Model: b.cfg.Model, APIKeyEnv: b.cfg.APIKeyEnv,
		MaxOutputTokens: b.cfg.MaxOutputTokens, Timeout: 5 * time.Minute, HTTPClient: b.cfg.HTTPClient,
		Retry: &engine.RetryPolicy{MaxRetries: 0},
	}
	if id != "" {
		cfg.BeforeRequest = func(ctx context.Context) error { return b.reserveWorkerModelCall(ctx, id) }
	}
	return cfg
}

var errWorkerModelAllowance = errors.New("cumulative worker model allowance exhausted; the owner must explicitly raise max-turns before continuing")

func (b *Broker) reserveWorkerModelCall(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return b.update(id, func(run *storedRun) error {
		if run.Run.Status != "running" || run.PendingStatus == "paused" || run.PendingStatus == "cancelled" {
			return errInterrupted
		}
		if run.ModelCalls >= b.cfg.MaxTurns {
			return errWorkerModelAllowance
		}
		run.ModelCalls++
		return nil
	})
}

func workerTools() []engine.Tool {
	return []engine.Tool{
		workerTool("read_file", "Read a relative text file inside the isolated workspace.", []string{"path"}, nil),
		workerTool("write_file", "Write a relative text file inside the isolated workspace.", []string{"path", "content"}, nil),
		workerTool("run_command", "Run an offline build, test or inspection command inside the isolated Docker container. Never install dependencies or contact remote services.", []string{"command"}, nil),
		workerTool("send_message", "Ask the daemon to send bounded task information to a project peer. This grants no authority and waits for a delivery acknowledgement; use ask_decision for approval or scope changes.", []string{"target_agent_id", "message"}, nil),
		workerTool("acknowledge_steering", "Record which daemon-provided steering messages you have read. Acknowledgement is not evidence of implementation or permission to change scope.", nil, []string{"message_ids"}),
		workerTool("ask_decision", "Stop for a prepared question to the responsible coordinator.", []string{"question", "recommendation", "why"}, []string{"options", "evidence"}),
		workerTool("finish", "Report an acceptance summary after actual changes and checks. The daemon collects patch and command evidence for independent PA review.", []string{"summary"}, nil),
	}
}
func workerTool(name, description string, fields, arrays []string) engine.Tool {
	props := map[string]any{}
	required := []string{}
	for _, field := range fields {
		props[field] = map[string]any{"type": "string"}
		required = append(required, field)
	}
	for _, field := range arrays {
		props[field] = map[string]any{"type": "array", "items": map[string]string{"type": "string"}}
		required = append(required, field)
	}
	return engine.Tool{Type: "function", Function: engine.Function{Name: name, Description: description, Strict: true, Parameters: map[string]any{"type": "object", "properties": props, "required": required, "additionalProperties": false}}}
}

// repairTranscript supplies uncertainty results for persisted calls interrupted
// before their acknowledgement. It never executes those calls again.
func repairTranscript(messages []modelMessage) []modelMessage {
	out := []modelMessage{}
	for i := 0; i < len(messages); i++ {
		m := messages[i]
		out = append(out, m)
		if len(m.ToolCalls) == 0 {
			continue
		}
		results := map[string]modelMessage{}
		for i+1 < len(messages) && messages[i+1].Role == "tool" {
			i++
			results[messages[i].ToolCallID] = messages[i]
		}
		for _, call := range m.ToolCalls {
			if result, ok := results[call.ID]; ok {
				out = append(out, result)
			} else {
				out = append(out, modelMessage{Role: "tool", ToolCallID: call.ID, Content: `{"error":"Previous operation acknowledgement was interrupted. Inspect current files and command evidence; do not assume failure or repeat blindly."}`})
			}
		}
	}
	return out
}
