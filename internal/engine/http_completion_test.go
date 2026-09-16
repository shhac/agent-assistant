package engine

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPEffortAndCallerTools(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Model     string `json:"model"`
			Effort    string `json:"reasoning_effort"`
			Tools     []Tool `json:"tools"`
			MaxTokens int    `json:"max_completion_tokens"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request.Model != "api-model" || request.Effort != "high" || request.MaxTokens != 512 || len(request.Tools) != 1 || request.Tools[0].Function.Name != "worker_only" {
			t.Errorf("unexpected HTTP request: %+v", request)
		}
		io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`)
	}))
	defer server.Close()
	result, _, err := Complete(context.Background(), Config{Engine: "openai-compatible", Endpoint: server.URL, Model: "api-model", Effort: "high", MaxOutputTokens: 512}, []Message{{Role: "user", Content: "hello"}}, []Tool{{Type: "function", Function: Function{Name: "worker_only"}}})
	if err != nil || result.Content != "ok" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}
