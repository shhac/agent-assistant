package server

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shhac/agent-assistant/internal/app"
	"github.com/shhac/agent-assistant/internal/config"
	"github.com/shhac/agent-assistant/internal/core"
)

func TestDashboardProjectDecisionMemoryFlow(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	store, err := core.Open(filepath.Join(dir, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	s := core.NewService(store, cfg)
	a := app.New(s, cfg, filepath.Join(dir, "config.json"), false)
	auth, _ := NewAuth(dir, "http://127.0.0.1:8340", "", nil)
	h := New(a, auth)
	call := func(method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "http://127.0.0.1:8340"+path, strings.NewReader(body))
		r.RemoteAddr = "127.0.0.1:4321"
		r.Header.Set("Authorization", "Bearer "+auth.admin)
		r.Header.Set("X-Requested-With", "agent-assistant")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	created := call("POST", "/api/projects", `{"title":"Export","description":"CSV","acceptance_criteria":"Valid CSV"}`)
	if created.Code != 201 {
		t.Fatal(created.Body.String())
	}
	var project core.Project
	_ = json.Unmarshal(created.Body.Bytes(), &project)
	d, err := s.CreateDecision(context.Background(), core.DecisionInput{ProjectID: project.ID, Title: "CSV first?", Context: "Excel adds effort", Recommendation: "CSV first", Choices: []string{"CSV", "Excel"}})
	if err != nil {
		t.Fatal(err)
	}
	if w := call("POST", "/api/decisions/"+d.ID+"/resolve", `{"choice":"CSV"}`); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if w := call("POST", "/api/decisions/"+d.ID+"/resolve", `{"choice":"Excel"}`); w.Code != 409 {
		t.Fatal("repeated decision overwrote answer")
	}

	custom, _ := s.CreateDecision(context.Background(), core.DecisionInput{Title: "Custom?", Context: "Options incomplete", Recommendation: "One", Choices: []string{"One", "Two"}})
	if w := call("POST", "/api/decisions/"+custom.ID+"/resolve", `{"choice":"One","answer":"Other"}`); w.Code != 400 {
		t.Fatal("ambiguous answer accepted", w.Code)
	}
	if w := call("POST", "/api/decisions/"+custom.ID+"/resolve", `{"answer":"Use the existing option"}`); w.Code != 200 || !strings.Contains(w.Body.String(), `"disposition":"custom"`) {
		t.Fatal(w.Code, w.Body.String())
	}
	stale, _ := s.CreateDecision(context.Background(), core.DecisionInput{Title: "Stale?", Context: "Already resolved elsewhere", Recommendation: "One", Choices: []string{"One", "Two"}})
	if w := call("POST", "/api/decisions/"+stale.ID+"/dismiss", `{"reason":""}`); w.Code != 400 {
		t.Fatal("blank reason accepted")
	}
	if w := call("POST", "/api/decisions/"+stale.ID+"/dismiss", `{"reason":"Already configured"}`); w.Code != 200 || !strings.Contains(w.Body.String(), `"status":"dismissed"`) || strings.Contains(w.Body.String(), `"answer"`) {
		t.Fatal(w.Code, w.Body.String())
	}
	memory := call("POST", "/api/memories", `{"content":"Prefer concise updates"}`)
	if memory.Code != 201 {
		t.Fatal(memory.Body.String())
	}
	var m core.Memory
	_ = json.Unmarshal(memory.Body.Bytes(), &m)
	if w := call("DELETE", "/api/memories/"+m.ID, ""); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if w := call("GET", "/api/state", ""); w.Code != 200 || !strings.Contains(w.Body.String(), "resolved") {
		t.Fatal(w.Body.String())
	}
	if w := call("GET", "/", ""); w.Code != 200 || !strings.Contains(w.Body.String(), "<html") {
		t.Fatal("embedded UI missing")
	}
}

// The dashboard receives execution health alongside the state it is already
// showing, so a stopped worker is visible without opening each project, and a
// quiet decision queue never stands in for a healthy one.
func TestStateExposesBlockedWorkWithNoOpenDecisions(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.Workers = []config.Worker{{ID: "test", Name: "Suggestions worker", Endpoint: "http://127.0.0.1:9999", Capabilities: []string{"implement"}}}
	store, err := core.Open(filepath.Join(dir, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	s := core.NewService(store, cfg)
	a := app.New(s, cfg, filepath.Join(dir, "config.json"), false)
	auth, _ := NewAuth(dir, "http://127.0.0.1:8340", "", nil)
	h := New(a, auth)
	call := func(method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "http://127.0.0.1:8340"+path, strings.NewReader(body))
		r.RemoteAddr = "127.0.0.1:4321"
		r.Header.Set("Authorization", "Bearer "+auth.admin)
		r.Header.Set("X-Requested-With", "agent-assistant")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	attention := func() []core.ProjectAttention {
		t.Helper()
		var out struct {
			Attention []core.ProjectAttention `json:"attention"`
			Decisions []core.Decision         `json:"decisions"`
		}
		if err := json.Unmarshal(call("GET", "/api/state", "").Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		for _, d := range out.Decisions {
			if d.Status == "open" {
				t.Fatalf("fixture unexpectedly has an open decision: %+v", d)
			}
		}
		return out.Attention
	}

	created := call("POST", "/api/projects", `{"title":"Suggestions","description":"Suggest","acceptance_criteria":"Reviewed"}`)
	if created.Code != 201 {
		t.Fatal(created.Body.String())
	}
	var project core.Project
	_ = json.Unmarshal(created.Body.Bytes(), &project)
	for _, item := range attention() {
		if item.NextAction == "owner" {
			t.Fatalf("a fresh project should not demand the owner: %+v", item)
		}
	}

	ctx := context.Background()
	agent, err := s.Delegate(ctx, core.DelegateInput{ProjectID: project.ID, ProfileID: "test", Role: "worker", Task: "Draft suggestions", AcceptanceCriteria: "Reviewed", Capabilities: []string{"implement"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.BeginDispatch(ctx, agent.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkDispatched(ctx, agent.ID, "external-"+agent.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateAgent(ctx, agent.ID, core.AgentUpdate{Status: "blocked", Summary: "The attempt stopped without a classified provider error.", ProviderFailureKind: "unknown", ModelFailureEvidence: "untyped_error"}); err != nil {
		t.Fatal(err)
	}

	got := attention()
	if len(got) == 0 {
		t.Fatal("no attention reported while a worker is blocked")
	}
	if got[0].Execution != "blocked" {
		t.Fatalf("execution = %q, want blocked", got[0].Execution)
	}
	if got[0].NextAction != "owner" || got[0].OpenDecisions != 0 {
		t.Fatalf("blocked work with no open decision must still be the owner's turn: %+v", got[0])
	}
	if got[0].AgentID != agent.ID || got[0].AgentName != "Suggestions worker" {
		t.Fatalf("attention does not name the affected worker: %+v", got[0])
	}
	if got[0].Reason == "" {
		t.Fatal("attention reports no reason for the blocker")
	}
}
