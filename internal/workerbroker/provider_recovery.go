package workerbroker

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/shhac/agent-assistant/internal/engine"
	"github.com/shhac/lib-agent-harness/completion"
)

const maxProviderFailures = 6

// Only a definitive provider rejection may enter this automatic recovery path.
// The failed completion never executed tools. Earlier acknowledged operations
// remain in Transcript and must not be replayed. The daemon admits every retry.
func (b *Broker) modelFailure(id string, err error) {
	var failure *completion.RequestError
	if !errors.As(err, &failure) || !failure.Retryable() {
		kind := completion.ErrorUnknown
		if failure != nil {
			switch failure.Kind {
			case completion.ErrorAuthentication, completion.ErrorContextLimit:
				kind = failure.Kind
			}
		}
		if errors.Is(err, engine.ErrContextPressure) {
			kind = completion.ErrorContextLimit
		}
		_ = b.update(id, func(r *storedRun) error {
			if r.Run.Status == "cancelled" || r.PendingStatus == "paused" || r.PendingStatus == "cancelled" {
				return nil
			}
			r.Run.ProviderFailureKind = string(kind)
			r.Run.RetryAt = time.Time{}
			r.PendingStatus = "blocked"
			r.PendingSummary = (&completion.RequestError{Kind: kind}).Error() + ". Inspect the preserved work and correct the problem before explicitly resuming; no automatic retry is scheduled."
			r.Run.Summary = "Finalizing isolated execution and collecting evidence"
			r.Run.UpdatedAt = now()
			return nil
		})
		return
	}
	_ = b.update(id, func(r *storedRun) error {
		if r.PendingStatus == "paused" || r.PendingStatus == "cancelled" {
			return nil
		}
		r.Run.ProviderFailures++
		r.Run.ProviderFailureKind = string(failure.Kind)
		if r.Run.ProviderFailures >= maxProviderFailures || r.ModelCalls >= b.cfg.MaxTurns {
			r.Run.RetryAt = time.Time{}
			r.PendingStatus = "blocked"
			r.PendingSummary = "Provider recovery allowance exhausted; progress is preserved. Resume explicitly after the provider recovers."
			return nil
		}
		delay := providerDelay(r.Run.ProviderFailures, failure.RetryAfter)
		r.Run.RetryAt = now().Add(delay)
		r.PendingStatus = "retry_wait"
		r.PendingSummary = fmt.Sprintf("Model provider temporarily unavailable (%s). Retry %d scheduled after %s; saved progress will continue without repeating tools.", failure.Kind, r.Run.ProviderFailures, r.Run.RetryAt.Format(time.RFC3339))
		return nil
	})
}
func providerDelay(failures int, retryAfter time.Duration) time.Duration {
	step := failures - 1
	if step < 0 {
		step = 0
	}
	if step > 6 {
		step = 6
	}
	base := 5 * time.Second * time.Duration(1<<step)
	if base > 5*time.Minute {
		base = 5 * time.Minute
	}
	delay := base + time.Duration(rand.Int64N(int64(base/4)+1))
	if delay > 5*time.Minute {
		delay = 5 * time.Minute
	}
	if retryAfter > delay {
		delay = retryAfter
	}
	return delay
}
func (b *Broker) clearProviderFailure(id string) {
	_ = b.update(id, func(r *storedRun) error {
		r.Run.ProviderFailures = 0
		r.Run.ProviderFailureKind = ""
		r.Run.RetryAt = time.Time{}
		return nil
	})
}
