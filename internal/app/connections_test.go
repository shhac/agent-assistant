package app

import (
	"context"
	"encoding/json"
	"github.com/shhac/agent-assistant/internal/config"
	"github.com/shhac/agent-assistant/internal/integrations/connections"
	"strings"
	"testing"
)

func TestCLIImportsMultipleAccountsWithoutCommissioning(t *testing.T) {
	a := testApp(t)
	cfg := a.Config()
	cfg.Linear.TeamIDs = []string{"legacy-team"}
	cfg.Linear.APIKeyEnv = "ASSISTANT_TEST_ABSENT_LINEAR_KEY"
	t.Setenv(cfg.Linear.APIKeyEnv, "")
	cfg.Connections = []config.Connection{{ID: "linear", Name: "Linear", Tool: "lin", Profiles: []string{"work", "personal"}}}
	if err := a.UpdateConfig(cfg); err != nil {
		t.Fatal(err)
	}
	a.connectionClient = connections.Client{Run: func(_ context.Context, name string, args []string) ([]byte, error) {
		if name != "lin" {
			t.Fatal(name)
		}
		if args[0] == "auth" {
			return []byte("{\"alias\":\"work\"}\n{\"alias\":\"personal\"}"), nil
		}
		return []byte("{\"id\":\"same-id\",\"identifier\":\"EX-1\",\"title\":\"Source task\",\"statusType\":\"started\"}\n{\"id\":\"finished\",\"title\":\"Finished task\",\"statusType\":\"completed\"}"), nil
	}}
	for i := 0; i < 2; i++ {
		if err := a.SyncLinear(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	s, err := a.Core.Snapshot(context.Background())
	if err != nil || len(s.Projects) != 2 || len(s.Agents) != 0 {
		t.Fatal(s, err)
	}
	if s.Projects[0].SourceID == s.Projects[1].SourceID {
		t.Fatal("accounts collided")
	}
	for _, p := range s.Projects {
		if !strings.Contains(p.AcceptanceCriteria, "Read the full source issue") {
			t.Fatal(p)
		}
	}
	raw, _, err := a.context(context.Background())
	if err != nil || !strings.Contains(string(raw), "connections") {
		t.Fatal(string(raw), err)
	}
}
func TestConnectionToolsRespectDemoAndStrictArgs(t *testing.T) {
	a := testApp(t)
	a.Demo = true
	a.connectionClient = connections.Client{Run: func(context.Context, string, []string) ([]byte, error) {
		t.Fatal("demo spawned a CLI")
		return nil, nil
	}}
	if _, err := a.Execute(context.Background(), "query_connection", json.RawMessage(`{"connection_id":"x"}`)); err == nil {
		t.Fatal("demo queried")
	}
	if _, err := a.Execute(context.Background(), "list_connections", json.RawMessage(`{"shell":"echo"}`)); err == nil {
		t.Fatal("unknown args accepted")
	}
	if _, err := a.DiscoverConnectionProfiles(context.Background(), "lin"); err != nil {
		t.Fatal(err)
	}
}
