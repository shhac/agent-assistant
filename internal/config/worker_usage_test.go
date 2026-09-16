package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkerUsageDefaultsForExistingConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"limits":{"max_agents":7}}`), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	want := WorkerUsage{CodexMaxUsedPercent: 90, ClaudeMaxUsedPercent: 90, OnUnavailable: "allow"}
	if got.Limits.WorkerUsage != want || got.Limits.MaxAgents != 7 {
		t.Fatalf("existing configuration did not receive usage defaults: %+v", got.Limits)
	}
}

func TestWorkerUsagePartialOverrideAndDisabledThresholdSurviveRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"limits":{"worker_usage":{"codex_max_used_percent":0,"on_unavailable":"pause"}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	want := WorkerUsage{CodexMaxUsedPercent: 0, ClaudeMaxUsedPercent: 90, OnUnavailable: "pause"}
	if got.Limits.WorkerUsage != want {
		t.Fatalf("explicit zero or omitted defaults lost: %+v", got.Limits.WorkerUsage)
	}
	if err := Save(path, got); err != nil {
		t.Fatal(err)
	}
	reloaded, err := Load(path)
	if err != nil || reloaded.Limits.WorkerUsage != want {
		t.Fatalf("usage policy changed after save: %+v, %v", reloaded.Limits.WorkerUsage, err)
	}
}

func TestWorkerUsageValidation(t *testing.T) {
	for _, tc := range []struct {
		name         string
		usage        WorkerUsage
		invalidField string
	}{
		{"disabled", WorkerUsage{0, 0, "allow"}, ""},
		{"full quota", WorkerUsage{100, 100, "pause"}, ""},
		{"negative codex", WorkerUsage{-1, 90, "allow"}, "codex_max_used_percent"},
		{"large codex", WorkerUsage{101, 90, "allow"}, "codex_max_used_percent"},
		{"negative claude", WorkerUsage{90, -1, "allow"}, "claude_max_used_percent"},
		{"large claude", WorkerUsage{90, 101, "allow"}, "claude_max_used_percent"},
		{"missing policy", WorkerUsage{90, 90, ""}, "on_unavailable"},
		{"unknown policy", WorkerUsage{90, 90, "deny"}, "on_unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Default()
			cfg.Limits.WorkerUsage = tc.usage
			err := cfg.Validate()
			if tc.invalidField == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), "limits.worker_usage."+tc.invalidField) {
				t.Fatalf("wanted field-specific validation for %s, got %v", tc.invalidField, err)
			}
		})
	}
}
