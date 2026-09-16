package core

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestChatCheckpointPreservesSourcesAndRequiresCompleteBoundary(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	first, _ := s.AddMessage(ctx, "user", "first owner request")
	second, _ := s.AddMessage(ctx, "assistant", "first reply")
	third, _ := s.AddMessage(ctx, "user", "second owner request")
	fourth, _ := s.AddMessage(ctx, "assistant", "second reply")
	before, _ := s.Snapshot(ctx)
	if err := s.SaveChatCheckpoint(ctx, "", first.ID, "summary"); err == nil {
		t.Fatal("accepted unfinished exchange")
	}
	if err := s.SaveChatCheckpoint(ctx, "", second.ID, "summary"); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveChatCheckpoint(ctx, "", fourth.ID, "stale summary"); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	if err := s.SaveChatCheckpoint(ctx, second.ID, third.ID, "half exchange"); err == nil {
		t.Fatal("advanced into half exchange")
	}
	if err := s.SaveChatCheckpoint(ctx, second.ID, fourth.ID, "updated summary"); err != nil {
		t.Fatal(err)
	}
	after, _ := s.Snapshot(ctx)
	if !reflect.DeepEqual(before.Messages, after.Messages) || after.ChatCheckpoint.ThroughID != fourth.ID {
		t.Fatal("sources lost")
	}
}
