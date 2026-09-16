package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type SteeringMessage struct {
	ID         string    `json:"id"`
	WorkItemID string    `json:"work_item_id"`
	Content    string    `json:"content"`
	CreatedAt  time.Time `json:"created_at"`
}
type SteeringReceipt struct {
	MessageID      string    `json:"message_id"`
	AgentID        string    `json:"agent_id"`
	AcknowledgedAt time.Time `json:"acknowledged_at"`
}

// AddSteering durably attaches direction to the outcome, not a transient
// session. The caller supplies an idempotency key and retries with the same text.
func (s *Service) AddSteering(ctx context.Context, workItemID, messageID, content string) (SteeringMessage, error) {
	content = strings.TrimSpace(content)
	if !required(workItemID, messageID, content) || len(messageID) > 200 || strings.TrimSpace(messageID) != messageID || len(content) > 16000 {
		return SteeringMessage{}, errors.New("steering requires a work item, bounded message ID and content")
	}
	out := SteeringMessage{ID: messageID, WorkItemID: workItemID, Content: content, CreatedAt: s.now().UTC()}
	err := s.store.update(ctx, func(v *Snapshot) error {
		messages := []SteeringMessage{}
		for _, m := range v.Steering {
			if m.ID == messageID {
				if m.WorkItemID != workItemID || m.Content != content {
					return fmt.Errorf("steering ID already used for different content or scope: %w", ErrConflict)
				}
				out = m
				return nil
			}
			if m.WorkItemID == workItemID {
				messages = append(messages, m)
			}
		}
		w := workItem(v, workItemID)
		if w == nil {
			return ErrNotFound
		}
		if workItemClosed(*w) {
			return errors.New("create a new work item to direct work after acceptance")
		}
		encoded, err := json.Marshal(append(messages, out))
		if err != nil {
			return err
		}
		if len(encoded) > 24*1024 {
			return errors.New("work item steering payload budget exceeded (24 KiB)")
		}
		if len(messages) >= 200 {
			return errors.New("work item steering limit reached")
		}
		v.Steering = append(v.Steering, out)
		w.UpdatedAt = out.CreatedAt
		record(v, out.CreatedAt, w.ProjectID, "work_item.steered", w.Title+": "+content)
		return nil
	})
	return out, err
}

func (s *Service) SteeringForAgent(ctx context.Context, id string) ([]SteeringMessage, error) {
	v, err := s.store.Snapshot(ctx)
	if err != nil {
		return nil, err
	}
	a := agent(&v, id)
	if a == nil {
		return nil, ErrNotFound
	}
	out := []SteeringMessage{}
	for _, m := range v.Steering {
		if m.WorkItemID == a.WorkItemID {
			out = append(out, m)
		}
	}
	return out, nil
}

// Acknowledgement confirms the agent read direction; it does not prove it was
// implemented. Acceptance separately requires evidence against the contract.
func (s *Service) AcknowledgeSteering(ctx context.Context, agentID string, ids []string) error {
	if len(ids) > 200 {
		return errors.New("too many steering acknowledgements")
	}
	return s.store.update(ctx, func(v *Snapshot) error {
		a := agent(v, agentID)
		if a == nil {
			return ErrNotFound
		}
		missing := []string{}
		for _, id := range ids {
			found := false
			for _, m := range v.Steering {
				if m.ID == id && m.WorkItemID == a.WorkItemID {
					found = true
					break
				}
			}
			if !found {
				return errors.New("steering acknowledgement is outside the agent's work item")
			}
			seen := false
			for _, r := range v.SteeringReceipts {
				if r.MessageID == id && r.AgentID == agentID {
					seen = true
					break
				}
			}
			if !seen && !contains(missing, id) {
				missing = append(missing, id)
			}
		}
		if len(missing) == 0 {
			return nil
		}
		if a.Status == "cancelled" || a.ExternalID == "" {
			return errors.New("new steering acknowledgements require an assigned, non-cancelled external session")
		}
		for _, id := range missing {
			v.SteeringReceipts = append(v.SteeringReceipts, SteeringReceipt{MessageID: id, AgentID: agentID, AcknowledgedAt: s.now().UTC()})
		}
		return nil
	})
}
