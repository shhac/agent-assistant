package app

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/shhac/agent-assistant/internal/core"
	"github.com/shhac/agent-assistant/internal/engine"
	"github.com/shhac/agent-assistant/internal/integrations/worker"
)

// ownerChatExecutor carries authority from the current owner turn. Background
// review, queue and supervision executors cannot synthesize this authority.
type ownerChatExecutor struct {
	app    *App
	turnID string
}

func (s ownerChatExecutor) Execute(ctx context.Context, name string, raw json.RawMessage) (any, error) {
	if name != "control_agent" {
		return s.app.Execute(ctx, name, raw)
	}
	if s.turnID == "" {
		return nil, errors.New("worker control requires a current owner turn")
	}
	var in engine.ControlAgentArgs
	if err := args(raw, &in); err != nil {
		return nil, err
	}
	// One durable operation per action/assignment in this turn, regardless of
	// retries or model-generated tool IDs. A new owner turn is a new request.
	return s.app.ControlAgent(ctx, in.AgentID, in.Action, "chat:"+s.turnID)
}

type AgentInspection struct {
	Agent        core.Agent                 `json:"agent"`
	Controls     AgentControls              `json:"controls"`
	Conversation core.AgentConversationPage `json:"conversation"`
}

func (a *App) InspectAgent(ctx context.Context, id string) (AgentInspection, error) {
	snapshot, err := a.Core.Snapshot(ctx)
	if err != nil {
		return AgentInspection{}, err
	}
	agent, ok := findAgent(snapshot, id)
	if !ok {
		return AgentInspection{}, core.ErrNotFound
	}
	controls, err := a.AgentControls(ctx, id)
	if err != nil {
		return AgentInspection{}, err
	}
	conversation, err := a.Core.AgentConversation(ctx, id, 0, 0, 20)
	if err != nil {
		return AgentInspection{}, err
	}
	return AgentInspection{Agent: agent, Controls: controls, Conversation: conversation}, nil
}

// An admission reservation is not evidence the worker is running. A refusal
// usually means our last snapshot raced a worker transition. Refresh read-only;
// inability to inspect the worker is distinct from uncertain message delivery.
func (a *App) refreshRefusedInstruction(ctx context.Context, agent core.Agent, client *worker.Client) {
	run, err := client.Get(ctx, agent.ExternalID)
	if err == nil {
		err = a.observeRun(ctx, agent, run, client, false)
	}
	if err != nil {
		_ = a.Core.MarkUncertain(ctx, agent.ID, "Message was refused before delivery; the worker's current state could not be refreshed. Inspect the preserved session before further execution.")
	}
}
