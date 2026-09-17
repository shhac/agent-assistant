package core

import (
	"errors"
	"testing"
	"time"
)

func queueFixture(t *testing.T, ids ...string) *Service {
	t.Helper()
	s, _ := fixture(t)
	for _, id := range ids {
		if _, err := s.EnqueueChat(testContext, id, "message "+id); err != nil {
			t.Fatal(err)
		}
	}
	return s
}

func queuedIDs(t *testing.T, s *Service) []string {
	t.Helper()
	turns, err := s.ChatTurns(testContext)
	if err != nil {
		t.Fatal(err)
	}
	out := []string{}
	for _, turn := range turns {
		if turn.Status == "queued" {
			out = append(out, turn.ID)
		}
	}
	return out
}

func TestHoldBlocksItsTurnAndEverythingAfterIt(t *testing.T) {
	s := queueFixture(t, "a", "b", "c")
	if _, err := s.HoldChat(testContext, "b", "editing", time.Minute); err != nil {
		t.Fatal(err)
	}
	// The turn ahead of the hold is unaffected.
	started, err := s.StartNextChat(testContext)
	if err != nil {
		t.Fatal(err)
	}
	if started.ID != "a" {
		t.Fatalf("started %q, want the turn ahead of the hold", started.ID)
	}
	if err := s.FinishChat(testContext, "a", "completed", "reply", ""); err != nil {
		t.Fatal(err)
	}
	// The held turn, and the one behind it, do not start.
	if _, err := s.StartNextChat(testContext); !errors.Is(err, ErrChatHeld) {
		t.Fatalf("started a held turn: %v", err)
	}
}

func TestHoldLapsesAndTheQueueResumesOnItsOwn(t *testing.T) {
	s := queueFixture(t, "a")
	if _, err := s.HoldChat(testContext, "a", "editing", time.Minute); err != nil {
		t.Fatal(err)
	}
	if _, err := s.StartNextChat(testContext); !errors.Is(err, ErrChatHeld) {
		t.Fatal("a live hold did not block the queue")
	}
	// A client that stops refreshing cannot hold the queue indefinitely.
	s.now = func() time.Time { return time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC).Add(MaxChatHold * 2) }
	started, err := s.StartNextChat(testContext)
	if err != nil {
		t.Fatalf("the queue did not resume after the lease lapsed: %v", err)
	}
	if started.ID != "a" {
		t.Fatalf("started %q", started.ID)
	}
}

// A hold naming a turn that has since started or been cancelled must not
// strand everything behind it.
func TestStaleHoldBlocksNothing(t *testing.T) {
	s := queueFixture(t, "a", "b")
	if _, err := s.HoldChat(testContext, "a", "editing", time.Minute); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CancelChat(testContext, "a"); err != nil {
		t.Fatal(err)
	}
	started, err := s.StartNextChat(testContext)
	if err != nil {
		t.Fatalf("a hold on a cancelled turn stranded the queue: %v", err)
	}
	if started.ID != "b" {
		t.Fatalf("started %q, want b", started.ID)
	}
}

func TestOneEditorHoldsTheQueueAtATime(t *testing.T) {
	s := queueFixture(t, "a", "b")
	if _, err := s.HoldChat(testContext, "a", "editing", time.Minute); err != nil {
		t.Fatal(err)
	}
	if _, err := s.HoldChat(testContext, "b", "editing", time.Minute); !errors.Is(err, ErrConflict) {
		t.Fatalf("a second editor took the queue: %v", err)
	}
	// Refreshing your own lease is not stealing it.
	if _, err := s.HoldChat(testContext, "a", "editing", time.Minute); err != nil {
		t.Fatalf("could not refresh an existing lease: %v", err)
	}
	if err := s.ReleaseChatHold(testContext, "a"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.HoldChat(testContext, "b", "editing", time.Minute); err != nil {
		t.Fatalf("the queue stayed held after release: %v", err)
	}
}

func TestEditKeepsIdentityAndMovesRevision(t *testing.T) {
	s := queueFixture(t, "a")
	turns, _ := s.ChatTurns(testContext)
	before := turns[0]
	after, err := s.EditChatMessage(testContext, "a", "  corrected text  ", before.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if after.ID != before.ID {
		t.Fatal("editing changed the key that protects against duplicate delivery")
	}
	if after.Message != "corrected text" {
		t.Fatalf("message = %q", after.Message)
	}
	if after.Revision == before.Revision {
		t.Fatal("revision did not move, so stale text could still start")
	}
	// The revision the editor was opened against is now stale.
	if _, err := s.EditChatMessage(testContext, "a", "again", before.Revision); !errors.Is(err, ErrConflict) {
		t.Fatalf("a stale edit was applied: %v", err)
	}
}

// The owner can open an editor at the moment the daemon starts the turn. That
// race is reported, never applied to a running turn.
func TestEditLosingTheRaceIsRefused(t *testing.T) {
	s := queueFixture(t, "a")
	turns, _ := s.ChatTurns(testContext)
	revision := turns[0].Revision
	if _, err := s.StartNextChat(testContext); err != nil {
		t.Fatal(err)
	}
	_, err := s.EditChatMessage(testContext, "a", "too late", revision)
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("edited a running turn: %v", err)
	}
	turns, _ = s.ChatTurns(testContext)
	if turns[0].Message != "message a" {
		t.Fatalf("the running turn's text changed to %q", turns[0].Message)
	}
}

func TestReorderSetsTheWholeOrderAtOnce(t *testing.T) {
	s := queueFixture(t, "a", "b", "c")
	v, _ := s.store.Snapshot(testContext)
	if _, err := s.ReorderChat(testContext, []string{"a", "c", "b"}, v.ChatQueueRevision); err != nil {
		t.Fatal(err)
	}
	if got := queuedIDs(t, s); got[0] != "a" || got[1] != "c" || got[2] != "b" {
		t.Fatalf("order = %v", got)
	}
	started, err := s.StartNextChat(testContext)
	if err != nil {
		t.Fatal(err)
	}
	if started.ID != "a" {
		t.Fatalf("started %q", started.ID)
	}
	if err := s.FinishChat(testContext, "a", "completed", "reply", ""); err != nil {
		t.Fatal(err)
	}
	next, err := s.StartNextChat(testContext)
	if err != nil {
		t.Fatal(err)
	}
	if next.ID != "c" {
		t.Fatalf("ran %q, not the order the owner set", next.ID)
	}
}

func TestReorderAgainstAStaleQueueIsRefused(t *testing.T) {
	s := queueFixture(t, "a", "b")
	v, _ := s.store.Snapshot(testContext)
	stale := v.ChatQueueRevision
	if _, err := s.EnqueueChat(testContext, "c", "message c"); err != nil {
		t.Fatal(err)
	}
	for _, order := range [][]string{{"b", "a"}, {"b", "a", "c"}} {
		if _, err := s.ReorderChat(testContext, order, stale); !errors.Is(err, ErrConflict) {
			t.Fatalf("a reorder decided against a stale queue was applied: %v", err)
		}
	}
	// Naming an unknown or duplicated turn is the same kind of refusal.
	v, _ = s.store.Snapshot(testContext)
	for _, order := range [][]string{{"a", "b", "gone"}, {"a", "a", "b"}} {
		if _, err := s.ReorderChat(testContext, order, v.ChatQueueRevision); !errors.Is(err, ErrConflict) {
			t.Fatalf("accepted %v", order)
		}
	}
	if got := queuedIDs(t, s); got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Fatalf("a refused reorder still changed the queue: %v", got)
	}
}

// A turn that has already started keeps its place; only queued turns move.
func TestReorderLeavesStartedTurnsAlone(t *testing.T) {
	s := queueFixture(t, "a", "b", "c")
	if _, err := s.StartNextChat(testContext); err != nil {
		t.Fatal(err)
	}
	v, _ := s.store.Snapshot(testContext)
	if _, err := s.ReorderChat(testContext, []string{"c", "b"}, v.ChatQueueRevision); err != nil {
		t.Fatal(err)
	}
	turns, _ := s.ChatTurns(testContext)
	if turns[0].ID != "a" || turns[0].Status != "running" {
		t.Fatalf("the running turn moved: %+v", turns[0])
	}
	if got := queuedIDs(t, s); got[0] != "c" || got[1] != "b" {
		t.Fatalf("queued order = %v", got)
	}
}
