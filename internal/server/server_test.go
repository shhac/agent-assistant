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
