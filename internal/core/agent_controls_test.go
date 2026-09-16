package core

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestOwnerPauseHoldsCapacityUntilConfirmedAndRequiresExplicitResume(t *testing.T) {
	s, _ := fixture(t)
	p := newProject(t, s)
	a := delegate(t, s, p, "")
	start(t, s, a)
	paused, send, err := s.PrepareOwnerControl(testContext, a.ID, "pause", "pause-key")
	if err != nil || !send || paused.Status != "pause_requested" {
		t.Fatal(paused, send, err)
	}
	v, _ := s.Snapshot(testContext)
	if executingCount(&v) != 1 {
		t.Fatal("released active capacity before cleanup")
	}
	if err = s.BeginInstruction(testContext, a.ID); err == nil {
		t.Fatal("instruction bypassed owner hold")
	}
	if _, err = s.UpdateAgent(testContext, a.ID, AgentUpdate{Status: "paused", Summary: "Cleanup confirmed"}); err != nil {
		t.Fatal(err)
	}
	v, _ = s.Snapshot(testContext)
	if executingCount(&v) != 0 {
		t.Fatal("paused worker retained execution slot")
	}
	if _, err = s.PrepareResume(testContext, a.ID); err == nil {
		t.Fatal("automatic recovery resumed paused worker")
	}
	again, send, err := s.PrepareOwnerControl(testContext, a.ID, "pause", "pause-key")
	if err != nil || send || again.Status != "paused" {
		t.Fatal("control retry reissued operation", again, send, err)
	}
	resumed, send, err := s.PrepareOwnerControl(testContext, a.ID, "resume", "resume-key")
	if err != nil || !send || resumed.Status != "resuming" {
		t.Fatal(resumed, send, err)
	}
}
func TestQueuedPauseCannotDispatchUntilOwnerResumes(t *testing.T) {
	s, _ := fixture(t)
	p := newProject(t, s)
	a := delegate(t, s, p, "")
	paused, send, err := s.PrepareOwnerControl(testContext, a.ID, "pause", "pause-key")
	if err != nil || send || paused.Status != "paused" {
		t.Fatal(paused, send, err)
	}
	if _, err = s.BeginDispatch(testContext, a.ID); err == nil {
		t.Fatal("queued pause dispatched")
	}
	resumed, send, err := s.PrepareOwnerControl(testContext, a.ID, "resume", "resume-key")
	if err != nil || send || resumed.Status != "queued" {
		t.Fatal(resumed, send, err)
	}
}
func TestWorkerConversationBoundedPaginationIsPrivateAndScoped(t *testing.T) {
	s, _ := fixture(t)
	p := newProject(t, s)
	a := delegate(t, s, p, "")
	other := delegate(t, s, p, "")
	for i := 0; i < 105; i++ {
		if err := s.RecordAgentConversation(testContext, a.ID, fmt.Sprint(i), "report", "worker_to_daemon", fmt.Sprintf("Report %03d", i)); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.RecordAgentConversation(testContext, other.ID, "private", "report", "worker_to_daemon", "Other worker message"); err != nil {
		t.Fatal(err)
	}
	page, err := s.AgentConversation(testContext, a.ID, 0, 0, 50)
	if err != nil || len(page.Messages) != 50 || !page.Truncated || page.Messages[0].Content != "Report 055" {
		t.Fatal(page, err)
	}
	earlier, err := s.AgentConversation(testContext, a.ID, 0, page.OldestSequence, 50)
	if err != nil || len(earlier.Messages) != 50 || earlier.Messages[0].Content != "Report 005" {
		t.Fatal(earlier, err)
	}
	after, err := s.AgentConversation(testContext, a.ID, page.NextCursor, 0, 50)
	if err != nil || len(after.Messages) != 0 {
		t.Fatal(after, err)
	}
	snapshot, _ := s.Snapshot(testContext)
	raw, _ := json.Marshal(snapshot)
	if strings.Contains(string(raw), "Report 055") || strings.Contains(string(raw), "Other worker message") {
		t.Fatal("conversation leaked into global state snapshot")
	}
	if err = s.RecordAgentConversation(testContext, a.ID, "long", "message", "daemon_to_worker", strings.Repeat("🪶", 4000)); err != nil {
		t.Fatal(err)
	}
	page, _ = s.AgentConversation(testContext, a.ID, 0, 0, 1)
	if len(page.Messages[0].Content) > 8300 || !json.Valid(mustMarshal(page)) {
		t.Fatal("unbounded or invalid message")
	}
}
func mustMarshal(v any) []byte { raw, _ := json.Marshal(v); return raw }

func TestConversationRetentionReportsLostHistoryEvenOnEmptyPoll(t *testing.T) {
	s, _ := fixture(t)
	p := newProject(t, s)
	a := delegate(t, s, p, "")
	// Seed retained history to exercise the actual append/sweep without 501
	// unrelated SQLite fsyncs in a retention policy test.
	if err := s.store.update(testContext, func(v *Snapshot) error {
		for i := int64(1); i <= 500; i++ {
			v.AgentConversation = append(v.AgentConversation, AgentConversationEntry{Sequence: i, ID: fmt.Sprint(i), AgentID: a.ID, Kind: "report", Direction: "worker_to_daemon", Content: "Report"})
		}
		v.ConversationSequence = 500
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordAgentConversation(testContext, a.ID, "new", "report", "worker_to_daemon", "New report"); err != nil {
		t.Fatal(err)
	}
	page, err := s.AgentConversation(testContext, a.ID, 501, 0, 50)
	if err != nil || !page.HistoryLimited || len(page.Messages) != 0 {
		t.Fatal("empty polling page hid dropped history", page, err)
	}
}

func TestOldWorkerReportCannotReleaseConcurrentOwnerHold(t *testing.T) {
	s, _ := fixture(t)
	p := newProject(t, s)
	a := delegate(t, s, p, "")
	start(t, s, a)
	if _, _, err := s.PrepareOwnerControl(testContext, a.ID, "pause", "pause-key"); err != nil {
		t.Fatal(err)
	}
	observed, err := s.UpdateAgent(testContext, a.ID, AgentUpdate{Status: "waiting", Summary: "Old report from before the pause"})
	if err != nil {
		t.Fatal(err)
	}
	if observed.Status != "pause_requested" || !holdsExecution(observed) {
		t.Fatal("stale in-flight report released owner pause", observed)
	}
}
