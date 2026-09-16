package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shhac/agent-assistant/internal/app"
	"github.com/shhac/agent-assistant/internal/config"
	"github.com/shhac/agent-assistant/internal/core"
)

func TestChatQueueRoutesAuthenticateValidateAndRetryIdempotently(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	store, err := core.Open(filepath.Join(dir, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	s := core.NewService(store, cfg)
	a := app.New(s, cfg, filepath.Join(dir, "config.json"), false)
	auth, err := NewAuth(dir, "http://127.0.0.1:8340", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	h := New(a, auth)
	call := func(method, path, body string, authorized, csrf bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "http://127.0.0.1:8340"+path, strings.NewReader(body))
		r.RemoteAddr = "127.0.0.1:1234"
		if authorized {
			r.Header.Set("Authorization", "Bearer "+auth.admin)
		}
		if csrf {
			r.Header.Set("X-Requested-With", "agent-assistant")
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	for _, route := range []struct{ method, path, body string }{{"POST", "/api/chat/messages", `{"id":"one","message":"Hello"}`}, {"GET", "/api/chat/turns", ""}, {"DELETE", "/api/chat/messages/one", ""}} {
		if w := call(route.method, route.path, route.body, false, true); w.Code != 401 {
			t.Fatal(route, w.Code, w.Body.String())
		}
	}
	if w := call("POST", "/api/chat/messages", `{"id":"one","message":"Hello"}`, true, false); w.Code != 403 {
		t.Fatal(w.Code)
	}
	for _, body := range []string{`{"id":"one","message":""}`, `{"id":"invalid.id","message":"hello"}`, `{"id":"one","message":"hello","extra":true}`} {
		if w := call("POST", "/api/chat/messages", body, true, true); w.Code != 400 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	for i := 0; i < 2; i++ {
		if w := call("POST", "/api/chat/messages", `{"id":"one","message":"Hello"}`, true, true); w.Code != 202 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if w := call("POST", "/api/chat/messages", `{"id":"one","message":"Changed"}`, true, true); w.Code != 409 {
		t.Fatal(w.Code, w.Body.String())
	}
	w := call("GET", "/api/chat/turns", "", true, true)
	var reply struct {
		Turns []core.ChatTurn `json:"turns"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &reply); err != nil {
		t.Fatal(err)
	}
	if len(reply.Turns) != 1 || reply.Turns[0].Status != "queued" {
		t.Fatal(reply)
	}
	if w := call("DELETE", "/api/chat/messages/one", "", true, true); w.Code != 200 || !strings.Contains(w.Body.String(), `"status":"cancelled"`) {
		t.Fatal(w.Code, w.Body.String())
	}
	for i := 0; i < 20; i++ {
		if _, err := s.EnqueueChat(context.Background(), fmt.Sprintf("pending-%d", i), "Queued"); err != nil {
			t.Fatal(err)
		}
	}
	if w := call("POST", "/api/chat/messages", `{"id":"extra","message":"Hello"}`, true, true); w.Code != 429 {
		t.Fatal(w.Code, w.Body.String())
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if w := call("POST", "/api/chat/messages", `{"id":"uncertain","message":"Hello"}`, true, true); w.Code != 500 {
		t.Fatal("storage failure must not look definitely rejected", w.Code, w.Body.String())
	}
}
