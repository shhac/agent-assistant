package workerbroker

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/shhac/lib-agent-harness/session"
)

// invoke calls one of the worker's tools the way the harness does: by name, with
// JSON arguments, against a real run. The engine transport is exercised
// end-to-end in native_integration_test.go; this is for the cases where the
// question is what a single tool does to durable state.
func invoke(t *testing.T, b *Broker, id, agentID, name string, args any) session.ToolResult {
	t.Helper()
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := b.snapshot(id)
	container := "agent-assistant-" + id
	implement := true
	if err == nil {
		if stored.Container != "" {
			container = stored.Container
		}
		implement = contains(stored.Request.Capabilities, "implement")
		if agentID == "" {
			agentID = stored.Request.AgentID
		}
	}
	tools := &workerTools{broker: b, id: id, container: container, implement: implement, agentID: agentID}
	result, err := tools.CallTool(context.Background(), session.ToolCall{Name: name, Arguments: raw})
	if err != nil {
		t.Fatalf("%s failed outside the tool contract: %v", name, err)
	}
	return result
}

// rawInvoke passes arguments through untouched, for malformed input.
func rawInvoke(b *Broker, id, agentID, name, args string) session.ToolResult {
	tools := &workerTools{broker: b, id: id, container: "agent-assistant-" + id, implement: true, agentID: agentID}
	result, err := tools.CallTool(context.Background(), session.ToolCall{Name: name, Arguments: json.RawMessage(args)})
	if err != nil {
		return session.ToolResult{Content: err.Error(), IsError: true}
	}
	return result
}

// Every tool the worker is offered is one this handler actually implements, and
// the ones that end an assignment are the ones marked as ending it. A tool that
// was advertised but unhandled would look to the worker like a refusal it could
// retry forever.
func TestAdvertisedToolsAreImplementedAndClosingOnesAreMarked(t *testing.T) {
	closing := map[string]bool{"send_message": true, "ask_decision": true, "finish": true}
	seen := map[string]bool{}
	for _, definition := range workerToolDefinitions() {
		seen[definition.Name] = true
		if definition.Closing != closing[definition.Name] {
			t.Errorf("%s is marked Closing=%v", definition.Name, definition.Closing)
		}
		if definition.Schema == nil || definition.Description == "" {
			t.Errorf("%s is advertised without a schema or description", definition.Name)
		}
	}
	for name := range closing {
		if !seen[name] {
			t.Errorf("%s is handled but never offered", name)
		}
	}
	unknown := rawInvoke(&Broker{}, "id", "agent", "Bash", `{}`)
	if !unknown.IsError || !strings.Contains(unknown.Content, "not available") {
		t.Errorf("an unoffered tool was not refused: %+v", unknown)
	}
}

// A command whose client was cancelled may still be running inside the
// container, still writing. Everything that would change the workspace on top of
// that is refused until it is settled — not only the acceptance report, because
// a write landing beside an unknown process is exactly what makes the evidence
// afterwards undescribable. Reading stays open: it is how a worker finds out
// what it is dealing with.
func TestAnUnsettledCommandHoldsEveryFurtherChange(t *testing.T) {
	b, _ := newFixture(t, "https://model.test", &fakeDocker{workspace: t.TempDir()})
	defer b.Close()
	id := runningRun(t, b)
	if err := b.update(id, func(run *storedRun) error {
		run.Commands = append(run.Commands, commandRecord{Command: "make", Success: false, Output: "[the command exceeded its limit]"})
		run.UnsettledCommands = 1
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		tool string
		args map[string]string
	}{
		{"write_file", map[string]string{"path": "hello.txt", "content": "after\n"}},
		{"run_command", map[string]string{"command": "go test ./..."}},
		{"finish", map[string]string{"summary": "all done"}},
	} {
		result := invoke(t, b, id, "agent-one", tc.tool, tc.args)
		if !result.IsError && tc.tool != "finish" {
			t.Errorf("%s ran while the workspace may still be changing: %s", tc.tool, result.Content)
		}
		if tc.tool == "finish" && !strings.Contains(result.Content, "held") {
			t.Errorf("finish was published over an unsettled command: %s", result.Content)
		}
	}
	// Reading stays available; it is not refused by the hold. The fixture has no
	// container behind it, so what matters is which refusal comes back.
	if read := invoke(t, b, id, "agent-one", "read_file", map[string]string{"path": "hello.txt"}); strings.Contains(read.Content, "never confirmed stopped") {
		t.Errorf("reading was held, so the worker cannot establish what happened: %s", read.Content)
	}

	// Confirmed removal of the container settles what is still moving. What those
	// commands did stays unestablished, and the next attempt is told to go and
	// find out rather than inheriting the assumption that it is fine.
	b.settleContainerProcesses(id)
	settled, _ := b.snapshot(id)
	if settled.UnsettledCommands != 0 {
		t.Fatalf("confirmed cleanup left the assignment permanently held: %d", settled.UnsettledCommands)
	}
	if len(settled.Commands) != 1 || settled.Commands[0].Success {
		t.Errorf("the uncertain command's outcome was repaired rather than preserved: %+v", settled.Commands)
	}
	if !strings.Contains(strings.Join(settled.Messages, "\n"), "Re-read the files") {
		t.Errorf("the next attempt was not told to re-verify: %v", settled.Messages)
	}
	if result := invoke(t, b, id, "agent-one", "write_file", map[string]string{"path": "hello.txt", "content": "after\n"}); result.IsError {
		t.Errorf("a settled assignment was still refusing work: %s", result.Content)
	}
}
