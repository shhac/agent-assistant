package core

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// ResolveDecision records the owner's selected option or custom answer. Existing
// worker decision propagation remains responsible for delivering actual answers.
func (s *Service) ResolveDecision(ctx context.Context, id, answer string) (Decision, error) {
	answer = strings.TrimSpace(answer)
	if answer == "" || len(answer) > 16*1024 {
		return Decision{}, errors.New("answer is required and must be at most 16 KiB")
	}
	return s.finishDecision(ctx, id, answer, "")
}

// DismissDecision closes an obsolete question with an audit reason. It is not an
// answer, approval or instruction: it never changes or resumes an assignment.
// A worker waiting on this question stays waiting until explicitly instructed.
func (s *Service) DismissDecision(ctx context.Context, id, reason string) (Decision, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" || len(reason) > 4096 {
		return Decision{}, errors.New("dismissal reason is required and must be at most 4 KiB")
	}
	return s.finishDecision(ctx, id, "", reason)
}
func (s *Service) finishDecision(ctx context.Context, id, answer, reason string) (Decision, error) {
	var out Decision
	err := s.store.update(ctx, func(v *Snapshot) error {
		for i := range v.Decisions {
			d := &v.Decisions[i]
			if d.ID != id {
				continue
			}
			if d.Status != "open" {
				return fmt.Errorf("decision already closed: %w", ErrConflict)
			}
			now := s.now().UTC()
			d.ResolvedAt = &now
			if reason != "" {
				d.Status = "dismissed"
				d.Disposition = "dismissed"
				d.ResolutionReason = reason
				d.Answer = ""
				record(v, now, d.ProjectID, "decision.dismissed", d.Title+": "+reason)
			} else {
				d.Status = "resolved"
				d.Disposition = "custom"
				d.Answer = answer
				for _, choice := range d.Choices {
					if choice == answer {
						d.Disposition = "choice"
						break
					}
				}
				record(v, now, d.ProjectID, "decision.resolved", d.Title+": "+answer)
			}
			out = *d
			return nil
		}
		return ErrNotFound
	})
	return out, err
}
