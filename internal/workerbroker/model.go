package workerbroker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

func (b *Broker) complete(ctx context.Context, messages []modelMessage) (modelMessage, error) {
	body, _ := json.Marshal(map[string]any{"model": b.cfg.Model, "messages": messages, "tools": workerTools(), "parallel_tool_calls": false, "tool_choice": "required", "max_completion_tokens": b.cfg.MaxOutputTokens})
	callCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(callCtx, http.MethodPost, b.cfg.ModelEndpoint, bytes.NewReader(body))
	if err != nil {
		return modelMessage{}, errors.New("cannot create worker model request")
	}
	req.Header.Set("Content-Type", "application/json")
	if b.cfg.APIKeyEnv != "" {
		key := os.Getenv(b.cfg.APIKeyEnv)
		if key == "" {
			return modelMessage{}, errors.New("worker model credential is unavailable")
		}
		req.Header.Set("Authorization", "Bearer "+key)
	}
	client := b.cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 90 * time.Second}
	}
	copyClient := *client
	copyClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := copyClient.Do(req)
	if err != nil {
		return modelMessage{}, errors.New("worker model request failed or timed out; usage is uncertain and the request was not retried")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return modelMessage{}, fmt.Errorf("worker model returned HTTP %d; request was not retried", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024+1))
	if err != nil || len(raw) > 2*1024*1024 {
		return modelMessage{}, errors.New("worker model response exceeded limit")
	}
	var result struct {
		Choices []struct {
			Message      modelMessage `json:"message"`
			FinishReason string       `json:"finish_reason"`
		} `json:"choices"`
	}
	if json.Unmarshal(raw, &result) != nil || len(result.Choices) != 1 || result.Choices[0].Message.Role != "assistant" {
		return modelMessage{}, errors.New("invalid worker model response")
	}
	if result.Choices[0].FinishReason == "length" {
		return modelMessage{}, errors.New("worker model output allowance exhausted; no partial action executed")
	}
	message := result.Choices[0].Message
	seen := map[string]bool{}
	for _, call := range message.ToolCalls {
		if call.ID == "" || seen[call.ID] || call.Type != "function" || !json.Valid([]byte(call.Function.Arguments)) {
			return modelMessage{}, errors.New("invalid worker tool call")
		}
		seen[call.ID] = true
	}
	return message, nil
}
func workerTools() []map[string]any {
	return []map[string]any{
		workerTool("read_file", "Read a relative text file inside the isolated workspace.", []string{"path"}, nil),
		workerTool("write_file", "Write a relative text file inside the isolated workspace.", []string{"path", "content"}, nil),
		workerTool("run_command", "Run an offline build, test or inspection command inside the isolated Docker container. Never install dependencies or contact remote services.", []string{"command"}, nil),
		workerTool("ask_decision", "Stop for a prepared question to the responsible coordinator.", []string{"question", "recommendation", "why"}, []string{"options", "evidence"}),
		workerTool("finish", "Report an acceptance summary after actual changes and checks. The daemon collects patch and command evidence for independent PA review.", []string{"summary"}, nil),
	}
}
func workerTool(name, description string, fields, arrays []string) map[string]any {
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
	return map[string]any{"type": "function", "function": map[string]any{"name": name, "description": description, "strict": true, "parameters": map[string]any{"type": "object", "properties": props, "required": required, "additionalProperties": false}}}
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
