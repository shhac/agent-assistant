// Command fakecli is a synthetic coding CLI used by the worker broker's tests.
//
// It is not a stub of the library's transport. It speaks the same stream-json
// and app-server protocols the installed CLIs speak, negotiates its tools over
// the same MCP bridge, and answers the same capability check — because a test
// that stubbed the transport would prove the broker talks to itself, not that
// it can drive a coding agent through the tool channel it configures.
//
// It performs no inference. What it "decides" comes from a script in its
// environment, so a test can express a multi-step read/edit/test/finish
// workflow, a long turn, an interruption or a failure and get exactly that.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/shhac/lib-agent-harness/session"
)

// Environment the tests use to drive this binary.
const (
	scriptEnv = "FAKE_CLI_SCRIPT"
	// delayEnv makes every turn take a stated time before it reports, so a test
	// can show work continuing past a boundary that used to end it.
	delayEnv = "FAKE_CLI_TURN_DELAY"
	// markerEnv names a file this process touches on startup, so a test can tell
	// whether a session was ever launched.
	markerEnv = "FAKE_CLI_MARKER"
	// promptEnv names a file every prompt is appended to, one per line. It is how
	// a test can say what the harness was actually told, and how many times —
	// which is the difference between direction delivered and direction repeated.
	promptEnv = "FAKE_CLI_PROMPT_FILE"
	// gateEnv names a file this process waits for before making a scripted
	// trailing call, and resultEnv names the file that call's outcome is written
	// to. Together they let a test place a request at a known point in the
	// daemon's lifecycle rather than guessing with a sleep.
	gateEnv   = "FAKE_CLI_TRAILING_GATE"
	resultEnv = "FAKE_CLI_TRAILING_RESULT"
)

// awaitGate waits for the test to say it is ready. Bounded, so a fixture whose
// test failed early stops rather than holding a process open.
func awaitGate() bool {
	path := os.Getenv(gateEnv)
	if path == "" {
		return true
	}
	for deadline := time.Now().Add(60 * time.Second); time.Now().Before(deadline); {
		if _, err := os.Stat(path); err == nil {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return false
}

// recordTrailing reports what the trailing call actually produced, so a test can
// tell a refusal apart from a call that never happened.
func recordTrailing(text string, isError bool) {
	path := os.Getenv(resultEnv)
	if path == "" {
		return
	}
	outcome := "ok"
	if isError {
		outcome = "refused"
	}
	appendLine(path, outcome+": "+strings.Join(strings.Fields(text), " "))
}

// recordPrompt appends what the harness was told, flattened to one line.
func recordPrompt(text string) {
	if path := os.Getenv(promptEnv); path != "" {
		appendLine(path, strings.Join(strings.Fields(text), " "))
	}
}

type step struct {
	Tool string         `json:"tool,omitempty"`
	Args map[string]any `json:"args,omitempty"`
	Text string         `json:"text,omitempty"`
	// Fail ends the turn as a provider failure instead of a success.
	Fail bool `json:"fail,omitempty"`
	// Hold makes the turn wait for an interrupt rather than finishing.
	Hold bool `json:"hold,omitempty"`
	// Trailing names a tool to call after the turn has already reported terminal.
	// Real harnesses have been observed doing exactly this, and it is what a
	// caller's tool channel has to be closed against.
	Trailing string `json:"trailing,omitempty"`
}

type turnScript struct {
	Steps []step `json:"steps"`
}

func main() {
	args := os.Args[1:]
	if marker := os.Getenv(markerEnv); marker != "" && len(args) > 0 && args[0] != "bridge" {
		appendLine(marker, strings.Join(args, " "))
	}
	switch {
	case len(args) > 0 && args[0] == "bridge":
		// The broker points its bridge command here, so the tests exercise the
		// library's real relay rather than a shortcut into the tool host.
		if err := session.RunBridge(newContext(), os.Stdin, os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, "bridge:", err)
			os.Exit(1)
		}
	case len(args) > 1 && args[0] == "debug" && args[1] == "models":
		emitCatalog()
	case len(args) > 0 && args[0] == "app-server":
		runCodex(args)
	default:
		runClaude(args)
	}
}

func appendLine(path, text string) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(text + "\n")
}

func emitCatalog() {
	catalog := map[string]any{"models": []any{map[string]any{
		"slug": "fake-model", "default_reasoning_level": "medium",
		"supported_reasoning_levels": []any{map[string]any{"effort": "low"}, map[string]any{"effort": "medium"}, map[string]any{"effort": "high"}},
		"shell_type":                 "local", "apply_patch_tool_type": "freeform",
		"experimental_supported_tools": []any{"shell"}, "tool_mode": "experimental",
		"node_repl_disabled": false, "base_instructions": "fake coding instructions",
		"context_window": 200000,
	}}}
	raw, _ := json.Marshal(catalog)
	_, _ = os.Stdout.Write(raw)
}

// --- tool channel ---------------------------------------------------------

type tools struct {
	mu     sync.Mutex
	cmd    *exec.Cmd
	in     io.WriteCloser
	out    *bufio.Scanner
	next   int
	names  []string
	server string
}

// connect starts the configured bridge and negotiates the tool surface over it.
func connect(args []string) (*tools, error) {
	command, bridgeArgs, env, server, err := bridgeFromArgs(args)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(command, bridgeArgs...)
	cmd.Env = os.Environ()
	for key, value := range env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	cmd.Stderr = os.Stderr
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err = cmd.Start(); err != nil {
		return nil, err
	}
	scanner := bufio.NewScanner(out)
	scanner.Buffer(make([]byte, 4096), 8<<20)
	t := &tools{cmd: cmd, in: in, out: scanner, server: server}
	if _, err = t.call("initialize", map[string]any{"protocolVersion": "2025-06-18", "clientInfo": map[string]any{"name": "fakecli", "version": "1"}}); err != nil {
		return nil, err
	}
	notice, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	if _, err = t.in.Write(append(notice, '\n')); err != nil {
		return nil, err
	}
	listed, err := t.call("tools/list", map[string]any{})
	if err != nil {
		return nil, err
	}
	var payload struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	if json.Unmarshal(listed, &payload) != nil {
		return nil, fmt.Errorf("unreadable tool list")
	}
	for _, tool := range payload.Tools {
		t.names = append(t.names, "mcp__"+server+"__"+tool.Name)
	}
	return t, nil
}

// bridgeFromArgs reads the MCP configuration the library passed, in whichever
// form the engine uses.
func bridgeFromArgs(args []string) (string, []string, map[string]string, string, error) {
	env := map[string]string{}
	server, command := "", ""
	var bridgeArgs []string
	for i, arg := range args {
		if after, ok := strings.CutPrefix(arg, "--mcp-config="); ok {
			var config struct {
				Servers map[string]struct {
					Command string            `json:"command"`
					Args    []string          `json:"args"`
					Env     map[string]string `json:"env"`
				} `json:"mcpServers"`
			}
			if json.Unmarshal([]byte(after), &config) != nil {
				return "", nil, nil, "", fmt.Errorf("unreadable mcp config")
			}
			for name, entry := range config.Servers {
				server, command, bridgeArgs, env = name, entry.Command, entry.Args, entry.Env
			}
			continue
		}
		if arg != "-c" || i+1 >= len(args) {
			continue
		}
		setting := args[i+1]
		key, value, found := strings.Cut(setting, "=")
		if !found || !strings.HasPrefix(key, "mcp_servers.") {
			continue
		}
		parts := strings.Split(strings.TrimPrefix(key, "mcp_servers."), ".")
		if len(parts) < 2 {
			continue
		}
		server = parts[0]
		switch {
		case parts[1] == "command":
			command = unquote(value)
		case parts[1] == "args":
			bridgeArgs = unquoteArray(value)
		case parts[1] == "env" && len(parts) == 3:
			env[parts[2]] = unquote(value)
		}
	}
	if command == "" || server == "" {
		return "", nil, nil, "", fmt.Errorf("no tool server configured")
	}
	return command, bridgeArgs, env, server, nil
}

func unquote(value string) string {
	var out string
	if json.Unmarshal([]byte(value), &out) == nil {
		return out
	}
	return strings.Trim(value, `"`)
}

func unquoteArray(value string) []string {
	var out []string
	if json.Unmarshal([]byte(value), &out) == nil {
		return out
	}
	return nil
}

func (t *tools) call(method string, params map[string]any) (json.RawMessage, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.next++
	raw, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": t.next, "method": method, "params": params})
	if _, err := t.in.Write(append(raw, '\n')); err != nil {
		return nil, err
	}
	if !t.out.Scan() {
		return nil, fmt.Errorf("tool channel closed")
	}
	var frame struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(t.out.Bytes(), &frame) != nil {
		return nil, fmt.Errorf("unreadable tool reply")
	}
	if frame.Error != nil {
		return nil, fmt.Errorf("tool error: %s", frame.Error.Message)
	}
	return frame.Result, nil
}

func (t *tools) invoke(name string, args map[string]any) (string, bool) {
	if args == nil {
		args = map[string]any{}
	}
	result, err := t.call("tools/call", map[string]any{"name": name, "arguments": args})
	if err != nil {
		return err.Error(), true
	}
	var payload struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if json.Unmarshal(result, &payload) != nil {
		return "unreadable tool result", true
	}
	text := ""
	if len(payload.Content) > 0 {
		text = payload.Content[0].Text
	}
	return text, payload.IsError
}

func (t *tools) close() {
	_ = t.in.Close()
	_ = t.cmd.Process.Kill()
	_ = t.cmd.Wait()
}

// --- shared ---------------------------------------------------------------

func script() []turnScript {
	raw := os.Getenv(scriptEnv)
	if raw == "" {
		return nil
	}
	var turns []turnScript
	if json.Unmarshal([]byte(raw), &turns) != nil {
		return nil
	}
	return turns
}

func turnDelay() time.Duration {
	if raw := os.Getenv(delayEnv); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil {
			return d
		}
	}
	return 0
}

func probeEndpoint(args []string) string {
	if base := os.Getenv("ANTHROPIC_BASE_URL"); base != "" {
		return base + "/v1/messages"
	}
	for _, arg := range args {
		if !strings.HasPrefix(arg, "model_providers.harness_probe=") {
			continue
		}
		const marker = `base_url="`
		at := strings.Index(arg, marker)
		if at < 0 {
			return ""
		}
		rest := arg[at+len(marker):]
		end := strings.Index(rest, `"`)
		if end < 0 {
			return ""
		}
		return rest[:end] + "/responses"
	}
	return ""
}

// announce sends the capability check the tool surface this process is running
// with, in the shape the corresponding real CLI uses.
func announce(endpoint string, body any) {
	raw, _ := json.Marshal(body)
	response, err := http.Post(endpoint, "application/json", strings.NewReader(string(raw)))
	if err != nil {
		return
	}
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
}

func newContext() contextish { return background{} }

type contextish = interface {
	Deadline() (time.Time, bool)
	Done() <-chan struct{}
	Err() error
	Value(any) any
}

type background struct{}

func (background) Deadline() (time.Time, bool) { return time.Time{}, false }
func (background) Done() <-chan struct{}       { return nil }
func (background) Err() error                  { return nil }
func (background) Value(any) any               { return nil }
