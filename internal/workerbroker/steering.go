package workerbroker

import (
	"errors"
	"strings"
)

// Receipts are cumulative so a daemon can reconcile them after either process
// restarts. The daemon validates that IDs belong to this execution's work item.
func (b *Broker) acknowledgeSteering(id, arguments string) (any, bool, error) {
	var in struct {
		MessageIDs []string `json:"message_ids"`
	}
	if strict([]byte(arguments), &in) != nil || len(in.MessageIDs) == 0 || len(in.MessageIDs) > 200 {
		return nil, false, errors.New("provide 1–200 steering message IDs")
	}
	for _, value := range in.MessageIDs {
		if strings.TrimSpace(value) != value || value == "" || len(value) > 200 {
			return nil, false, errors.New("invalid steering message ID")
		}
	}
	err := b.update(id, func(run *storedRun) error {
		if run.Run.Status != "running" || run.PendingStatus != "" {
			return errInterrupted
		}
		ids := append([]string{}, run.Run.SteeringAcknowledgements...)
		for _, value := range in.MessageIDs {
			if !contains(ids, value) {
				ids = append(ids, value)
			}
		}
		if len(ids) > 200 {
			return errors.New("steering receipt limit reached")
		}
		if len(ids) != len(run.Run.SteeringAcknowledgements) {
			run.Run.SteeringAcknowledgements = ids
			run.Run.UpdatedAt = now()
		}
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return map[string]any{"acknowledged": in.MessageIDs, "meaning": "read, not implemented"}, false, nil
}
