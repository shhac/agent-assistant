package main

import (
	"bufio"
	"encoding/json"
	"os"
	"sync"
	"time"
)

// Codex's app-server dialect: JSON-RPC over stdio, a thread that outlives its
// turns, and notifications rather than a message stream. Steering is native
// here — the same turn continues — which is the other half of the control
// behaviour the daemon has to handle correctly.

type codexSession struct {
	mu       sync.Mutex
	out      *bufio.Writer
	thread   string
	tools    *tools
	turn     int
	active   string
	stopping chan struct{}
	steered  chan string
}

func runCodex(args []string) {
	connected, err := connect(args)
	if err != nil {
		os.Exit(3)
	}
	defer connected.close()

	if endpoint := probeEndpoint(args); endpoint != "" {
		// The shape captured from the installed CLI: definitions live under
		// input[additional_tools], grouped into namespaces, with MCP tools
		// deferred behind tool_search. The hosted tools are deliberately absent,
		// because that is what the real one sends.
		announce(endpoint, map[string]any{
			"model": modelFrom(args), "reasoning": map[string]any{"effort": effortFrom(args)},
			"input": []any{
				map[string]any{"type": "additional_tools", "role": "system", "tools": []any{
					map[string]any{"type": "namespace", "name": "functions", "tools": []any{
						map[string]any{"type": "function", "name": "list_mcp_resources"},
						map[string]any{"type": "function", "name": "list_mcp_resource_templates"},
						map[string]any{"type": "function", "name": "read_mcp_resource"},
					}},
					map[string]any{"type": "tool_search"},
				}},
				map[string]any{"type": "message", "role": "user", "content": "Capability check only."},
			},
		})
		return
	}

	s := &codexSession{out: bufio.NewWriter(os.Stdout), tools: connected}
	plan := script()
	reader := bufio.NewScanner(os.Stdin)
	reader.Buffer(make([]byte, 4096), 8<<20)
	for reader.Scan() {
		var frame struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if json.Unmarshal(reader.Bytes(), &frame) != nil {
			continue
		}
		switch frame.Method {
		case "initialize":
			s.reply(frame.ID, map[string]any{"userAgent": "fakecli"})
		case "initialized":
		case "thread/start":
			s.thread = "thread-1"
			s.reply(frame.ID, map[string]any{"thread": map[string]any{"id": s.thread}})
		case "thread/resume":
			var in struct {
				ThreadID string `json:"threadId"`
			}
			_ = json.Unmarshal(frame.Params, &in)
			s.thread = in.ThreadID
			s.reply(frame.ID, map[string]any{"thread": map[string]any{"id": s.thread}})
		case "turn/start":
			recordPrompt(turnText(frame.Params))
			id := "turn-" + itoa(s.turn+1)
			s.begin(id)
			s.reply(frame.ID, map[string]any{"turn": map[string]any{"id": id}})
			go s.runTurn(id, plan)
		case "turn/interrupt":
			s.mu.Lock()
			stop := s.stopping
			s.mu.Unlock()
			if stop != nil {
				select {
				case <-stop:
				default:
					close(stop)
				}
			}
			s.reply(frame.ID, map[string]any{})
		case "turn/steer":
			var in struct {
				ExpectedTurnID string `json:"expectedTurnId"`
				Input          []struct {
					Text string `json:"text"`
				} `json:"input"`
			}
			_ = json.Unmarshal(frame.Params, &in)
			s.mu.Lock()
			steered, active := s.steered, s.active
			s.mu.Unlock()
			if active != in.ExpectedTurnID {
				s.fail(frame.ID, -32000, "turn is not active")
				continue
			}
			if steered != nil && len(in.Input) > 0 {
				recordPrompt(in.Input[0].Text)
				select {
				case steered <- in.Input[0].Text:
				default:
				}
			}
			s.reply(frame.ID, map[string]any{"turnId": active})
		default:
			s.fail(frame.ID, -32601, "method not found")
		}
	}
}

func turnText(params json.RawMessage) string {
	var in struct {
		Input []struct {
			Text string `json:"text"`
		} `json:"input"`
	}
	if json.Unmarshal(params, &in) != nil || len(in.Input) == 0 {
		return ""
	}
	return in.Input[0].Text
}

func modelFrom(args []string) string {
	for i, arg := range args {
		if arg == "--model" && i+1 < len(args) {
			return args[i+1]
		}
	}
	return "fake-model"
}

func effortFrom(args []string) string {
	for i, arg := range args {
		if arg == "-c" && i+1 < len(args) {
			if after, ok := trimPrefix(args[i+1], "model_reasoning_effort="); ok {
				return unquote(after)
			}
		}
	}
	return "medium"
}

func trimPrefix(value, prefix string) (string, bool) {
	if len(value) >= len(prefix) && value[:len(prefix)] == prefix {
		return value[len(prefix):], true
	}
	return "", false
}

func itoa(n int) string {
	raw, _ := json.Marshal(n)
	return string(raw)
}

func (s *codexSession) reply(id json.RawMessage, result any) {
	s.send(map[string]any{"id": id, "result": result})
}

func (s *codexSession) fail(id json.RawMessage, code int, message string) {
	s.send(map[string]any{"id": id, "error": map[string]any{"code": code, "message": message}})
}

func (s *codexSession) notify(method string, params map[string]any) {
	s.send(map[string]any{"method": method, "params": params})
}

func (s *codexSession) send(frame map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, _ := json.Marshal(frame)
	_, _ = s.out.Write(append(raw, '\n'))
	_ = s.out.Flush()
}

func (s *codexSession) begin(id string) {
	s.mu.Lock()
	s.active = id
	s.stopping = make(chan struct{})
	s.steered = make(chan string, 4)
	s.turn++
	s.mu.Unlock()
}

func (s *codexSession) runTurn(id string, plan []turnScript) {
	s.mu.Lock()
	index := s.turn - 1
	stop, steered := s.stopping, s.steered
	s.mu.Unlock()
	s.notify("turn/started", map[string]any{"threadId": s.thread, "turn": map[string]any{"id": id}})

	var steps []step
	if index < len(plan) {
		steps = plan[index].Steps
	}
	if delay := turnDelay(); delay > 0 {
		select {
		case <-time.After(delay):
		case <-stop:
			s.finish(id, "interrupted")
			return
		}
	}
	for _, current := range steps {
		if current.Hold {
			select {
			case <-stop:
				s.finish(id, "interrupted")
				return
			case direction := <-steered:
				// Native steering continues this same turn, which is what makes it
				// different from Claude's composition.
				s.notify("item/completed", map[string]any{"threadId": s.thread, "turnId": id,
					"item": map[string]any{"id": "steer", "type": "agentMessage", "text": "acknowledged: " + direction}})
				continue
			}
		}
		if current.Fail {
			s.finish(id, "failed")
			return
		}
		if current.Text != "" {
			s.notify("item/completed", map[string]any{"threadId": s.thread, "turnId": id,
				"item": map[string]any{"id": "msg", "type": "agentMessage", "text": current.Text}})
			continue
		}
		if current.Tool == "" {
			continue
		}
		s.notify("item/started", map[string]any{"threadId": s.thread, "turnId": id,
			"item": map[string]any{"id": "call-" + current.Tool, "type": "mcpToolCall", "tool": current.Tool, "status": "running"}})
		result, isError := s.tools.invoke(current.Tool, current.Args)
		status := "completed"
		if isError {
			status = "failed"
		}
		_ = result
		s.notify("item/completed", map[string]any{"threadId": s.thread, "turnId": id,
			"item": map[string]any{"id": "call-" + current.Tool, "type": "mcpToolCall", "tool": current.Tool, "status": status}})
	}
	s.finish(id, "completed")
}

func (s *codexSession) finish(id, status string) {
	s.notify("thread/tokenUsage/updated", map[string]any{"threadId": s.thread, "turnId": id, "tokenUsage": map[string]any{
		"last":  map[string]any{"inputTokens": 30, "outputTokens": 9, "cachedInputTokens": 4, "cacheWriteInputTokens": 6, "reasoningOutputTokens": 2},
		"total": map[string]any{"inputTokens": 30, "outputTokens": 9, "cachedInputTokens": 4, "cacheWriteInputTokens": 6, "reasoningOutputTokens": 2},
	}})
	s.notify("turn/completed", map[string]any{"threadId": s.thread, "turn": map[string]any{"id": id, "status": status}})
	s.mu.Lock()
	s.active = ""
	s.mu.Unlock()
}
