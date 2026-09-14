package core

import (
	"context"
	"errors"
	"sort"
)

// ClaimEvent records an inbound delivery before acknowledging it. Pending claims
// are surfaced on restart; they are never silently replayed after uncertain effects.
func (s *Service) ClaimEvent(ctx context.Context, id string) (bool, error) {
	if !required(id) {
		return false, errors.New("event ID is required")
	}
	claimed := false
	err := s.store.update(ctx, func(v *Snapshot) error {
		if _, ok := v.Events[id]; ok {
			return nil
		}
		v.Events[id] = false
		claimed = true
		return nil
	})
	return claimed, err
}
func (s *Service) CompleteEvent(ctx context.Context, id string) error {
	return s.store.update(ctx, func(v *Snapshot) error {
		if _, ok := v.Events[id]; !ok {
			return ErrNotFound
		}
		v.Events[id] = true
		return nil
	})
}
func (s *Service) PendingEvents(ctx context.Context) ([]string, error) {
	v, err := s.store.Snapshot(ctx)
	if err != nil {
		return nil, err
	}
	ids := []string{}
	for id, done := range v.Events {
		if !done {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids, nil
}
