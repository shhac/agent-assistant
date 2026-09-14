package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigAtomicPrivateRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "nested", "config.json")
	c := Default()
	c.Assistant.Name = "Juniper"
	if err := Save(p, c); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatal(info.Mode())
	}
	got, err := Load(p)
	if err != nil || got.Assistant.Name != "Juniper" {
		t.Fatal(got, err)
	}
	c.Limits.MaxAgents = 0
	if err = Save(p, c); err == nil {
		t.Fatal("invalid replacement accepted")
	}
	got, err = Load(p)
	if err != nil || got.Limits.MaxAgents != 4 {
		t.Fatal("invalid save changed file")
	}
}
func TestUnknownFieldsAndTrailingJSONRejected(t *testing.T) {
	for _, body := range []string{`{"unexpected":true}`, `{} {}`, `{"limits":{"max_agents":0}}`} {
		p := filepath.Join(t.TempDir(), "config.json")
		os.WriteFile(p, []byte(body), 0600)
		if _, err := Load(p); err == nil {
			t.Fatalf("accepted %s", body)
		}
	}
}
func TestBoundariesCannotBeConfiguredAway(t *testing.T) {
	for _, mutate := range []func(*Config){func(c *Config) { c.Dashboard.Addr = "0.0.0.0:8340" }, func(c *Config) { c.Dashboard.Tailscale = "funnel" }, func(c *Config) { c.Dashboard.Tailscale = "serve" }, func(c *Config) { c.Model.BaseURL = "https://secret:password@example.com" }, func(c *Config) { c.Model.BaseURL = "http://example.com" }, func(c *Config) { c.Model.APIKeyEnv = "raw token!" }, func(c *Config) {
		c.Workers = []Worker{{ID: "bad", Endpoint: "https://example.com", Capabilities: []string{"purchase"}}}
	}} {
		c := Default()
		mutate(&c)
		if err := c.Validate(); err == nil {
			t.Fatal("unsafe configuration accepted")
		}
	}
}
func TestXDGPaths(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "configuration"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(root, "state"))
	p, err := Paths()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(p.Config, filepath.Join(root, "configuration")) || !strings.HasPrefix(p.State, filepath.Join(root, "state")) {
		t.Fatal(p)
	}
}
