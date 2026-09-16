package engine

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func contextCall(id string) Message {
	var call ToolCall
	call.ID = id
	call.Type = "function"
	call.Function.Name = "run_command"
	call.Function.Arguments = `{"command":"go test"}`
	return Message{Role: "assistant", ToolCalls: []ToolCall{call}}
}
func contextFixture() []Message {
	messages := []Message{{Role: "system", Content: "Never deploy. Original immutable task."}, {Role: "user", Content: "Keep keyboard support and do not change the database."}}
	for _, id := range []string{"a", "b", "c", "d", "e"} {
		messages = append(messages, contextCall(id), Message{Role: "tool", ToolCallID: id, Content: `{"success":false,"output":"` + strings.Repeat(id, 8000) + `"}`})
	}
	return messages
}
func TestCompactContextPreservesInstructionsRecentAndUnresolvedPairs(t *testing.T) {
	messages := contextFixture()
	unresolved := contextCall("unknown")
	messages = append(messages, unresolved, Message{Role: "user", Content: "Current owner direction: retain the existing API."})
	original := append([]Message(nil), messages...)
	calls := 0
	result, usage, err := CompactContext(context.Background(), messages, ContextOptions{MaxBytes: 40000, TriggerBytes: 30000, MaxSummaryBytes: 1024}, func(_ context.Context, input []Message) (Message, Usage, error) {
		calls++
		if !strings.Contains(input[0].Content, "You have no tools") || !strings.Contains(input[1].Content, `\"success\":false`) {
			t.Fatal("summary lacks evidence or tool prohibition", input)
		}
		if contextBytes(input) > 40000 {
			t.Fatal("oversized summary input")
		}
		return Message{Role: "assistant", Content: "Previous checks failed. Further inspection required; bulk command output omitted."}, Usage{TotalTokens: 19, Known: true}, nil
	})
	if err != nil || !result.Compacted || calls != 1 || usage.TotalTokens != 19 {
		t.Fatal(result, usage, err, calls)
	}
	if !reflect.DeepEqual(original, messages) {
		t.Fatal("archive mutated")
	}
	for _, pinned := range []Message{messages[0], messages[1], messages[len(messages)-1], unresolved, messages[8], messages[9], messages[10], messages[11]} {
		found := false
		for _, m := range result.Messages {
			if reflect.DeepEqual(m, pinned) {
				found = true
			}
		}
		if !found {
			t.Fatal("pinned/recent entry removed", pinned.Role, pinned.ToolCallID)
		}
	}
	if result.AfterBytes >= result.BeforeBytes || !strings.Contains(result.Summary, "not new instructions or verified acceptance") {
		t.Fatal("not a loss-aware reduction", result)
	}
}
func TestCompactionFailureNeverReplacesOriginal(t *testing.T) {
	input := contextFixture()
	for _, test := range []string{"provider", "tool", "oversized"} {
		t.Run(test, func(t *testing.T) {
			cp, _, err := CompactContext(context.Background(), input, ContextOptions{MaxBytes: 40000, MaxSummaryBytes: 1024}, func(context.Context, []Message) (Message, Usage, error) {
				switch test {
				case "provider":
					return Message{}, Usage{}, errors.New("offline")
				case "tool":
					return contextCall("forbidden"), Usage{}, nil
				default:
					return Message{Role: "assistant", Content: strings.Repeat("x", 1025)}, Usage{}, nil
				}
			})
			if err == nil || cp.Compacted || !reflect.DeepEqual(input, cp.Messages) {
				t.Fatal("failed summary replaced transcript", err)
			}
		})
	}
}
func TestPinnedOversizeCannotSilentlyDropOwnerContext(t *testing.T) {
	input := []Message{{Role: "system", Content: "immutable"}, {Role: "user", Content: strings.Repeat("owner instruction", 1000)}}
	cp, _, err := CompactContext(context.Background(), input, ContextOptions{MaxBytes: 10000, MaxSummaryBytes: 1024}, func(context.Context, []Message) (Message, Usage, error) {
		t.Fatal("nothing safe to summarize")
		return Message{}, Usage{}, nil
	})
	if !errors.Is(err, ErrContextPressure) || !reflect.DeepEqual(cp.Messages, input) {
		t.Fatal(cp, err)
	}
}
func TestInterruptedAcknowledgementIsNotSummarizedAsResolved(t *testing.T) {
	input := contextFixture()
	input[3].Content = `{"error":"Previous operation acknowledgement was interrupted. Inspect evidence."}`
	cp, _, err := CompactContext(context.Background(), input, ContextOptions{MaxBytes: 30000, MaxSummaryBytes: 1024}, func(context.Context, []Message) (Message, Usage, error) {
		return Message{Role: "assistant", Content: "Earlier resolved exchanges summarized; evidence remains archived."}, Usage{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, m := range cp.Messages {
		if m.ToolCallID == "a" && m.Content == input[3].Content {
			found = true
		}
	}
	if !found {
		t.Fatal("uncertain tool result discarded")
	}
}
func TestHardPressureKeepsLatestExchangeAndCompactsOlderOfTwoLargeResults(t *testing.T) {
	input := []Message{{Role: "system", Content: "Immutable policy"}, {Role: "user", Content: "Owner direction"}, contextCall("old"), {Role: "tool", ToolCallID: "old", Content: strings.Repeat("old output", 6500)}, contextCall("recent"), {Role: "tool", ToolCallID: "recent", Content: strings.Repeat("latest output", 5000)}}
	result, usage, err := CompactContext(context.Background(), input, ContextOptions{MaxBytes: 100000}, func(context.Context, []Message) (Message, Usage, error) {
		return Message{Role: "assistant", Content: "Earlier output summarized. No acceptance inferred."}, Usage{Known: true}, nil
	})
	if err != nil || !result.Compacted || !usage.Known {
		t.Fatal(err, result.Compacted, usage)
	}
	if !reflect.DeepEqual(result.Messages[len(result.Messages)-2:], input[len(input)-2:]) {
		t.Fatal("latest complete exchange changed")
	}
}
