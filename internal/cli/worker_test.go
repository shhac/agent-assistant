package cli

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shhac/agent-assistant/internal/config"
	"github.com/shhac/agent-assistant/internal/integrations/worker"
	"github.com/shhac/agent-assistant/internal/quota"
	"github.com/shhac/lib-agent-harness/session"
)

func quotaFixture(used float64) session.QuotaSnapshot {
	observation := session.Observation{Quality: session.Measured, ObservedAt: time.Now()}
	return session.QuotaSnapshot{Observation: observation, Complete: true, Windows: []session.QuotaWindow{{Observation: observation, ID: "codex/primary", Scope: "codex", UsedPercent: &used}}}
}

// A separately operated broker is measured by the same account policy as a
// managed one. Its configuration is its own, but the rules are not a copy.
func TestStandaloneBrokerHonoursTheConfiguredHeadroomPolicy(t *testing.T) {
	cfg := config.Default()
	profile := cfg.WorkerModel
	for _, tc := range []struct {
		name        string
		used        float64
		unavailable bool
		onMissing   string
		threshold   int
		held        bool
	}{
		{"headroom", 10, false, "allow", 90, false},
		{"consumed", 95, false, "allow", 90, true},
		{"disabled", 95, false, "allow", 0, false},
		{"unavailable allows by default", 0, true, "allow", 90, false},
		{"unavailable pauses when asked", 0, true, "pause", 90, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			policy := cfg
			policy.Limits.WorkerUsage.CodexMaxUsedPercent = tc.threshold
			policy.Limits.WorkerUsage.OnUnavailable = tc.onMissing
			inspected := 0
			meter := &quota.Meter{Inspect: func(context.Context, session.Options) (session.Inspection, error) {
				inspected++
				if tc.unavailable {
					return session.Inspection{}, errors.New("CLI unavailable")
				}
				return session.Inspection{Quota: quotaFixture(tc.used)}, nil
			}}
			err := standaloneAdmission(policy, profile, meter)(context.Background())
			var held *worker.HoldError
			if errors.As(err, &held) != tc.held {
				t.Fatalf("admission result %v", err)
			}
			if tc.threshold == 0 && inspected != 0 {
				t.Fatal("a disabled gate inspected the CLI login")
			}
			if tc.held && held.Hold.Kind != worker.HoldSubscriptionQuota {
				t.Fatalf("wrong hold kind %q", held.Hold.Kind)
			}
		})
	}
}

// An engine whose subscription cannot be inspected locally is not silently
// exempt. It is unmeasured, and follows the same unavailable-usage policy the
// daemon applies, so "pause" really does mean nothing runs unmeasured.
func TestStandaloneBrokerTreatsUninspectableEnginesAsUnmeasured(t *testing.T) {
	cfg := config.Default()
	profile := cfg.WorkerModel
	profile.Engine = "openai-compatible"
	meter := &quota.Meter{Inspect: func(context.Context, session.Options) (session.Inspection, error) {
		t.Fatal("an uninspectable engine contacted a CLI login")
		return session.Inspection{}, nil
	}}
	if err := standaloneAdmission(cfg, profile, meter)(context.Background()); err != nil {
		t.Fatal("the default allow policy blocked an unmeasured engine", err)
	}
	cfg.Limits.WorkerUsage.OnUnavailable = "pause"
	err := standaloneAdmission(cfg, profile, meter)(context.Background())
	var held *worker.HoldError
	if !errors.As(err, &held) || !held.Hold.OwnerAction {
		t.Fatalf("pause policy let an unmeasured engine run: %v", err)
	}
}
