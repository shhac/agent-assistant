package workerbroker

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWorkerModelProfileKeepsItsEffortAndIsolatedTools(t *testing.T) {
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Model  string `json:"model"`
			Effort string `json:"reasoning_effort"`
			Tools  []struct {
				Function struct {
					Name string `json:"name"`
				} `json:"function"`
			} `json:"tools"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		if payload.Model != "worker-fixture" || payload.Effort != "low" {
			t.Errorf("worker selection lost: %+v", payload)
		}
		names := map[string]bool{}
		for _, tool := range payload.Tools {
			names[tool.Function.Name] = true
		}
		if !names["run_command"] || !names["finish"] || names["delegate"] || names["remember_preference"] {
			t.Errorf("incorrect worker tool boundary: %v", names)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","tool_calls":[{"id":"finish-1","type":"function","function":{"name":"finish","arguments":"{\"summary\":\"Verified fixture\"}"}}]}}]}`))
	}))
	defer remote.Close()
	b := &Broker{cfg: Config{Engine: "openai-compatible", Model: "worker-fixture", Effort: "low", ModelEndpoint: remote.URL, MaxOutputTokens: 4096}}
	response, err := b.complete(context.Background(), []modelMessage{{Role: "user", Content: "Verify fixture"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(response.ToolCalls) != 1 || response.ToolCalls[0].Function.Name != "finish" {
		t.Fatalf("lost worker action: %+v", response)
	}
}

func TestWorkerRejectsDuplicateActionIDsBeforeExecutingBatch(t *testing.T) {
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"same-id","type":"function","function":{"name":"run_command","arguments":"{\"command\":\"true\"}"}},{"id":"same-id","type":"function","function":{"name":"run_command","arguments":"{\"command\":\"true\"}"}}]}}]}`))
	}))
	defer remote.Close()
	b := &Broker{cfg: Config{Engine: "openai-compatible", Model: "fixture", ModelEndpoint: remote.URL}}
	response, err := b.complete(context.Background(), []modelMessage{{Role: "user", Content: "Check"}})
	if err == nil || len(response.ToolCalls) != 0 {
		t.Fatalf("corrupt batch accepted: %+v %v", response, err)
	}
}
