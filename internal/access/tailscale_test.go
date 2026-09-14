package access

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDoesNotTakeAnOccupiedRoute(t *testing.T) {
	called := false
	ts := Tailscale{Run: func(context.Context, ...string) ([]byte, error) {
		return []byte(`{"TCP":{"8443":{"HTTPS":true}},"Web":{"host:8443":{"Handlers":{"/":{"Proxy":"http://127.0.0.1:8330"}}}}}`), nil
	}, Wire: func(context.Context, string, int, string, string) (string, func() error, error) {
		called = true
		return "", nil, nil
	}}
	_, _, err := ts.Start(context.Background(), 8443, "127.0.0.1:8340", filepath.Join(t.TempDir(), "route.json"))
	if err == nil || called {
		t.Fatal("occupied route was mutated")
	}
}
func TestCleanupSkipsChangedRoute(t *testing.T) {
	current := []byte(`{}`)
	stopped := false
	ts := Tailscale{Run: func(context.Context, ...string) ([]byte, error) { return current, nil }, Wire: func(context.Context, string, int, string, string) (string, func() error, error) {
		current = []byte(`{"TCP":{"8443":{"HTTPS":true}}}`)
		return "https://host:8443", func() error { stopped = true; return nil }, nil
	}}
	record := filepath.Join(t.TempDir(), "route.json")
	_, cleanup, err := ts.Start(context.Background(), 8443, "127.0.0.1:8340", record)
	if err != nil {
		t.Fatal(err)
	}
	current = []byte(`{"TCP":{"8443":{"HTTPS":true}},"AllowFunnel":{"host:8443":true}}`)
	if cleanup() == nil || stopped {
		t.Fatal("changed route was removed")
	}
}
func TestRecoverOwnedRoute(t *testing.T) {
	current := []byte(`{"TCP":{"8443":{"HTTPS":true}}}`)
	r, _ := route(current, 8443)
	b, _ := json.Marshal(ownership{Address: "127.0.0.1:8340", Port: 8443, Route: r})
	record := filepath.Join(t.TempDir(), "route.json")
	_ = os.WriteFile(record, b, 0600)
	ts := Tailscale{Run: func(context.Context, ...string) ([]byte, error) { return current, nil }, Wire: func(context.Context, string, int, string, string) (string, func() error, error) {
		return "https://host:8443", func() error { return nil }, nil
	}}
	_, cleanup, err := ts.Start(context.Background(), 8443, "127.0.0.1:8340", record)
	if err != nil {
		t.Fatal(err)
	}
	if err = cleanup(); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(record); !os.IsNotExist(err) {
		t.Fatal("ownership was not removed")
	}
}
