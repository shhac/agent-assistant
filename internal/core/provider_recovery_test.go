package core

import (
	"strings"
	"testing"
	"time"
)

func providerWaitingAgent(t *testing.T, s *Service, due time.Time) Agent {
	t.Helper()
	p := newProject(t, s)
	a := delegate(t, s, p, "")
	start(t, s, a)
	a, err := s.UpdateAgent(testContext, a.ID, AgentUpdate{Status: "retry_wait", Summary: "Provider overloaded; retry scheduled", RetryAt: due, ProviderFailures: 2, ProviderFailureKind: "overloaded", ExternalID: "external-" + a.ID})
	if err != nil {
		t.Fatal(err)
	}
	return a
}
func TestProviderRecoveryDueTimeAndGenericBudgetAreIndependent(t *testing.T) {
	s, cfg := fixture(t)
	due := s.now().Add(time.Minute)
	a := providerWaitingAgent(t, s, due)
	if _, err := s.PrepareResume(testContext, a.ID); err == nil {
		t.Fatal("provider retry started early")
	}
	if err := s.BeginInstruction(testContext, a.ID); err == nil {
		t.Fatal("message bypassed scheduled provider retry")
	}
	if err := s.store.update(testContext, func(v *Snapshot) error { agent(v, a.ID).Recoveries = cfg.Limits.MaxRecoveries; return nil }); err != nil {
		t.Fatal(err)
	}
	s.now = func() time.Time { return due }
	resumed, err := s.PrepareResume(testContext, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if resumed.Status != "resuming" || resumed.Recoveries != cfg.Limits.MaxRecoveries || !strings.Contains(resumed.ResumeKey, ":provider-retry:2:") {
		t.Fatalf("provider retry spent general recovery allowance: %+v", resumed)
	}
	if _, err = s.PrepareResume(testContext, a.ID); err == nil {
		t.Fatal("duplicated a pending resume")
	}
	if err = s.MarkDispatched(testContext, a.ID, a.ExternalID); err != nil {
		t.Fatal(err)
	}
	snapshot, _ := s.Snapshot(testContext)
	if snapshot.Agents[0].ResumeKey != "" {
		t.Fatal("acknowledged resume left an uncertain intent")
	}
}
func TestProviderRecoveryRespectsPauseOwnerHoldAndCapacity(t *testing.T) {
	for _, gate := range []string{"daemon", "owner-pause", "owner-stop", "capacity"} {
		t.Run(gate, func(t *testing.T) {
			s, cfg := fixture(t)
			a := providerWaitingAgent(t, s, s.now().Add(-time.Second))
			switch gate {
			case "daemon":
				s.SetPaused(testContext, true)
			case "owner-pause":
				s.PrepareOwnerControl(testContext, a.ID, "pause", "pause-provider")
			case "owner-stop":
				s.PrepareOwnerControl(testContext, a.ID, "stop", "stop-provider")
			case "capacity":
				cfg.Limits.MaxAgents = 1
				s.UpdateConfig(cfg)
				p := newProject(t, s)
				other := delegate(t, s, p, "")
				start(t, s, other)
			}
			if _, err := s.PrepareResume(testContext, a.ID); err == nil {
				t.Fatal("provider recovery escaped " + gate)
			}
		})
	}
}
func TestInterruptedCleanupStillHonoursProviderRetryAfter(t *testing.T) {
	s, _ := fixture(t)
	a := providerWaitingAgent(t, s, s.now().Add(time.Hour))
	if _, err := s.UpdateAgent(testContext, a.ID, AgentUpdate{Status: "interrupted", Summary: "Artifact collection failed after provider rejection", RetryAt: a.RetryAt, ProviderFailures: a.ProviderFailures, ProviderFailureKind: a.ProviderFailureKind}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PrepareResume(testContext, a.ID); err == nil {
		t.Fatal("cleanup interruption bypassed provider Retry-After")
	}
}

func TestModelBlockedWorkerNeedsExplicitOwnerResume(t *testing.T) {
	for _, kind := range []string{"authentication", "context_limit", "unknown"} {
		t.Run(kind, func(t *testing.T) {
			s, _ := fixture(t)
			p := newProject(t, s)
			a := delegate(t, s, p, "")
			start(t, s, a)
			if _, err := s.UpdateAgent(testContext, a.ID, AgentUpdate{Status: "blocked", Summary: "Model stopped; inspect saved work", ProviderFailureKind: kind}); err != nil {
				t.Fatal(err)
			}
			if _, err := s.PrepareResume(testContext, a.ID); err == nil {
				t.Fatal("automatic resume bypassed model block")
			}
			if err := s.BeginInstruction(testContext, a.ID); err == nil {
				t.Fatal("instruction woke model-blocked worker")
			}
			resumed, send, err := s.PrepareOwnerControl(testContext, a.ID, "resume", "owner-model-resume")
			if err != nil || !send || resumed.Status != "resuming" {
				t.Fatalf("explicit owner resume unavailable: %+v %v %v", resumed, send, err)
			}
		})
	}
}
