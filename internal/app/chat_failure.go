package app

import (
	"context"
	"errors"
	"strings"

	"github.com/shhac/agent-assistant/internal/engine"
)

// Preserve useful next steps without copying raw provider, tool or storage errors
// into durable conversation metadata.
func chatFailureReason(err error) string {
	detail := strings.ToLower(err.Error())
	reason := "The assistant could not finish this message."
	switch {
	case errors.Is(err, engine.ErrNotConfigured):
		reason = "Choose an assistant model in Settings before sending another message."
	case strings.Contains(detail, "daily model call allowance"):
		reason = "The daily model-call allowance is exhausted. Wait for it to reset or adjust the limit in Settings."
	case errors.Is(err, context.DeadlineExceeded) || strings.Contains(detail, "timed out"):
		reason = "The reply timed out. Check the recorded actions before asking the assistant to continue."
	case strings.Contains(detail, "context limit"):
		reason = "The conversation exceeds the model's context limit. The assistant could not process this turn."
	case errors.Is(err, engine.ErrTurnLimit):
		reason = "The assistant reached its per-turn action limit. Review its progress before asking it to continue."
	case strings.Contains(detail, "codex") || strings.Contains(detail, "claude"):
		reason = "Check the selected CLI installation, login, and model in Settings; the assistant could not use that profile."
	}
	return reason + " Recorded actions were preserved; no automatic replay was attempted."
}
