package core

import (
	"context"
	"errors"
	"fmt"
)

// memoryKinds are the categories an owner can choose between. An empty kind is
// preserved as uncategorized; nothing infers one from the text.
var memoryKinds = []string{"preference", "observation"}

func validMemoryKind(kind string) bool { return kind == "" || contains(memoryKinds, kind) }

// liveMemory finds the memory currently answering for a key. A corrected
// memory keeps its key so the chain stays readable, so the superseded records
// are skipped: an update must land on the memory the owner is being shown, not
// on the tombstone behind it.
func liveMemory(v *Snapshot, key string) *Memory {
	for i := range v.Memories {
		if v.Memories[i].Key == key && v.Memories[i].SupersededAt.IsZero() {
			return &v.Memories[i]
		}
	}
	return nil
}

func memoryByID(v *Snapshot, id string) *Memory {
	for i := range v.Memories {
		if v.Memories[i].ID == id {
			return &v.Memories[i]
		}
	}
	return nil
}

// Remember records an assistant-sourced memory. The assistant's remember tool
// deduplicates on key, so this keeps the upsert behaviour.
func (s *Service) Remember(ctx context.Context, key, value string) (Memory, error) {
	return s.RememberKind(ctx, key, value, "", "assistant")
}

// RememberKind records a memory, optionally categorized, upserting on key.
func (s *Service) RememberKind(ctx context.Context, key, value, kind, source string) (Memory, error) {
	if !required(key, value) {
		return Memory{}, errors.New("memory key and content are required")
	}
	if !validMemoryKind(kind) {
		return Memory{}, errors.New("memory kind must be preference or observation")
	}
	out := Memory{ID: uid(), Key: key, Content: value, Kind: kind, Source: source, UpdatedAt: s.now().UTC()}
	err := s.store.update(ctx, func(v *Snapshot) error {
		existing := liveMemory(v, key)
		if existing == nil {
			v.Memories = append(v.Memories, out)
			record(v, out.UpdatedAt, "", "memory.created", key)
			return nil
		}
		out.ID = existing.ID
		if out.Kind == "" {
			out.Kind = existing.Kind
		}
		if out.Source == "" {
			out.Source = existing.Source
		}
		// Keep what this memory replaced; never inherit a tombstone.
		out.Supersedes = existing.Supersedes
		*existing = out
		record(v, out.UpdatedAt, "", "memory.updated", key)
		return nil
	})
	return out, err
}

// Correct supersedes a memory rather than overwriting it: the replacement
// records what it replaced, and the original is kept so the owner can see what
// was believed before. The activity entry names the memory, not its content.
func (s *Service) Correct(ctx context.Context, id, content, kind string) (Memory, error) {
	if !required(content) {
		return Memory{}, errors.New("corrected memory content is required")
	}
	if !validMemoryKind(kind) {
		return Memory{}, errors.New("memory kind must be preference or observation")
	}
	now := s.now().UTC()
	var out Memory
	err := s.store.update(ctx, func(v *Snapshot) error {
		m := memoryByID(v, id)
		if m == nil {
			return ErrNotFound
		}
		if !m.SupersededAt.IsZero() {
			return fmt.Errorf("memory was already corrected: %w", ErrConflict)
		}
		out = Memory{ID: uid(), Key: m.Key, Content: content, Kind: kind, Source: m.Source, Supersedes: m.ID, UpdatedAt: now}
		if kind == "" {
			out.Kind = m.Kind
		}
		m.SupersededAt = now
		v.Memories = append(v.Memories, out)
		record(v, now, "", "memory.corrected", m.Key)
		return nil
	})
	return out, err
}

// Forget removes a memory and repairs the correction chain around it, so the
// list can never show a corrected memory whose replacement is gone, or a
// replacement claiming a predecessor that no longer exists.
func (s *Service) Forget(ctx context.Context, id string) error {
	return s.store.update(ctx, func(v *Snapshot) error {
		target := memoryByID(v, id)
		if target == nil {
			return ErrNotFound
		}
		key, supersedes := target.Key, target.Supersedes
		for i := range v.Memories {
			// A replacement is going away: its predecessor is current again.
			if v.Memories[i].ID == supersedes {
				v.Memories[i].SupersededAt = Memory{}.SupersededAt
			}
			// A predecessor is going away: nothing is left to point back to.
			if v.Memories[i].Supersedes == id {
				v.Memories[i].Supersedes = ""
			}
		}
		for i := range v.Memories {
			if v.Memories[i].ID == id {
				v.Memories = append(v.Memories[:i], v.Memories[i+1:]...)
				break
			}
		}
		record(v, s.now().UTC(), "", "memory.forgotten", key)
		return nil
	})
}
