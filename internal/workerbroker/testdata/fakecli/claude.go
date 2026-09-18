package main

import (
	"bufio"
	"encoding/json"
	"os"
	"sync"
	"time"
)

// Claude's stream-json dialect, as the installed CLI speaks it: a control
// handshake, a user message per turn, tool_use and tool_result frames while the
// turn runs, and a terminal result. Interrupting is a control request that is
// acknowledged and then produces an errored terminal result — which is exactly
// the behaviour the daemon has to cope with.

type claudeSession struct {
	mu        sync.Mutex
	out       *bufio.Writer
	sessionID string
	tools     *tools
	turn      int
	// interrupted is set by a control request while a turn is running.
	interrupted chan struct{}
}

func runClaude(args []string) {
	id := ""
	for i, arg := range args {
		if (arg == "--session-id" || arg == "--resume") && i+1 < len(args) {
			id = args[i+1]
		}
	}
	connected, err := connect(args)
	if err != nil {
		os.Exit(3)
	}
	defer connected.close()

	if endpoint := probeEndpoint(args); endpoint != "" {
		// The capability check: report the surface this process is running with,
		// in the shape a real request carries, and stop. No turn is run.
		tools := []map[string]any{}
		for _, name := range connected.names {
			tools = append(tools, map[string]any{"name": name, "input_schema": map[string]any{"type": "object"}})
		}
		announce(endpoint, map[string]any{"model": "fake-model", "tools": tools})
		// A second request with no tools, the way the installed CLI names a
		// session. The daemon must not read this as a missing surface.
		announce(endpoint, map[string]any{"model": "fake-model", "tools": []any{},
			"system": []any{map[string]any{"type": "text", "text": "You are naming a coding session so the user can pick it out of a long list."}}})
		return
	}

	s := &claudeSession{out: bufio.NewWriter(os.Stdout), sessionID: id, tools: connected}
	s.emit(map[string]any{
		"type": "system", "subtype": "init", "session_id": id,
		"tools":          connected.names,
		"mcp_servers":    []any{map[string]any{"name": connected.server, "source": "dynamic", "status": "connected"}},
		"permissionMode": "dontAsk", "slash_commands": []any{},
	})
	plan := script()
	var turns sync.WaitGroup
	reader := bufio.NewScanner(os.Stdin)
	reader.Buffer(make([]byte, 4096), 8<<20)
	for reader.Scan() {
		var frame map[string]json.RawMessage
		if json.Unmarshal(reader.Bytes(), &frame) != nil {
			continue
		}
		switch text(frame, "type") {
		case "control_request":
			s.control(frame)
		case "user":
			recordPrompt(userText(frame))
			// On its own goroutine, because the real CLI keeps reading control
			// requests while a turn runs. A loop that ran the turn inline could
			// never see the interrupt that was meant to stop it.
			turns.Add(1)
			go func() { defer turns.Done(); s.runTurn(plan) }()
		}
	}
	turns.Wait()
}

func text(frame map[string]json.RawMessage, key string) string {
	var out string
	_ = json.Unmarshal(frame[key], &out)
	return out
}

func userText(frame map[string]json.RawMessage) string {
	var message struct {
		Content string `json:"content"`
	}
	_ = json.Unmarshal(frame["message"], &message)
	return message.Content
}

func (s *claudeSession) emit(frame map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, _ := json.Marshal(frame)
	_, _ = s.out.Write(append(raw, '\n'))
	_ = s.out.Flush()
}

func (s *claudeSession) control(frame map[string]json.RawMessage) {
	var request struct {
		Subtype string `json:"subtype"`
	}
	_ = json.Unmarshal(frame["request"], &request)
	id := text(frame, "request_id")
	switch request.Subtype {
	case "initialize":
		s.emit(map[string]any{"type": "control_response", "response": map[string]any{
			"subtype": "success", "request_id": id,
			"response": map[string]any{"account": map[string]any{"email": "", "subscriptionType": "synthetic"}},
		}})
	case "interrupt":
		s.mu.Lock()
		stop := s.interrupted
		s.mu.Unlock()
		if stop != nil {
			select {
			case <-stop:
			default:
				close(stop)
			}
		}
		s.emit(map[string]any{"type": "control_response", "response": map[string]any{"subtype": "success", "request_id": id, "response": map[string]any{}}})
	default:
		s.emit(map[string]any{"type": "control_response", "response": map[string]any{
			"subtype": "error", "request_id": id, "error": "Unsupported control request subtype: " + request.Subtype,
		}})
	}
}

func (s *claudeSession) runTurn(plan []turnScript) {
	s.mu.Lock()
	index := s.turn
	s.turn++
	s.interrupted = make(chan struct{})
	stop := s.interrupted
	s.mu.Unlock()

	var steps []step
	if index < len(plan) {
		steps = plan[index].Steps
	}
	if delay := turnDelay(); delay > 0 {
		select {
		case <-time.After(delay):
		case <-stop:
			s.terminal(true, "error_during_execution")
			return
		}
	}
	trailing := ""
	for _, current := range steps {
		if current.Trailing != "" {
			trailing = current.Trailing
			continue
		}
		if current.Hold {
			<-stop
			s.terminal(true, "error_during_execution")
			return
		}
		if current.Fail {
			s.terminal(true, "error_during_execution")
			return
		}
		if current.Text != "" {
			s.emit(map[string]any{"type": "assistant", "session_id": s.sessionID, "message": map[string]any{
				"id": "msg", "model": "fake-model", "content": []any{map[string]any{"type": "text", "text": current.Text}},
				"usage": map[string]any{"input_tokens": 10, "output_tokens": 5, "cache_read_input_tokens": 0, "cache_creation_input_tokens": 3},
			}})
			continue
		}
		if current.Tool == "" {
			continue
		}
		name := "mcp__" + s.tools.server + "__" + current.Tool
		s.emit(map[string]any{"type": "assistant", "session_id": s.sessionID, "message": map[string]any{
			"id": "msg", "model": "fake-model",
			"content": []any{map[string]any{"type": "tool_use", "id": "call-" + current.Tool, "name": name, "input": current.Args}},
			"usage":   map[string]any{"input_tokens": 12, "output_tokens": 4, "cache_read_input_tokens": 0, "cache_creation_input_tokens": 0},
		}})
		result, isError := s.tools.invoke(current.Tool, current.Args)
		s.emit(map[string]any{"type": "user", "session_id": s.sessionID, "message": map[string]any{
			"content": []any{map[string]any{"type": "tool_result", "tool_use_id": "call-" + current.Tool, "is_error": isError, "content": result}},
		}})
	}
	s.terminal(false, "success")
	if trailing == "" {
		return
	}
	// A call made after the turn has already ended. The pause is so this is about
	// the state the channel is left in rather than about racing the terminal
	// frame down a different socket than the one that carried it.
	time.Sleep(50 * time.Millisecond)
	s.tools.invoke(trailing, map[string]any{"path": "after-the-turn.txt", "content": "late"})
}

func (s *claudeSession) terminal(failed bool, subtype string) {
	frame := map[string]any{
		"type": "result", "subtype": subtype, "is_error": failed, "session_id": s.sessionID,
		"usage": map[string]any{"input_tokens": 40, "output_tokens": 12, "cache_read_input_tokens": 5, "cache_creation_input_tokens": 7},
	}
	if !failed {
		frame["result"] = "done"
	}
	s.emit(frame)
}
