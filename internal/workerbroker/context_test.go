package workerbroker

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/shhac/agent-assistant/internal/engine"
	"github.com/shhac/agent-assistant/internal/integrations/worker"
)

func largeTranscript() []modelMessage {
	messages := []modelMessage{{Role: "user", Content: "Owner direction: preserve keyboard shortcuts."}}
	for _, id := range []string{"a", "b", "c", "d", "e", "f"} {
		var call toolCall
		call.ID = id
		call.Type = "function"
		call.Function.Name = "run_command"
		call.Function.Arguments = `{"command":"verify"}`
		messages = append(messages, modelMessage{Role: "assistant", ToolCalls: []toolCall{call}}, modelMessage{Role: "tool", ToolCallID: id, Content: `{"success":false,"output":"` + strings.Repeat(id, 16000) + `"}`})
	}
	return messages
}
func contextRun(t *testing.T, b *Broker) string {
	t.Helper()
	response := request(t, b, "/runs", "dispatch-one", startRequest())
	if response.Code != 201 {
		t.Fatal(response.Code, response.Body.String())
	}
	var run worker.Run
	if err := json.Unmarshal(response.Body.Bytes(), &run); err != nil {
		t.Fatal(err)
	}
	if err := b.update(run.ID, func(r *storedRun) error { r.Run.Status = "running"; r.Transcript = largeTranscript(); return nil }); err != nil {
		t.Fatal(err)
	}
	return run.ID
}
func TestWorkerContextArchivesCompactionResumesCheckpointAndNewRunFresh(t *testing.T) {
	calls := 0
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var payload struct {
			Messages []engine.Message `json:"messages"`
			Tools    []engine.Tool    `json:"tools"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		if len(payload.Tools) != 0 || len(payload.Messages) != 2 {
			t.Error("summary was given execution tools", payload.Tools)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"Checks failed; inspect the preserved command log. No acceptance was established."}}]}`))
	}))
	defer remote.Close()
	b, cfg := newFixture(t, remote.URL, &fakeDocker{})
	id := contextRun(t, b)
	original, _ := b.snapshot(id)
	working, err := b.prepareContext(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	saved, _ := b.snapshot(id)
	if calls != 1 || saved.ModelCalls != 1 || saved.Run.ContextCompactions != 1 || saved.ContextThrough != len(original.Transcript) || !reflect.DeepEqual(saved.Transcript, original.Transcript) {
		t.Fatal("archive or accounting changed", calls, saved.ModelCalls, saved.ContextThrough)
	}
	if working[0].Content != workerPrompt(original.Request) || !strings.Contains(working[1].Content, "Owner direction") {
		t.Fatal("contract/direction lost", working[:2])
	}
	if err = b.Close(); err != nil {
		t.Fatal(err)
	}
	recovered, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer recovered.Close()
	persisted, _ := recovered.snapshot(id)
	if !reflect.DeepEqual(workingMessages(persisted), working) {
		t.Fatal("restart lost working checkpoint")
	}
	input := startRequest()
	input.DispatchKey = "other-task"
	input.AgentID = "other-agent"
	input.Task = "A fresh independent task"
	response := request(t, recovered, "/runs", input.DispatchKey, input)
	if response.Code != 201 {
		t.Fatal(response.Code, response.Body.String())
	}
	var fresh worker.Run
	_ = json.Unmarshal(response.Body.Bytes(), &fresh)
	next, _ := recovered.snapshot(fresh.ID)
	if next.ContextThrough != 0 || len(next.WorkingContext) != 0 || len(next.ContextCheckpoints) != 0 || next.ModelCalls != 0 || len(next.Transcript) != 0 {
		t.Fatal("new assignment inherited context", next)
	}
}
func TestWorkerSummaryFailureAndPausePreserveArchiveAndCountAttempt(t *testing.T) {
	for _, mode := range []string{"failure", "pause"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			var b *Broker
			var id string
			remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if mode == "failure" {
					w.WriteHeader(503)
					return
				}
				if err := b.update(id, func(run *storedRun) error { run.PendingStatus = "paused"; return nil }); err != nil {
					t.Error(err)
				}
				_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"Checkpoint. Command checks failed; inspect evidence."}}]}`))
			}))
			defer remote.Close()
			b, _ = newFixture(t, remote.URL, &fakeDocker{})
			defer b.Close()
			id = contextRun(t, b)
			original, _ := b.snapshot(id)
			messages, err := b.prepareContext(context.Background(), id)
			saved, _ := b.snapshot(id)
			if calls != 1 || saved.ModelCalls != 1 || !reflect.DeepEqual(saved.Transcript, original.Transcript) {
				t.Fatal("wrong attempt/archive accounting", calls, saved.ModelCalls)
			}
			if mode == "failure" {
				if err == nil || len(saved.WorkingContext) != 0 {
					t.Fatal("failure replaced original context", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err = b.completeForRun(context.Background(), id, messages); !errors.Is(err, errInterrupted) {
				t.Fatal("pause admitted another model call", err)
			}
			if calls != 1 {
				t.Fatal("pause invoked provider", calls)
			}
		})
	}
}
func TestContextSummaryRespectsCumulativeBudget(t *testing.T) {
	remote := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("exhausted allowance contacted provider") }))
	defer remote.Close()
	b, _ := newFixture(t, remote.URL, &fakeDocker{})
	defer b.Close()
	id := contextRun(t, b)
	if err := b.update(id, func(run *storedRun) error { run.ModelCalls = b.cfg.MaxTurns; return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := b.prepareContext(context.Background(), id); !errors.Is(err, errWorkerModelAllowance) {
		t.Fatal(err)
	}
	saved, _ := b.snapshot(id)
	if len(saved.WorkingContext) != 0 {
		t.Fatal("unpaid summary used")
	}
}
func TestRepairedPrefixInvalidatesContextCursor(t *testing.T) {
	run := storedRun{Request: startRequest(), Transcript: []modelMessage{{Role: "user", Content: "Original"}}, WorkingContext: []modelMessage{{Role: "assistant", Content: "Stale checkpoint"}}, ContextThrough: 1}
	run.ContextDigest = transcriptDigest(run.Transcript)
	run.Transcript[0].Content = "Repaired authority"
	got := workingMessages(run)
	if len(got) != 2 || got[1].Content != "Repaired authority" {
		t.Fatal("changed archive prefix skipped", got)
	}
}
