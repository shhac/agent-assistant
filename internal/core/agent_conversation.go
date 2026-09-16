package core

import (
	"context"
	"errors"
	"time"
	"unicode/utf8"
)

type AgentConversationEntry struct {
	Sequence  int64     `json:"sequence"`
	ID        string    `json:"id"`
	AgentID   string    `json:"agent_id"`
	Kind      string    `json:"kind"`
	Direction string    `json:"direction"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}
type AgentConversationPage struct {
	HistoryLimited bool                     `json:"history_limited"`
	Messages       []AgentConversationEntry `json:"messages"`
	NextCursor     int64                    `json:"next_cursor"`
	OldestSequence int64                    `json:"oldest_sequence"`
	HasMore        bool                     `json:"has_more"`
	Truncated      bool                     `json:"truncated"`
}

// RecordAgentConversation records only application-visible communication. Native
// model reasoning and implementation tool payloads are deliberately excluded.
func (s *Service) RecordAgentConversation(ctx context.Context, id, key, kind, direction, content string) error {
	if !required(key, kind, direction, content) {
		return errors.New("conversation entry needs identity and content")
	}
	if len(content) > 8192 {
		content = content[:8192]
		for !utf8.ValidString(content) {
			content = content[:len(content)-1]
		}
		content += "\n[Message excerpt truncated]"
	}
	return s.store.update(ctx, func(v *Snapshot) error {
		if agent(v, id) == nil {
			return ErrNotFound
		}
		for _, entry := range v.AgentConversation {
			if entry.ID == key && entry.AgentID == id {
				return nil
			}
		}
		if v.ConversationDropped == nil {
			v.ConversationDropped = map[string]bool{}
		}
		v.ConversationSequence++
		v.AgentConversation = append(v.AgentConversation, AgentConversationEntry{Sequence: v.ConversationSequence, ID: key, AgentID: id, Kind: kind, Direction: direction, Content: content, CreatedAt: s.now().UTC()})
		count := 0
		kept := make([]AgentConversationEntry, 0, len(v.AgentConversation))
		for i := len(v.AgentConversation) - 1; i >= 0; i-- {
			entry := v.AgentConversation[i]
			if entry.AgentID == id {
				count++
				if count > 500 {
					v.ConversationDropped[id] = true
					continue
				}
			}
			kept = append(kept, entry)
		}
		for i, j := 0, len(kept)-1; i < j; i, j = i+1, j-1 {
			kept[i], kept[j] = kept[j], kept[i]
		}
		if len(kept) > 10000 {
			for _, entry := range kept[:len(kept)-10000] {
				v.ConversationDropped[entry.AgentID] = true
			}
			kept = kept[len(kept)-10000:]
		}
		v.AgentConversation = kept
		return nil
	})
}
func (s *Service) AgentConversation(ctx context.Context, id string, after, before int64, limit int) (AgentConversationPage, error) {
	out := AgentConversationPage{Messages: []AgentConversationEntry{}}
	if after < 0 || before < 0 || after > 0 && before > 0 {
		return out, errors.New("choose a nonnegative after or before cursor")
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	v, err := s.store.Snapshot(ctx)
	if err != nil {
		return out, err
	}
	if agent(&v, id) == nil {
		return out, ErrNotFound
	}
	out.HistoryLimited = v.ConversationDropped[id]
	all := []AgentConversationEntry{}
	for _, e := range v.AgentConversation {
		if e.AgentID == id && (after == 0 || e.Sequence > after) && (before == 0 || e.Sequence < before) {
			all = append(all, e)
		}
	}
	if after > 0 {
		out.HasMore = len(all) > limit
		if len(all) > limit {
			all = all[:limit]
		}
	} else if len(all) > limit {
		out.Truncated = true
		all = all[len(all)-limit:]
	}
	out.Messages = all
	if len(all) > 0 {
		out.NextCursor = all[len(all)-1].Sequence
		out.OldestSequence = all[0].Sequence
	}
	return out, nil
}
