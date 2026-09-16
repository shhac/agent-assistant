package app

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/shhac/agent-assistant/internal/config"
	"github.com/shhac/lib-agent-harness/session"
)

const quotaCacheAge = time.Minute

var errWorkerUsageHeld = errors.New("worker work held by subscription usage policy")

// Cache native telemetry by login, not project or model: several workers may
// share one subscription. Never persist account metadata or credential material.
type workerUsageMeter struct {
	mu      sync.Mutex
	inspect func(context.Context, session.Options) (session.Inspection, error)
	entries map[usageIdentity]usageEntry
}
type usageIdentity struct{ engine, binary, home string }
type usageEntry struct {
	quota   session.QuotaSnapshot
	fetched time.Time
}

func (m *workerUsageMeter) read(ctx context.Context, model config.Model) session.QuotaSnapshot {
	key := usageIdentity{engine: model.Engine, binary: model.CodexBin, home: model.CodexHome}
	if model.Engine == "claude" {
		key.binary, key.home = model.ClaudeBin, model.ClaudeHome
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	if e, ok := m.entries[key]; ok && now.Sub(e.fetched) < quotaCacheAge {
		return e.quota
	}
	inspect := m.inspect
	if inspect == nil {
		inspect = session.Inspect
	}
	bounded, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	// Account inspection may fail while quota succeeds. Keep usable quota even
	// when Inspect returns a partial error; never interpret an error as zero use.
	result, _ := inspect(bounded, session.Options{Engine: session.Engine(key.engine), Binary: key.binary, Home: key.home})
	if ctx.Err() == nil {
		if m.entries == nil {
			m.entries = make(map[usageIdentity]usageEntry)
		}
		m.entries[key] = usageEntry{result.Quota, time.Now()}
	}
	return result.Quota
}

// workerUsageAllowed runs BEFORE durable dispatch/resume/instruction intent.
// A hold leaves the existing queued/interrupted state eligible for the next tick.
func (a *App) workerUsageAllowed(ctx context.Context, profileID string) error {
	cfg := a.Config()
	profile, err := a.Core.GetProfile(profileID)
	if err != nil {
		return err
	}
	policy := cfg.Limits.WorkerUsage
	id, name := "worker-usage:"+profile.ID, "Usage for "+profile.Name
	unavailable := func(reason string) error {
		detail := reason + "; new worker work is allowed"
		if policy.OnUnavailable == "pause" {
			detail = reason + "; new worker work is paused"
		}
		a.Status(id, name, "unavailable", detail)
		if policy.OnUnavailable == "pause" {
			return fmt.Errorf("%w: %s", errWorkerUsageHeld, detail)
		}
		return nil
	}
	if !profile.Managed {
		return unavailable("The external broker's CLI account cannot be inspected locally")
	}
	model := workerModel(cfg, profile)
	threshold := 0
	switch model.Engine {
	case "codex":
		threshold = policy.CodexMaxUsedPercent
	case "claude":
		threshold = policy.ClaudeMaxUsedPercent
	default:
		return unavailable("Subscription usage is unavailable for this engine")
	}
	if threshold == 0 {
		a.Status(id, name, "disabled", "Worker usage limit is disabled for "+model.Engine)
		return nil
	}
	quota := a.workerUsage.read(ctx, model)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	held, known, detail := quotaDecision(quota, model, threshold, time.Now())
	if held {
		a.Status(id, name, "paused", detail)
		return fmt.Errorf("%w: %s", errWorkerUsageHeld, detail)
	}
	if !known {
		return unavailable("Fresh subscription usage is unavailable for " + model.Engine)
	}
	a.Status(id, name, "connected", fmt.Sprintf("%s; new worker work pauses at %d%% consumed", detail, threshold))
	return nil
}

func quotaDecision(q session.QuotaSnapshot, model config.Model, threshold int, now time.Time) (held, known bool, detail string) {
	if q.IsStale(now, 2*quotaCacheAge) {
		return false, false, ""
	}
	missing, peak, seen := !q.Complete, 0.0, false
	for _, w := range q.Windows {
		if !quotaApplies(w, model) {
			continue
		}
		seen = true
		if w.IsStale(now, 2*quotaCacheAge) || w.UsedPercent == nil || math.IsNaN(*w.UsedPercent) || math.IsInf(*w.UsedPercent, 0) || *w.UsedPercent < 0 || (w.ResetsAt != nil && !w.ResetsAt.After(now)) {
			missing = true
			continue
		}
		used := *w.UsedPercent
		peak = max(peak, used)
		if used >= float64(threshold) {
			detail = fmt.Sprintf("New worker work paused: %s %s is %.1f%% consumed (limit %d%%)", model.Engine, w.ID, used, threshold)
			if w.ResetsAt != nil {
				detail += "; resets " + w.ResetsAt.UTC().Format(time.RFC3339)
			}
			return true, true, detail
		}
	}
	return false, seen && !missing, fmt.Sprintf("%s peak applicable usage %.1f%%", model.Engine, peak)
}

// Ignore unrelated pools (e.g. Codex code reviews or Claude Sonnet when an Opus
// worker is selected). Unknown-only snapshots stay unavailable, never zero.
func quotaApplies(w session.QuotaWindow, model config.Model) bool {
	scope, name := strings.ToLower(w.Scope), strings.ToLower(model.Model)
	if model.Engine == "codex" {
		return scope == "default" || scope == "codex" || scope == name
	}
	switch w.ID {
	case "five_hour", "seven_day":
		return true
	}
	for _, family := range []string{"opus", "sonnet", "haiku"} {
		if strings.Contains(name, family) && (w.ID == "seven_day_"+family || (strings.HasPrefix(w.ID, "model:") && strings.Contains(scope, family))) {
			return true
		}
	}
	return strings.HasPrefix(w.ID, "model:") && strings.EqualFold(w.Scope, model.Model)
}
