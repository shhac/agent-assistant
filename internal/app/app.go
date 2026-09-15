// Package app composes the coordination model with the deterministic core.
package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shhac/agent-assistant/internal/config"
	"github.com/shhac/agent-assistant/internal/core"
	"github.com/shhac/agent-assistant/internal/engine"
	"github.com/shhac/agent-assistant/internal/integrations/connections"
)

type App struct {
	connectionClient connections.Client
	dispatchDisabled atomic.Bool
	Core             *core.Service
	mu               sync.RWMutex
	cfg              config.Config
	configPath       string
	Demo             bool
	chat             chan struct{}
	statuses         map[string]core.Integration
}

func New(s *core.Service, cfg config.Config, path string, demo bool) *App {
	return &App{connectionClient: connections.New(), Core: s, cfg: cfg, configPath: path, Demo: demo, chat: make(chan struct{}, 1), statuses: map[string]core.Integration{}}
}
func (a *App) Config() config.Config { a.mu.RLock(); defer a.mu.RUnlock(); return a.cfg }
func (a *App) UpdateConfig(cfg config.Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	oldNetwork, _ := json.Marshal(a.cfg.Dashboard)
	newNetwork, _ := json.Marshal(cfg.Dashboard)
	oldSlack, _ := json.Marshal(a.cfg.Slack)
	newSlack, _ := json.Marshal(cfg.Slack)
	if !bytes.Equal(oldNetwork, newNetwork) || !bytes.Equal(oldSlack, newSlack) {
		return errors.New("dashboard and Slack connection changes require stopping the daemon and editing its config")
	}
	if err := config.Save(a.configPath, cfg); err != nil {
		return err
	}
	if err := a.Core.UpdateConfig(cfg); err != nil {
		return err
	}
	a.cfg = cfg
	return nil
}
func (a *App) Status(id, name, state, detail string) {
	a.mu.Lock()
	a.statuses[id] = core.Integration{ID: id, Name: name, Status: state, Detail: detail}
	a.mu.Unlock()
}
func (a *App) Snapshot(ctx context.Context) (core.Snapshot, error) {
	s, err := a.Core.Snapshot(ctx)
	if err != nil {
		return s, err
	}
	cfg := a.Config()
	s.Integrations = []core.Integration{{ID: "model", Name: "Assistant model", Status: "not_configured", Detail: "Choose a model in Settings"}, {ID: "linear", Name: "Linear", Status: "not_configured", Detail: "Use a named lin connection; legacy API integration remains available"}, {ID: "slack", Name: "Slack", Status: "not_configured", Detail: "Configure owner identity and Socket Mode credentials"}, {ID: "workers", Name: "Worker runtimes", Status: "not_configured", Detail: "Add an approved execution broker"}}
	if cfg.Model.Model != "" {
		s.Integrations[0].Status = "configured"
		s.Integrations[0].Detail = strings.Join([]string{cfg.Model.Engine, cfg.Model.Model, cfg.Model.Effort}, " / ")
	}
	if len(cfg.Workers) > 0 {
		s.Integrations[3].Status = "configured"
		s.Integrations[3].Detail = fmt.Sprintf("%d approved profiles", len(cfg.Workers))
	}
	for _, c := range cfg.Connections {
		state, detail := "configured", "Read-only CLI accounts: "+strings.Join(c.Profiles, ", ")
		if c.Tool == "agent-notion" {
			if len(c.Profiles) == 0 {
				state, detail = "configured", "Read-only CLI default account"
			} else {
				state, detail = "unavailable", "Choose the CLI default account for Notion"
			}
		}
		s.Integrations = append(s.Integrations, core.Integration{ID: "connection:" + c.ID, Name: c.Name, Status: state, Detail: detail})
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	for i, st := range s.Integrations {
		if live, ok := a.statuses[st.ID]; ok {
			s.Integrations[i] = live
		}
	}
	return s, nil
}
func (a *App) context(ctx context.Context) (json.RawMessage, []engine.Message, error) {
	s, err := a.Snapshot(ctx)
	if err != nil {
		return nil, nil, err
	}
	history := []engine.Message{}
	start := len(s.Messages) - 24
	if start < 0 {
		start = 0
	}
	for _, m := range s.Messages[start:] {
		if m.Role == "user" || m.Role == "assistant" {
			history = append(history, engine.Message{Role: m.Role, Content: m.Content})
		}
	}
	s.Messages = []core.Message{}
	if len(s.Activity) > 40 {
		s.Activity = s.Activity[len(s.Activity)-40:]
	}
	type profile struct {
		ProjectID    string   `json:"project_id,omitempty"`
		ID           string   `json:"id"`
		Name         string   `json:"name"`
		Capabilities []string `json:"capabilities"`
	}
	profiles := []profile{}
	for _, p := range a.Config().Workers {
		profiles = append(profiles, profile{p.ProjectID, p.ID, p.Name, p.Capabilities})
	}
	raw, err := json.Marshal(struct {
		State       core.Snapshot       `json:"state"`
		Profiles    []profile           `json:"worker_profiles"`
		Connections []config.Connection `json:"connections"`
	}{s, profiles, a.Config().Connections})
	return raw, history, err
}
func (a *App) Chat(ctx context.Context, message string) (engine.Result, error) {
	if len(strings.TrimSpace(message)) == 0 || len(message) > 24000 {
		return engine.Result{}, errors.New("message must contain 1–24000 characters")
	}
	select {
	case a.chat <- struct{}{}:
		defer func() { <-a.chat }()
	case <-ctx.Done():
		return engine.Result{}, ctx.Err()
	}
	if a.Demo {
		return engine.Result{}, errors.New("demo mode does not invoke models or workers; start without --demo and configure a model to chat")
	}
	cfg := a.Config()
	e, err := engine.New(engine.Config{WorkDirRoot: a.Core.StateDirectory(), Engine: cfg.Model.Engine, Effort: cfg.Model.Effort, CodexBin: cfg.Model.CodexBin, CodexHome: cfg.Model.CodexHome, Endpoint: strings.TrimRight(cfg.Model.BaseURL, "/") + "/chat/completions", Model: cfg.Model.Model, APIKeyEnv: cfg.Model.APIKeyEnv, AssistantName: cfg.Assistant.Name, Personality: cfg.Assistant.Personality, MaxTurns: cfg.Limits.MaxModelTurns, MaxOutputTokens: cfg.Model.MaxTokens, BeforeRequest: func(ctx context.Context) error {
		return a.Core.ReserveModelCall(ctx, a.Config().Limits.MaxModelCallsPerDay)
	}}, a)
	if err != nil {
		return engine.Result{}, err
	}
	raw, history, err := a.context(ctx)
	if err != nil {
		return engine.Result{}, err
	}
	if _, err = a.Core.AddMessage(ctx, "user", message); err != nil {
		return engine.Result{}, err
	}
	result, err := e.Chat(ctx, engine.Request{Message: message, History: history, Context: raw})
	saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err != nil {
		_, _ = a.Core.AddMessage(saveCtx, "system", err.Error()+". Any recorded coordination actions remain visible; no automatic replay was attempted.")
		return result, err
	}
	_, err = a.Core.AddMessage(saveCtx, "assistant", result.Message)
	return result, err
}
func args(raw json.RawMessage, v any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return errors.New("invalid trailing arguments")
	}
	return nil
}
func (a *App) Execute(ctx context.Context, name string, raw json.RawMessage) (any, error) {
	switch name {
	case "list_connections", "query_connection":
		return a.runConnectionTool(ctx, name, raw)
	case "message_agent":
		var in struct {
			AgentID string `json:"agent_id"`
			Message string `json:"message"`
		}
		if err := args(raw, &in); err != nil {
			return nil, err
		}
		snap, err := a.Core.Snapshot(ctx)
		if err != nil {
			return nil, err
		}
		for _, agent := range snap.Agents {
			if agent.ID == in.AgentID {
				return a.SendAgent(ctx, agent, in.Message)
			}
		}
		return nil, core.ErrNotFound

	case "read_state":
		if err := args(raw, &struct{}{}); err != nil {
			return nil, err
		}
		state, _, err := a.context(ctx)
		if err != nil {
			return nil, err
		}
		return state, nil
	case "create_project":
		var in engine.CreateProjectArgs
		if err := args(raw, &in); err != nil {
			return nil, err
		}
		return a.Core.CreateProject(ctx, core.ProjectInput{Directories: in.Directories, Title: in.Title, Description: in.Objective, AcceptanceCriteria: strings.Join(in.AcceptanceCriteria, "\n")})
	case "update_project":
		var in engine.UpdateProjectArgs
		if err := args(raw, &in); err != nil {
			return nil, err
		}
		return a.Core.RefineProjectWithDirectories(ctx, in.ProjectID, in.Objective, strings.Join(in.AcceptanceCriteria, "\n"), in.Directories)
	case "delegate":
		var in engine.DelegateArgs
		if err := args(raw, &in); err != nil {
			return nil, err
		}
		return a.Core.Delegate(ctx, core.DelegateInput{ProjectID: in.ProjectID, ParentID: in.ParentID, ProfileID: in.WorkerProfile, Role: in.Role, Task: in.Objective, AcceptanceCriteria: strings.Join(in.AcceptanceCriteria, "\n")})
	case "ask_decision":
		var in engine.DecisionArgs
		if err := args(raw, &in); err != nil {
			return nil, err
		}
		why := in.Why
		if len(in.Evidence) > 0 {
			why += "\nEvidence: " + strings.Join(in.Evidence, "; ")
		}
		return a.Core.CreateDecision(ctx, core.DecisionInput{ProjectID: in.ProjectID, Title: in.Question, Context: why, Recommendation: in.Recommendation, Choices: in.Options})
	case "remember_preference":
		var in engine.PreferenceArgs
		if err := args(raw, &in); err != nil {
			return nil, err
		}
		return a.Core.Remember(ctx, in.Key, in.Value)
	case "report_status":
		var in engine.StatusArgs
		if err := args(raw, &in); err != nil {
			return nil, err
		}
		return map[string]bool{"recorded": true}, a.Core.RecordActivity(ctx, in.ProjectID, "assistant.update", in.Summary+evidenceText(in.Evidence))
	case "complete_project":
		var in struct {
			ProjectID string   `json:"project_id"`
			Evidence  []string `json:"evidence"`
		}
		if err := args(raw, &in); err != nil {
			return nil, err
		}
		return map[string]bool{"completed": true}, a.Core.CompleteProject(ctx, in.ProjectID, in.Evidence)
	default:
		return nil, errors.New("unavailable coordination action")
	}
}
func evidenceText(e []string) string {
	if len(e) == 0 {
		return ""
	}
	return "\nEvidence: " + strings.Join(e, "; ")
}

// SetNoDispatch is a boot-only restriction; it cannot be removed by live configuration.
func (a *App) SetNoDispatch() { a.dispatchDisabled.Store(true) }
