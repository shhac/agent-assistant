package engine

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestOversizedSummaryCorrectionPreservesSourceAndUsage(t *testing.T) {
	original := contextFixture()
	calls := 0
	var source string
	cp, usage, err := CompactContext(context.Background(), original, ContextOptions{MaxBytes: 40000, MaxSummaryBytes: 1024}, func(_ context.Context, input []Message) (Message, Usage, error) {
		calls++
		if calls == 1 {
			source = input[1].Content
			if !strings.Contains(input[0].Content, "Aim for 512 bytes") {
				t.Fatal("missing concise target")
			}
			return Message{Role: "assistant", Content: strings.Repeat("x", 1025)}, Usage{InputTokens: 10, OutputTokens: 5, TotalTokens: 15, Known: true}, nil
		}
		if calls != 2 || input[1].Content != source || !strings.Contains(input[0].Content, "Aim for 256 bytes") {
			t.Fatal("correction lost source or did not tighten target")
		}
		return Message{Role: "assistant", Content: "Earlier checks failed; consult archived evidence. No verified acceptance."}, Usage{InputTokens: 8, OutputTokens: 2, TotalTokens: 10, Known: true}, nil
	})
	if err != nil || !cp.Compacted || calls != 2 || !usage.Known || usage.TotalTokens != 25 {
		t.Fatalf("calls=%d usage=%+v err=%v", calls, usage, err)
	}
	if !reflect.DeepEqual(original, contextFixture()) {
		t.Fatal("original mutated")
	}
}

func TestSummaryCorrectionHonoursCancellationAndUnknownUsage(t *testing.T) {
	for _, cancelFirst := range []bool{true, false} {
		ctx, cancel := context.WithCancel(context.Background())
		calls := 0
		_, used, err := summarizeContext(ctx, contextFixture(), 1024, func(context.Context, []Message) (Message, Usage, error) {
			calls++
			if calls == 1 {
				if cancelFirst {
					cancel()
				}
				return Message{Role: "assistant", Content: strings.Repeat("x", 1025)}, Usage{}, nil
			}
			return Message{Role: "assistant", Content: "Valid summary"}, Usage{TotalTokens: 10, Known: true}, nil
		})
		cancel()
		if used.Known {
			t.Fatal("unknown consumption became known")
		}
		if cancelFirst {
			if !errors.Is(err, context.Canceled) || calls != 1 {
				t.Fatal(calls, err)
			}
		} else if err != nil || calls != 2 || used.TotalTokens != 10 {
			t.Fatal(calls, used, err)
		}
	}
}
