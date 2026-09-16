package core

import (
	"errors"
	"strings"
	"testing"
)

func TestCorrectingAMemoryKeepsWhatWasBelievedBefore(t *testing.T) {
	s, _ := fixture(t)
	original, err := s.RememberKind(testContext, "worker-model", "Worker model information is unavailable.", "observation", "assistant")
	if err != nil {
		t.Fatal(err)
	}
	corrected, err := s.Correct(testContext, original.ID, "The project worker runs Opus 5.", "observation")
	if err != nil {
		t.Fatal(err)
	}
	v, err := s.Snapshot(testContext)
	if err != nil {
		t.Fatal(err)
	}
	var before, after *Memory
	for i := range v.Memories {
		switch v.Memories[i].ID {
		case original.ID:
			before = &v.Memories[i]
		case corrected.ID:
			after = &v.Memories[i]
		}
	}
	if before == nil {
		t.Fatal("correcting a memory discarded the original")
	}
	if before.SupersededAt.IsZero() {
		t.Fatal("the original memory is not marked superseded")
	}
	if before.Content != "Worker model information is unavailable." {
		t.Fatal("the original memory was rewritten instead of superseded")
	}
	if after == nil || after.Supersedes != original.ID {
		t.Fatalf("the replacement does not record what it replaced: %+v", after)
	}
	if after.Kind != "observation" || after.Source != "assistant" {
		t.Fatalf("classification was not carried forward: %+v", after)
	}
}

// Activity is an audit trail the owner reads; a correction names the memory
// rather than restating what it now says.
func TestCorrectionActivityDoesNotRestateContent(t *testing.T) {
	s, _ := fixture(t)
	original, err := s.RememberKind(testContext, "tone", "Keep updates brief.", "preference", "owner")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Correct(testContext, original.ID, "Keep updates brief and lead with a recommendation.", ""); err != nil {
		t.Fatal(err)
	}
	v, _ := s.Snapshot(testContext)
	found := false
	for _, entry := range v.Activity {
		if entry.Kind != "memory.corrected" {
			continue
		}
		found = true
		if strings.Contains(entry.Summary, "lead with a recommendation") {
			t.Fatalf("correction activity restated the memory content: %q", entry.Summary)
		}
	}
	if !found {
		t.Fatal("correcting a memory recorded no activity")
	}
}

func TestCorrectingTwiceIsRefusedRatherThanForkingHistory(t *testing.T) {
	s, _ := fixture(t)
	original, err := s.Remember(testContext, "tone", "Keep updates brief.")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Correct(testContext, original.ID, "First correction.", ""); err != nil {
		t.Fatal(err)
	}
	_, err = s.Correct(testContext, original.ID, "Second correction.", "")
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected a conflict for an already-corrected memory, got %v", err)
	}
}

// Nothing infers a category from the text: an older memory stays uncategorized.
func TestMemoryKindIsNeverInferred(t *testing.T) {
	s, _ := fixture(t)
	m, err := s.Remember(testContext, "legacy", "Something recorded before categories existed.")
	if err != nil {
		t.Fatal(err)
	}
	if m.Kind != "" {
		t.Fatalf("kind = %q, want it left uncategorized", m.Kind)
	}
	if _, err := s.RememberKind(testContext, "bad", "content", "guess", "owner"); err == nil {
		t.Fatal("an unknown memory kind was accepted")
	}
}

// The assistant's remember tool deduplicates on key; correcting must not
// silently change an owner preference behind that upsert.
func TestRememberKeepsClassificationWhenUpdatingByKey(t *testing.T) {
	s, _ := fixture(t)
	if _, err := s.RememberKind(testContext, "tone", "Keep updates brief.", "preference", "owner"); err != nil {
		t.Fatal(err)
	}
	updated, err := s.Remember(testContext, "tone", "Keep updates brief and direct.")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Kind != "preference" || updated.Source != "owner" {
		t.Fatalf("an update by key dropped the classification: %+v", updated)
	}
}
