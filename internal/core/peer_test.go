package core

import "testing"

func TestPeerMessageSourceUsesCurrentAuthorityWithoutWakingSender(t *testing.T) {
	s, cfg := fixture(t)
	p := newProject(t, s)
	a := delegate(t, s, p, "")
	if _, err := s.PeerMessageSource(testContext, a.ID); err == nil {
		t.Fatal("queued sender permitted")
	}
	start(t, s, a)
	if _, err := s.UpdateAgent(testContext, a.ID, AgentUpdate{Status: "waiting", Summary: "Waiting for a peer"}); err != nil {
		t.Fatal(err)
	}
	got, err := s.PeerMessageSource(testContext, a.ID)
	if err != nil || got.Status != "waiting" || got.ExternalID == "" {
		t.Fatalf("sender: %+v, %v", got, err)
	}
	snapshot, err := s.Snapshot(testContext)
	if err != nil || snapshot.Agents[0].Status != "waiting" {
		t.Fatal("source check woke sender", err)
	}
	cfg.Workers[0].Capabilities = []string{"coordinate"}
	s.UpdateConfig(cfg)
	if _, err := s.PeerMessageSource(testContext, a.ID); err == nil {
		t.Fatal("revoked sender permitted")
	}
}

func TestPeerMessageSourceRejectsPausedAndInterruptedSessions(t *testing.T) {
	s, _ := fixture(t)
	a := delegate(t, s, newProject(t, s), "")
	start(t, s, a)
	if err := s.SetPaused(testContext, true); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PeerMessageSource(testContext, a.ID); err == nil {
		t.Fatal("paused sender permitted")
	}
	if err := s.SetPaused(testContext, false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateAgent(testContext, a.ID, AgentUpdate{Status: "interrupted", Summary: "Interrupted"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PeerMessageSource(testContext, a.ID); err == nil {
		t.Fatal("interrupted sender permitted")
	}
	if _, err := s.PeerMessageSource(testContext, "missing"); err == nil {
		t.Fatal("unknown sender permitted")
	}
}
