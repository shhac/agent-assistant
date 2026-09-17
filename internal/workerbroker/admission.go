package workerbroker

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/shhac/agent-assistant/internal/engine"
	"github.com/shhac/agent-assistant/internal/integrations/worker"
)

// Stages name which kind of inference is being admitted. A context summary is
// as billable as an ordinary turn and is admitted and accounted identically.
const (
	stageTurn    = "model_turn"
	stageSummary = "context_summary"
)

// reserveWorkerModelCall is the one place a worker's model work is authorized.
// It runs as the completion transport's BeforeRequest hook, which fires after
// non-billable local probes and immediately before the request leaves, so a
// refusal here spends nothing and a failure earlier adds no unknown usage.
//
// Order matters: the local budget is checked before the account is inspected,
// so a run that already has to stop does not cost an inspection; and both
// checks complete before any pending accounting is written, so a refused call
// never leaves a reservation behind.
func (b *Broker) reserveWorkerModelCall(ctx context.Context, id, stage string, request *string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	current, err := b.snapshot(id)
	if err != nil {
		return err
	}
	budget := b.tokenBudget()
	if hold := budgetHold(current, budget); hold != nil {
		return b.refuse(id, *hold)
	}
	if b.cfg.Admit != nil {
		if admitErr := b.cfg.Admit(ctx); admitErr != nil {
			var held *worker.HoldError
			if errors.As(admitErr, &held) {
				return b.refuse(id, held.Hold)
			}
			return admitErr
		}
	}
	reservation := uid()
	if err = b.update(id, func(run *storedRun) error {
		if run.Run.Status != "running" || run.PendingStatus == "paused" || run.PendingStatus == "cancelled" {
			return errInterrupted
		}
		// Re-check under the lock: an owner budget change or a settled call may
		// have landed between the snapshot and here.
		if hold := budgetHold(*run, budget); hold != nil {
			return &worker.HoldError{Hold: *hold}
		}
		run.ModelCalls++
		run.UsageLedger = true
		run.Run.ResourceHold = nil
		run.PendingUsage = &pendingUsage{RequestID: reservation, Stage: stage, StartedAt: now()}
		publishUsage(run, budget)
		run.Run.UpdatedAt = now()
		return nil
	}); err != nil {
		var held *worker.HoldError
		if errors.As(err, &held) {
			return b.refuse(id, held.Hold)
		}
		return err
	}
	*request = reservation
	return nil
}

// refuse records the hold before returning, because completion transports wrap
// admission errors in an opaque type the caller cannot unwrap. The execution
// loop only needs to recognize that this was a hold, not carry its details.
func (b *Broker) refuse(id string, hold worker.ResourceHold) error {
	b.holdOnResources(id, hold)
	return &worker.HoldError{Hold: hold}
}

// holdOnResources preserves the run exactly as it stands. No request was made
// and no response was received, so the transcript, any queued owner message and
// any unresolved operation stay untouched for the next admitted attempt. Owner
// pause and stop still win.
func (b *Broker) holdOnResources(id string, hold worker.ResourceHold) {
	_ = b.update(id, func(r *storedRun) error {
		if r.Run.Status == "cancelled" || r.PendingStatus == "cancelled" || r.PendingStatus == "paused" {
			return nil
		}
		held := hold
		r.Run.ResourceHold = &held
		// A hold is not a provider rejection: it schedules no retry, spends no
		// recovery allowance and leaves the failure classification alone.
		r.Run.RetryAt = time.Time{}
		r.PendingStatus = "usage_wait"
		r.PendingSummary = hold.Reason
		r.Run.Summary = "Finalizing isolated execution and collecting evidence"
		r.Run.UpdatedAt = now()
		return nil
	})
}

func (b *Broker) tokenBudget() int64 {
	if b.cfg.TokenBudget == nil {
		return 0
	}
	budget := b.cfg.TokenBudget()
	if budget < 0 {
		return 0
	}
	return budget
}

// budgetHold answers whether the next request may be made under the configured
// token budget. Zero disables it entirely, which is the only way past a run
// whose consumption cannot be established: raising a budget cannot retroactively
// measure what a provider never reported.
func budgetHold(run storedRun, budget int64) *worker.ResourceHold {
	if budget <= 0 {
		return nil
	}
	if run.UsageUnknownCalls > 0 {
		return &worker.ResourceHold{Kind: worker.HoldUsageUnknown, OwnerAction: true, Reason: fmt.Sprintf("Token budget enforcement stopped: %d earlier model call(s) reported no usage, so this worker's consumption cannot be established. Raising the budget cannot measure them. Decide explicitly whether to continue with the budget disabled (limits.worker_token_budget = 0) and resume.", run.UsageUnknownCalls)}
	}
	if used := run.UsageInputTokens + run.UsageOutputTokens; used >= budget {
		return &worker.ResourceHold{Kind: worker.HoldTokenBudget, OwnerAction: true, Reason: fmt.Sprintf("Worker token budget reached: %d of %d tokens used. Raise limits.worker_token_budget, or set it to 0, then resume this assignment explicitly to continue with its saved context.", used, budget)}
	}
	return nil
}

// settleUsage closes one reservation. Matching by request ID means a repeated
// or late settlement changes nothing, and a reservation belonging to an earlier
// process is left for restart reconciliation to convert into uncertainty.
func (b *Broker) settleUsage(id, request string, usage engine.Usage) {
	if request == "" {
		return
	}
	budget := b.tokenBudget()
	_ = b.update(id, func(run *storedRun) error {
		if run.PendingUsage == nil || run.PendingUsage.RequestID != request {
			return nil
		}
		run.PendingUsage = nil
		if usage.Known {
			run.UsageInputTokens += int64(max(usage.InputTokens, 0))
			run.UsageOutputTokens += int64(max(usage.OutputTokens, 0))
		} else {
			run.UsageUnknownCalls++
		}
		publishUsage(run, budget)
		return nil
	})
}

// publishUsage keeps the reported ledger identical to the persisted one. The
// budget travels with it so the owner sees the limit their worker is measured
// against, not just a number of tokens.
func publishUsage(run *storedRun, budget int64) {
	run.Run.Usage = worker.Usage{InputTokens: run.UsageInputTokens, OutputTokens: run.UsageOutputTokens, UnknownCalls: run.UsageUnknownCalls, TokenBudget: budget}
}

// reconcileUsage runs once per run at broker startup. A surviving reservation
// may have been billed, and calls recorded before this ledger existed were
// never measured at all; both become explicit unknown consumption rather than
// disappearing into an apparently free history.
func reconcileUsage(run *storedRun, budget int64) {
	if !run.UsageLedger {
		run.UsageLedger = true
		run.UsageUnknownCalls += run.ModelCalls
	}
	if run.PendingUsage != nil {
		run.PendingUsage = nil
		run.UsageUnknownCalls++
	}
	publishUsage(run, budget)
}
