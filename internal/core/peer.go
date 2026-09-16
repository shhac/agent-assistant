package core

import (
	"context"
	"errors"
)

// PeerMessageSource resolves sender identity from durable state and rechecks its
// current authority without waking it or reserving another execution slot.
func (s *Service) PeerMessageSource(ctx context.Context, id string) (Agent, error) {
	v, err := s.store.Snapshot(ctx)
	if err != nil {
		return Agent{}, err
	}
	if v.Paused {
		return Agent{}, errors.New("coordination is paused")
	}
	a := agent(&v, id)
	if a == nil {
		return Agent{}, ErrNotFound
	}
	if terminal(a.Status) || a.ExternalID == "" {
		return Agent{}, errors.New("peer messages require a live sender session")
	}
	if a.Status == "interrupted" || a.Status == "resuming" || (a.Status == "reconciling" && a.ResumeKey != "") {
		return Agent{}, errors.New("sender requires reconciliation and resume before messaging")
	}
	if err := s.dispatchAuthority(&v, a); err != nil {
		return Agent{}, err
	}
	return *a, nil
}
