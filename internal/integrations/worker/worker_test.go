package worker

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func request() StartRequest {
	return StartRequest{DispatchKey: "dispatch-one", AgentID: "agent-one", ProjectID: "project-one", Role: "worker", Task: "Implement the agreed change", AcceptanceCriteria: "A passing review", Capabilities: []string{"implement"}}
}
func TestStartKeepsStableIdentityAndProhibitions(t *testing.T) {
	calls := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "POST" || r.URL.Path != "/runs" || r.Header.Get("Idempotency-Key") != "dispatch-one" {
			t.Error("incorrect dispatch contract")
		}
		var in StartRequest
		_ = json.NewDecoder(r.Body).Decode(&in)
		if strings.Join(in.Prohibitions, ",") != "deployment,production_data_access,purchases" {
			t.Errorf("lost prohibitions: %v", in.Prohibitions)
		}
		_, _ = w.Write([]byte(`{"id":"external-one","dispatch_key":"dispatch-one","status":"running"}`))
	}))
	defer s.Close()
	c, _ := New(Config{Endpoint: s.URL, Capabilities: []string{"implement"}})
	in := request()
	in.Prohibitions = []string{}
	run, err := c.Start(context.Background(), in)
	if err != nil || run.ID != "external-one" || calls != 1 {
		t.Fatalf("dispatch failed: %#v %v", run, err)
	}
}
func TestManagerOnlyGetsCoordinationTools(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in StartRequest
		_ = json.NewDecoder(r.Body).Decode(&in)
		if strings.Join(in.Capabilities, ",") != "coordinate" || strings.Join(in.DelegationCapabilities, ",") != "coordinate,implement" {
			t.Errorf("manager authority mixed with tools: %#v", in)
		}
		_, _ = w.Write([]byte(`{"id":"external-one","dispatch_key":"dispatch-one","status":"running"}`))
	}))
	defer s.Close()
	c, _ := New(Config{Endpoint: s.URL, Capabilities: []string{"coordinate", "implement"}})
	in := request()
	in.Role = "manager"
	in.Capabilities = []string{"coordinate", "implement"}
	if _, err := c.Start(context.Background(), in); err != nil {
		t.Fatal(err)
	}
}
func TestOutOfScopeStartNeverContactsBroker(t *testing.T) {
	calls := 0
	s := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
	defer s.Close()
	c, _ := New(Config{Endpoint: s.URL, Capabilities: []string{"review"}})
	if _, err := c.Start(context.Background(), request()); err == nil || calls != 0 {
		t.Fatal("out of scope work was dispatched")
	}
	if _, err := New(Config{Endpoint: s.URL, Capabilities: []string{"deploy"}}); err == nil {
		t.Fatal("prohibited capability accepted")
	}
}
func TestUncertainStartNeverRetries(t *testing.T) {
	calls := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(503)
		_, _ = w.Write([]byte("secret"))
	}))
	defer s.Close()
	c, _ := New(Config{Endpoint: s.URL, Capabilities: []string{"implement"}})
	_, err := c.Start(context.Background(), request())
	if !errors.Is(err, ErrUncertain) || calls != 1 {
		t.Fatalf("uncertainty not preserved: %v %d", err, calls)
	}
}
func TestFindAndRunIdentity(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/runs" || r.URL.Query().Get("dispatch_key") != "dispatch-one" {
			t.Error("missing reconciliation key")
		}
		_, _ = w.Write([]byte(`{"id":"external","dispatch_key":"wrong-key","status":"running"}`))
	}))
	defer s.Close()
	c, _ := New(Config{Endpoint: s.URL, Capabilities: []string{"implement"}})
	if _, err := c.Find(context.Background(), "dispatch-one"); err == nil {
		t.Fatal("accepted another dispatch result")
	}
}
func TestMalformedSuccessfulStartIsUncertain(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{"status":"running"}`)) }))
	defer s.Close()
	c, _ := New(Config{Endpoint: s.URL, Capabilities: []string{"implement"}})
	_, err := c.Start(context.Background(), request())
	if !errors.Is(err, ErrUncertain) {
		t.Fatalf("missing receipt must be uncertain: %v", err)
	}
}
func TestSendRequiresIdempotencyAndPreservesMessage(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/runs/external/messages" || r.Header.Get("Idempotency-Key") != "decision-one" {
			t.Error("unsafe message path/key")
		}
		var in map[string]string
		_ = json.NewDecoder(r.Body).Decode(&in)
		if in["message"] != "Use the recommended option" {
			t.Error("message lost")
		}
		_, _ = w.Write([]byte(`{"id":"external","dispatch_key":"dispatch-one","status":"running"}`))
	}))
	defer s.Close()
	c, _ := New(Config{Endpoint: s.URL, Capabilities: []string{"coordinate"}})
	if _, err := c.Send(context.Background(), "external", "decision-one", "Use the recommended option"); err != nil {
		t.Fatal(err)
	}
}
