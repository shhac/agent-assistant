package engine

import (
	"context"
	"errors"

	"github.com/shhac/lib-agent-harness/completion"
)

// Length correction is a new, admitted tool-free inference over the same source,
// not a replay of project work or an automatic retry of a provider failure.
// Permit one correction and keep the original hard ceiling. Never truncate a
// summary: cutting a sentence can remove the qualifier that makes it true.
func summarizeContext(ctx context.Context, source []Message, maxBytes int, summarize ContextSummarizer) (Message, Usage, error) {
	var total Usage
	target := maxBytes
	for attempt := 0; attempt < 2; attempt++ {
		if err := ctx.Err(); err != nil {
			return Message{}, total, err
		}
		reply, used, err := summarize(ctx, summaryMessages(source, target))
		mergeContextUsage(&total, used, attempt == 0)
		if err != nil {
			return Message{}, total, err
		}
		err = validateContextSummary(reply, maxBytes)
		if err == nil {
			return reply, total, nil
		}
		var diagnostic *completion.RequestError
		if attempt != 0 || !errors.As(err, &diagnostic) || diagnostic.Code != "context_summary_too_large" {
			return Message{}, total, err
		}
		// Ask for half the original ceiling (and a quarter-size target), retaining
		// the same source so a rejected summary cannot become new factual authority.
		target = maxBytes / 2
	}
	panic("unreachable summary correction")
}
