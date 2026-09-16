package core

import (
	"context"
	"errors"
	"strings"
)

// PrepareOwnerControl persists the owner's hold before contacting a broker.
// Retrying a pending key never assumes that the external operation failed.
func (s *Service) PrepareOwnerControl(ctx context.Context, id, action, key string) (Agent, bool, error) {
	var out Agent
	send := false
	if !contains([]string{"pause", "resume", "stop"}, action) || strings.TrimSpace(key) == "" || len(key) > 256 {
		return out, false, errors.New("valid control action and bounded operation ID required")
	}
	err := s.store.update(ctx, func(v *Snapshot) error {
		a := agent(v, id)
		if a == nil {
			return ErrNotFound
		}
		if _, exists := v.Events[key]; exists {
			out = *a
			return nil
		}
		if terminal(a.Status) {
			return errors.New("assignment has ended")
		}
		if action == "resume" {
			if a.OwnerControl == "stop" {
				return errors.New("confirm the pending stop before another control")
			}
			if v.Paused {
				return errors.New("coordination is paused")
			}
			if a.Status != "paused" && a.Status != "interrupted" && !(a.Status == "blocked" && a.ProviderFailureKind != "") {
				return errors.New("resume requires a confirmed paused or interrupted worker")
			}
			if !a.RetryAt.IsZero() && s.now().Before(a.RetryAt) {
				return errors.New("provider retry is not due")
			}
			saved := a.OwnerControl
			a.OwnerControl = ""
			status := a.Status
			a.Status = "interrupted"
			if err := s.dispatchAuthority(v, a); err != nil {
				a.OwnerControl = saved
				a.Status = status
				return err
			}
			if executingCount(v) >= s.configuration().Limits.MaxAgents {
				return errors.New("agent execution capacity reached")
			}
			a.OwnerControl = "resume"
			a.Status = "resuming"
			a.ResumeKey = key
			if a.ExternalID == "" {
				a.OwnerControl = ""
				a.Status = "queued"
			}
		} else {
			if a.Status == "resuming" || a.Status == "dispatching" || a.Status == "reconciling" {
				return errors.New("reconcile the uncertain execution before another control")
			}
			a.OwnerControl = action
			if action == "pause" {
				a.Status = "pause_requested"
				if a.ExternalID == "" {
					a.Status = "paused"
				}
			} else {
				a.Status = "stop_requested"
				if a.ExternalID == "" {
					a.Status = "cancelled"
				}
			}
		}
		a.ControlKey = key
		a.LastUpdate = s.now().UTC()
		a.Summary = "Owner requested " + action
		send = a.ExternalID != ""
		v.Events[key] = !send
		out = *a
		record(v, a.LastUpdate, a.ProjectID, "agent.control", a.Name+": "+action)
		return nil
	})
	return out, send, err
}
func (s *Service) SetAgentControlCapabilities(ctx context.Context, id string, capabilities []string) error {
	return s.store.update(ctx, func(v *Snapshot) error {
		a := agent(v, id)
		if a == nil {
			return ErrNotFound
		}
		a.ControlCapabilities = nil
		for _, c := range capabilities {
			if contains([]string{"pause", "resume", "stop"}, c) && !contains(a.ControlCapabilities, c) {
				a.ControlCapabilities = append(a.ControlCapabilities, c)
			}
		}
		return nil
	})
}
func (s *Service) ConfirmOwnerResume(ctx context.Context, id string) error {
	return s.store.update(ctx, func(v *Snapshot) error {
		a := agent(v, id)
		if a == nil {
			return ErrNotFound
		}
		if a.OwnerControl == "resume" {
			a.OwnerControl = ""
		}
		return nil
	})
}
