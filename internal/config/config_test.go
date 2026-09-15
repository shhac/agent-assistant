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

func TestModelDefaultsAndIndependentProfiles(t *testing.T) {
	c := Default()
	for name, profile := range map[string]Model{"assistant": c.Model, "worker": c.WorkerModel} {
		if profile.Engine != "codex" || profile.Model != map[string]string{"assistant": "gpt-6-astra", "worker": "gpt-5.6-terra"}[name] || profile.Effort != "high" || profile.CodexBin != "codex" {
			t.Fatalf("%s defaults: %+v", name, profile)
		}
	}
	c.WorkerModel.Model = "worker-model"
	c.WorkerModel.Effort = "low"
	path := filepath.Join(t.TempDir(), "config.json")
	if err := Save(path, c); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Model != c.Model || got.WorkerModel != c.WorkerModel {
		t.Fatalf("profiles lost independence: %+v", got)
	}
}
func TestLegacyAPIConfigRetainsProviderAndBillingPath(t *testing.T) {
	for _, body := range []string{`{"model":{"model":"existing-model","base_url":"https://provider.example/v1","api_key_env":"EXISTING_KEY"}}`, `{"model":{}}`} {
		path := filepath.Join(t.TempDir(), "config.json")
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		got, err := Load(path)
		if err != nil {
			t.Fatal(err)
		}
		if got.Model.Engine != "openai-compatible" || got.Model.Effort != "" || got.WorkerModel != got.Model {
			t.Fatalf("legacy profile unexpectedly migrated: %+v", got)
		}
		if strings.Contains(body, "existing-model") && (got.Model.Model != "existing-model" || got.Model.APIKeyEnv != "EXISTING_KEY" || got.Model.BaseURL != "https://provider.example/v1") {
			t.Fatal(got.Model)
		}
		if body == `{"model":{}}` && got.Model.Model != "" {
			t.Fatal("unconfigured legacy model enabled inference")
		}
	}
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"assistant":{"name":"Juniper"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil || got.Model.Engine != "codex" || got.Model.Model != "gpt-6-astra" || got.Model.Effort != "high" {
		t.Fatal(got.Model, err)
	}
}
func TestInvalidEngineAndEffortRejected(t *testing.T) {
	for _, mutate := range []func(*Config){
		func(c *Config) { c.Model.Engine = "unknown" },
		func(c *Config) { c.Model.Effort = "maximumish" },
		func(c *Config) { c.WorkerModel.Engine = "unknown" },
		func(c *Config) { c.WorkerModel.Effort = "maximumish" },
		func(c *Config) { c.Model.CodexBin = "" },
	} {
		c := Default()
		mutate(&c)
		if err := c.Validate(); err == nil {
			t.Fatal("invalid engine configuration accepted")
		}
	}
}

func TestLegacyAssistantTokenCapDoesNotChangeWorkerCap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"model":{"model":"existing-model","max_tokens":65536}}`), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Model.MaxTokens != 65536 || got.WorkerModel.MaxTokens != 4096 {
		t.Fatalf("legacy limits changed: %+v", got)
	}
}

func TestNamespaceKeepsLegacyConfigAndStateTogether(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(root, "state"))
	fresh, err := Paths()
	if err != nil || !strings.Contains(fresh.Config, Namespace) || !strings.Contains(fresh.State, Namespace) {
		t.Fatal(fresh, err)
	}
	legacy := filepath.Join(root, "state", "agent-assistant", "state.db")
	os.MkdirAll(filepath.Dir(legacy), 0700)
	os.WriteFile(legacy, []byte("state"), 0600)
	got, err := Paths()
	if err != nil || got.State != legacy || got.Config != filepath.Join(root, "config", "agent-assistant", "config.json") {
		t.Fatal(got, err)
	}
	os.MkdirAll(filepath.Dir(fresh.Config), 0700)
	os.WriteFile(fresh.Config, []byte("{}"), 0600)
	if _, err = Paths(); err == nil {
		t.Fatal("ambiguous namespaces accepted")
	}
}
func TestConnectionProfilesAndIdentityValidation(t *testing.T) {
	c := Default()
	c.Connections = []Connection{{ID: "work", Name: "Work", Tool: "lin", Profiles: []string{"first", "second"}}}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*Config){func(c *Config) { c.Connections[0].Profiles = []string{"first", "first"} }, func(c *Config) { c.Connections[0].Tool = "sh" }, func(c *Config) { c.Assistant.Avatar.Accent = "url(https://example.com)" }, func(c *Config) { c.Assistant.Theme = "arbitrary" }} {
		d := Default()
		d.Connections = []Connection{{ID: "work", Name: "Work", Tool: "lin", Profiles: []string{"first"}}}
		mutate(&d)
		if err := d.Validate(); err == nil {
			t.Fatal("invalid config accepted")
		}
	}
}
