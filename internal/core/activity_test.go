package core

import (
	"testing"
	"time"
)

// Recorded times come from several clocks, so a snapshot must order the feed by
// the recorded time rather than by the order entries happened to be appended.
func TestSnapshotOrdersActivityNewestFirst(t *testing.T) {
	s, _ := fixture(t)
	p := newProject(t, s)
	times := []time.Time{
		time.Date(2026, 9, 16, 0, 5, 0, 0, time.UTC),
		time.Date(2026, 9, 16, 16, 53, 0, 0, time.UTC),
		time.Date(2026, 9, 16, 13, 40, 0, 0, time.UTC),
	}
	summaries := []string{"midnight setup", "latest failure", "afternoon update"}
	for i := range times {
		s.now = func() time.Time { return times[i] }
		if err := s.RecordActivity(testContext, p.ID, "assistant.update", summaries[i]); err != nil {
			t.Fatal(err)
		}
	}
	v, err := s.Snapshot(testContext)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Activity) < 3 {
		t.Fatalf("expected the recorded activity, got %d entries", len(v.Activity))
	}
	if v.Activity[0].Summary != "latest failure" {
		t.Fatalf("newest entry should lead the feed, got %q", v.Activity[0].Summary)
	}
	for i := 1; i < len(v.Activity); i++ {
		if v.Activity[i].CreatedAt.After(v.Activity[i-1].CreatedAt) {
			t.Fatalf("entry %d is newer than the entry before it", i)
		}
	}
}

// Entries sharing a recorded time must still order deterministically, so a
// repeated snapshot does not reshuffle the feed under the owner.
func TestSnapshotActivityOrderIsStableForEqualTimes(t *testing.T) {
	s, _ := fixture(t)
	p := newProject(t, s)
	for _, summary := range []string{"first", "second", "third"} {
		if err := s.RecordActivity(testContext, p.ID, "assistant.update", summary); err != nil {
			t.Fatal(err)
		}
	}
	first, err := s.Snapshot(testContext)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Snapshot(testContext)
	if err != nil {
		t.Fatal(err)
	}
	for i := range first.Activity {
		if first.Activity[i].ID != second.Activity[i].ID {
			t.Fatalf("activity order changed between snapshots at %d", i)
		}
	}
}
