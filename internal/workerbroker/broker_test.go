package workerbroker

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shhac/agent-assistant/internal/integrations/worker"
)

const fixtureImage = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

type fakeDocker struct {
	mu        sync.Mutex
	calls     [][]string
	workspace string
	stopped   bool
}

func (d *fakeDocker) Run(ctx context.Context, args []string, input []byte) ([]byte, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.calls = append(d.calls, append([]string{}, args...))
	if len(args) > 1 && args[0] == "image" {
		return []byte(fixtureImage), nil
	}
	if args[0] == "run" {
		for i, a := range args {
			if a == "--mount" {
				mount := args[i+1]
				d.workspace = strings.TrimSuffix(strings.TrimPrefix(mount, "type=bind,src="), ",dst=/workspace")
			}
		}
		d.stopped = false
		return []byte("container-id"), nil
	}
	if args[0] == "rm" {
		d.stopped = true
		return nil, nil
	}
	if args[0] == "exec" {
		if contains(args, "--interactive") {
			p := args[len(args)-1]
			if !strings.HasPrefix(p, "/workspace/") {
				return nil, errors.New("outside fake workspace")
			}
			target := filepath.Join(d.workspace, strings.TrimPrefix(p, "/workspace/"))
			_ = os.MkdirAll(filepath.Dir(target), 0755)
			return nil, os.WriteFile(target, input, 0644)
		}
		if args[len(args)-2] == "-lc" {
			return []byte("PASS: synthetic verification\n"), nil
		}
		p := args[len(args)-1]
		return os.ReadFile(filepath.Join(d.workspace, strings.TrimPrefix(p, "/workspace/")))
	}
	if args[0] == "container" {
		if args[1] == "inspect" {
			id := strings.TrimPrefix(args[len(args)-1], "agent-assistant-")
			return []byte(strings.Repeat("b", 64) + " " + id), nil
		}
		return nil, nil
	}
	return nil, nil
}
func newFixture(t *testing.T, endpoint string, d *fakeDocker) (*Broker, Config) {
	t.Helper()
	t.Setenv("WORKER_TEST_TOKEN", "fixture-auth")
	t.Setenv("WORKER_TEST_MODEL_KEY", "fixture-model-secret")
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	if err := os.MkdirAll(source, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "hello.txt"), []byte("before\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := Config{StateDir: filepath.Join(dir, "state"), Workspace: source, ProjectID: "project-one", Image: fixtureImage, ModelEndpoint: endpoint, Model: "fixture-model", APIKeyEnv: "WORKER_TEST_MODEL_KEY", TokenEnv: "WORKER_TEST_TOKEN", Command: d}
	b, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return b, cfg
}
func startRequest() worker.StartRequest {
	return worker.StartRequest{DispatchKey: "dispatch-one", AgentID: "agent-one", ProjectID: "project-one", Role: "worker", Task: "Update the greeting", AcceptanceCriteria: "Greeting says after and synthetic check passes", Capabilities: []string{"implement"}, Prohibitions: []string{"deployment", "production_data_access", "purchases"}}
}
func request(t *testing.T, b *Broker, path, key string, value any) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(value)
	r := httptest.NewRequest("POST", "http://broker.test"+path, strings.NewReader(string(body)))
	r.Header.Set("Authorization", "Bearer fixture-auth")
	r.Header.Set("Idempotency-Key", key)
	w := httptest.NewRecorder()
	b.Handler().ServeHTTP(w, r)
	return w
}
func TestWorkerImplementsInCopyAndPublishesActualEvidence(t *testing.T) {
	calls := 0
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer fixture-model-secret" {
			t.Error("host model missing configured key")
		}
		names := []string{"write_file", "run_command", "finish"}
		args := []string{`{"path":"hello.txt","content":"after\n"}`, `{"command":"printf synthetic-test"}`, `{"summary":"Updated the greeting and synthetic verification passed."}`}
		call := toolCall{ID: names[calls-1], Type: "function"}
		call.Function.Name = names[calls-1]
		call.Function.Arguments = args[calls-1]
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": modelMessage{Role: "assistant", ToolCalls: []toolCall{call}}, "finish_reason": "tool_calls"}}})
	}))
	defer provider.Close()
	docker := &fakeDocker{}
	b, cfg := newFixture(t, provider.URL, docker)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- b.Run(ctx) }()
	defer func() { cancel(); <-done; _ = b.Close() }()
	resp := request(t, b, "/runs", "dispatch-one", startRequest())
	if resp.Code != 201 {
		t.Fatal(resp.Body.String())
	}
	var run worker.Run
	_ = json.Unmarshal(resp.Body.Bytes(), &run)
	deadline := time.After(5 * time.Second)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-deadline:
			t.Fatal("worker did not complete")
		case <-ticker.C:
			current, _ := b.snapshot(run.ID)
			if current.Run.Status == "completed" {
				run = current.Run
				goto finished
			}
			if current.Run.Status == "interrupted" {
				t.Fatal(current.Run.Summary)
			}
		}
	}
finished:
	if len(run.Evidence) < 8 || calls != 3 {
		t.Fatalf("missing actual evidence: %#v calls=%d", run, calls)
	}
	original, _ := os.ReadFile(filepath.Join(cfg.Workspace, "hello.txt"))
	if string(original) != "before\n" {
		t.Fatal("original workspace was modified")
	}
	patch, err := os.ReadFile(filepath.Join(cfg.StateDir, "runs", run.ID, "artifacts", "changes.patch"))
	if err != nil || !strings.Contains(string(patch), "-before\n+after\n") {
		t.Fatalf("missing actual diff: %q %v", patch, err)
	}
	commands, _ := os.ReadFile(filepath.Join(cfg.StateDir, "runs", run.ID, "artifacts", "commands.json"))
	if !strings.Contains(string(commands), "PASS: synthetic verification") {
		t.Fatal("actual command result not captured")
	}
	docker.mu.Lock()
	defer docker.mu.Unlock()
	if !docker.stopped {
		t.Fatal("completion published before container stopped")
	}
	for _, call := range docker.calls {
		if call[0] != "run" {
			continue
		}
		for _, pair := range [][]string{{"--network", "none"}, {"--cap-drop", "ALL"}, {"--security-opt", "no-new-privileges"}, {"--pull", "never"}} {
			if pair[0] == "--pull" {
				if !contains(call, "--pull=never") {
					t.Error("image may pull")
				}
				continue
			}
			found := false
			for i, v := range call {
				if v == pair[0] && i+1 < len(call) && call[i+1] == pair[1] {
					found = true
				}
			}
			if !found {
				t.Errorf("missing isolation %v", pair)
			}
		}
		joined := strings.Join(call, " ")
		if strings.Contains(joined, "docker.sock") || strings.Contains(joined, "fixture-model-secret") {
			t.Fatal("host socket or credentials mounted")
		}
		if !contains(call, "--read-only") {
			t.Fatal("root filesystem writable")
		}
	}
}
func TestProtocolScopeAuthenticationAndIdempotency(t *testing.T) {
	b, _ := newFixture(t, "http://127.0.0.1:1", &fakeDocker{})
	defer b.Close()
	r := httptest.NewRequest("GET", "http://broker.test/runs", nil)
	w := httptest.NewRecorder()
	b.Handler().ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal("anonymous broker access")
	}
	in := startRequest()
	first := request(t, b, "/runs", in.DispatchKey, in)
	again := request(t, b, "/runs", in.DispatchKey, in)
	if first.Code != 201 || again.Code != 200 || first.Body.String() != again.Body.String() {
		t.Fatal("start retry is not stable")
	}
	in.Task = "Different work"
	if request(t, b, "/runs", in.DispatchKey, in).Code != 409 {
		t.Fatal("key reuse accepted")
	}
	in = startRequest()
	in.Role = "manager"
	if request(t, b, "/runs", in.DispatchKey, in).Code != 403 {
		t.Fatal("manager accepted by direct-worker broker")
	}
	in = startRequest()
	in.ProjectID = "other-project"
	if request(t, b, "/runs", in.DispatchKey, in).Code != 403 {
		t.Fatal("cross-project work accepted")
	}
}
func TestInterruptedMessageCannotBypassExplicitResume(t *testing.T) {
	b, _ := newFixture(t, "http://127.0.0.1:1", &fakeDocker{})
	defer b.Close()
	w := request(t, b, "/runs", "dispatch-one", startRequest())
	var run worker.Run
	_ = json.Unmarshal(w.Body.Bytes(), &run)
	_ = b.update(run.ID, func(r *storedRun) error { r.Run.Status = "interrupted"; return nil })
	if request(t, b, "/runs/"+run.ID+"/messages", "message-one", map[string]string{"message": "Continue"}).Code != 409 {
		t.Fatal("message bypassed recovery allowance")
	}
	if request(t, b, "/runs/"+run.ID+"/resume", "resume-one", map[string]string{"instruction": "Continue"}).Code != 200 {
		t.Fatal("explicit resume rejected")
	}
}
func TestWorkspaceFilterAndConfinedReads(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	_ = os.MkdirAll(filepath.Join(source, ".git", "hooks"), 0755)
	_ = os.WriteFile(filepath.Join(source, ".env"), []byte("private"), 0600)
	_ = os.WriteFile(filepath.Join(source, ".git", "hooks", "pre-commit"), []byte("private"), 0600)
	_ = os.WriteFile(filepath.Join(source, "normal.txt"), []byte("safe"), 0644)
	outside := filepath.Join(dir, "outside")
	_ = os.WriteFile(outside, []byte("private"), 0600)
	_ = os.Symlink(outside, filepath.Join(source, "link"))
	baseline, err := copyWorkspace(source, filepath.Join(dir, "copy"))
	if err != nil {
		t.Fatal(err)
	}
	if len(baseline) != 1 || string(baseline["normal.txt"]) != "safe" {
		t.Fatalf("unsafe copy: %v", baseline)
	}
	root, _ := os.OpenRoot(source)
	defer root.Close()
	if _, err = readConfined(root, "link"); err == nil {
		t.Fatal("followed outside symlink")
	}
}
func TestBinaryBaselineSurvivesPersistence(t *testing.T) {
	b, cfg := newFixture(t, "http://127.0.0.1:1", &fakeDocker{})
	defer b.Close()
	raw := []byte{0x89, 0x50, 0x4e, 0x47, 0xff, 0, 0xfe}
	dir := filepath.Join(cfg.StateDir, "runs", "binary", "workspace")
	_ = os.MkdirAll(dir, 0755)
	_ = os.WriteFile(filepath.Join(dir, "image.png"), raw, 0644)
	run := storedRun{Run: worker.Run{ID: "binary"}, WorkDir: dir, Baseline: map[string][]byte{"image.png": raw}}
	encoded, _ := json.Marshal(run)
	var restored storedRun
	_ = json.Unmarshal(encoded, &restored)
	if !reflect.DeepEqual(restored.Baseline["image.png"], raw) {
		t.Fatal("binary baseline corrupted")
	}
	if _, err := b.artifacts(restored); err != nil {
		t.Fatal(err)
	}
	patch, _ := os.ReadFile(filepath.Join(cfg.StateDir, "runs", "binary", "artifacts", "changes.patch"))
	if len(patch) != 0 {
		t.Fatalf("false binary diff after roundtrip: %s", patch)
	}
}
func TestIncompleteToolsAreNotReplayedOnRecovery(t *testing.T) {
	call := toolCall{ID: "pending", Type: "function"}
	call.Function.Name = "write_file"
	got := repairTranscript([]modelMessage{{Role: "assistant", ToolCalls: []toolCall{call}}})
	if len(got) != 2 || got[1].Role != "tool" || got[1].ToolCallID != "pending" || !strings.Contains(got[1].Content, "interrupted") {
		t.Fatal("missing uncertainty repair")
	}
}

func TestDockerEnvironmentExcludesHostCredentials(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "private-key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "private-cloud")
	t.Setenv("HOME", "/private-owner-home")
	got := dockerEnvironment()
	if len(got) != 2 || strings.Contains(strings.Join(got, "\n"), "private") {
		t.Fatalf("host credentials inherited: %v", got)
	}
}
func TestReviewWorkspaceIsReadOnly(t *testing.T) {
	b, _ := newFixture(t, "http://127.0.0.1:1", &fakeDocker{})
	defer b.Close()
	r := storedRun{Request: worker.StartRequest{Capabilities: []string{"review"}}, WorkDir: "/synthetic/workspace"}
	args := b.containerArgs(r)
	if !contains(args, "type=bind,src=/synthetic/workspace,dst=/workspace,readonly") {
		t.Fatal("review-only worker received writable source mount")
	}
}
func TestCumulativeModelAllowanceSurvivesResume(t *testing.T) {
	calls := 0
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		call := toolCall{ID: "test", Type: "function"}
		call.Function.Name = "run_command"
		call.Function.Arguments = `{"command":"synthetic-check"}`
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": modelMessage{Role: "assistant", ToolCalls: []toolCall{call}}}}})
	}))
	defer provider.Close()
	b, _ := newFixture(t, provider.URL, &fakeDocker{})
	b.cfg.MaxTurns = 1
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- b.Run(ctx) }()
	defer func() { cancel(); <-done; _ = b.Close() }()
	w := request(t, b, "/runs", "dispatch-one", startRequest())
	var run worker.Run
	_ = json.Unmarshal(w.Body.Bytes(), &run)
	waitBlocked := func() {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			r, _ := b.snapshot(run.ID)
			if r.Run.Status == "blocked" {
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
		t.Fatal("worker did not reach bound")
	}
	waitBlocked()
	if request(t, b, "/runs/"+run.ID+"/resume", "resume-one", map[string]string{"instruction": "Continue"}).Code != 200 {
		t.Fatal("resume request not accepted")
	}
	waitBlocked()
	if calls != 1 {
		t.Fatalf("resume reset cumulative allowance: %d calls", calls)
	}
}
func TestCleanupRequiresProvenContainerIdentity(t *testing.T) {
	r := storedRun{Run: worker.Run{ID: "fixture"}, Container: "agent-assistant-fixture"}
	for _, tc := range []struct {
		name       string
		inspect    []byte
		inspectErr error
		listingErr error
		wantErr    bool
	}{{"missing", nil, errors.New("missing"), nil, false}, {"unavailable", nil, errors.New("daemon down"), errors.New("daemon down"), true}, {"wrong-owner", []byte(strings.Repeat("a", 64) + " other"), nil, nil, true}, {"owned", []byte(strings.Repeat("a", 64) + " fixture"), nil, nil, false}} {
		t.Run(tc.name, func(t *testing.T) {
			removed := false
			cmd := CommandFunc(func(_ context.Context, args []string, _ []byte) ([]byte, error) {
				if args[0] == "rm" {
					removed = true
					if args[2] != strings.Repeat("a", 64) {
						t.Fatal("cleanup used mutable container name")
					}
					return nil, nil
				}
				if args[1] == "inspect" {
					return tc.inspect, tc.inspectErr
				}
				return nil, tc.listingErr
			})
			err := reconcileContainer(context.Background(), cmd, r)
			if (err != nil) != tc.wantErr {
				t.Fatalf("cleanup result %v", err)
			}
			if tc.wantErr && removed {
				t.Fatal("removed unverified container")
			}
		})
	}
}

func TestFailedCommandEvidenceReachesRunDespiteModelSuccessClaim(t *testing.T) {
	calls := 0
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		call := toolCall{ID: "check", Type: "function"}
		call.Function.Name = "run_command"
		call.Function.Arguments = `{"command":"synthetic-failing-test"}`
		if calls == 2 {
			call.ID = "finish"
			call.Function.Name = "finish"
			call.Function.Arguments = `{"summary":"Everything passed."}`
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": modelMessage{Role: "assistant", ToolCalls: []toolCall{call}}}}})
	}))
	defer provider.Close()
	docker := &fakeDocker{}
	b, _ := newFixture(t, provider.URL, docker)
	b.cfg.Command = CommandFunc(func(ctx context.Context, args []string, input []byte) ([]byte, error) {
		if len(args) > 1 && args[0] == "exec" && args[len(args)-1] == "synthetic-failing-test" {
			return []byte("FAIL: acceptance expectation was not satisfied"), errors.New("nonzero test exit")
		}
		return docker.Run(ctx, args, input)
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- b.Run(ctx) }()
	defer func() { cancel(); <-done; _ = b.Close() }()
	response := request(t, b, "/runs", "dispatch-one", startRequest())
	var run worker.Run
	_ = json.Unmarshal(response.Body.Bytes(), &run)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		current, _ := b.snapshot(run.ID)
		if current.Run.Status == "completed" {
			req := httptest.NewRequest("GET", "http://broker.test/runs/"+run.ID, nil)
			req.Header.Set("Authorization", "Bearer fixture-auth")
			w := httptest.NewRecorder()
			b.Handler().ServeHTTP(w, req)
			_ = json.Unmarshal(w.Body.Bytes(), &run)
			text := strings.Join(run.Evidence, "\n")
			for _, want := range []string{"0 SUCCEEDED, 1 FAILED", "Command 1 FAILED: synthetic-failing-test", "FAIL: acceptance expectation was not satisfied", "Content patch SHA-256:"} {
				if !strings.Contains(text, want) {
					t.Fatalf("real failed check hidden by model summary; missing %q in %s", want, text)
				}
			}
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("worker did not provide final evidence")
}
func TestEvidenceDigestIsBoundedAndExplicitAboutOmissions(t *testing.T) {
	names := make([]string, 100)
	for i := range names {
		names[i] = strings.Repeat("x", 2000)
	}
	commands := make([]commandRecord, 30)
	for i := range commands {
		commands[i] = commandRecord{Command: strings.Repeat("c", 8000), Success: i > 0, Output: strings.Repeat("o", 64*1024)}
	}
	evidence := strings.Join(evidenceDigest(names, commands, strings.Repeat("patch\n", 10000)), "\n")
	if len(evidence) > 20000 {
		t.Fatalf("evidence grew beyond context bound: %d", len(evidence))
	}
	for _, want := range []string{"29 SUCCEEDED, 1 FAILED", "Command 1 FAILED", "84 omitted", "24 omitted", "TRUNCATED:", "additional command output may be omitted"} {
		if !strings.Contains(evidence, want) {
			t.Errorf("missing explicit evidence limitation %q", want)
		}
	}
}
