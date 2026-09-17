package workerbroker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/shhac/agent-assistant/internal/engine"
	"github.com/shhac/lib-agent-harness/completion"
)

func transcriptDigest(messages []modelMessage) string {
	raw, _ := json.Marshal(messages)
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

// workingMessages never edits the archive. A changed repaired prefix invalidates
// the optimization, so recovery re-derives context rather than skipping evidence.
func workingMessages(run storedRun) []modelMessage {
	messages := []modelMessage{{Role: "system", Content: workerPrompt(run.Request)}}
	if run.ContextThrough > 0 && run.ContextThrough <= len(run.Transcript) && run.ContextDigest != "" && transcriptDigest(run.Transcript[:run.ContextThrough]) == run.ContextDigest {
		messages = append(messages, run.WorkingContext...)
		return append(messages, run.Transcript[run.ContextThrough:]...)
	}
	return append(messages, run.Transcript...)
}

func (b *Broker) prepareContext(ctx context.Context, id string) ([]modelMessage, error) {
	// Queue removal and archive append share a transaction. Owner messages arriving
	// during compaction remain queued for the following turn, never overwritten.
	if err := b.update(id, func(run *storedRun) error {
		for _, message := range run.Messages {
			run.Transcript = append(run.Transcript, modelMessage{Role: "user", Content: message})
		}
		run.Messages = nil
		return nil
	}); err != nil {
		return nil, err
	}
	run, err := b.snapshot(id)
	if err != nil {
		return nil, err
	}
	messages := workingMessages(run)
	toolBytes, _ := json.Marshal(workerTools())
	maxBytes := 128*1024 - len(toolBytes) - 2048
	checkpoint, _, err := engine.CompactContext(ctx, messages, engine.ContextOptions{MaxBytes: maxBytes}, func(ctx context.Context, input []engine.Message) (engine.Message, engine.Usage, error) {
		cfg := b.modelConfig(id)
		cfg.MaxOutputTokens = min(cfg.MaxOutputTokens, 2048)
		return engine.Complete(ctx, cfg, input, nil)
	})
	if err != nil {
		return nil, workerModelDiagnostic(err)
	}
	if !checkpoint.Compacted {
		raw, _ := json.Marshal(messages)
		if err = b.update(id, func(current *storedRun) error { current.Run.ContextBytes = len(raw); return nil }); err != nil {
			return nil, err
		}
		return messages, nil
	}
	if len(checkpoint.Messages) == 0 || checkpoint.Messages[0].Role != "system" || checkpoint.Messages[0].Content != workerPrompt(run.Request) {
		return nil, workerCompletionDiagnostic("worker contract was not preserved during context compaction", completion.PhaseResponse, "worker_contract_changed")
	}
	through := len(run.Transcript)
	digest := transcriptDigest(run.Transcript)
	if err = b.update(id, func(current *storedRun) error {
		if len(current.Transcript) < through || transcriptDigest(current.Transcript[:through]) != digest {
			return workerCompletionDiagnostic("worker context changed during compaction; original history retained", completion.PhaseResponse, "worker_context_changed")
		}
		current.WorkingContext = append([]modelMessage(nil), checkpoint.Messages[1:]...)
		current.ContextThrough = through
		current.ContextDigest = digest
		current.ContextCheckpoints = append(current.ContextCheckpoints, checkpoint)
		current.Run.ContextCompactions = len(current.ContextCheckpoints)
		current.Run.ContextBytes = checkpoint.AfterBytes
		current.Run.Summary = "Earlier exchanges compacted into a working checkpoint; full transcript and execution evidence preserved"
		current.Run.UpdatedAt = now()
		return nil
	}); err != nil {
		return nil, err
	}
	return checkpoint.Messages, nil
}
