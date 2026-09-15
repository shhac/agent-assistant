package app

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/shhac/agent-assistant/internal/core"
	linearapi "github.com/shhac/agent-assistant/internal/integrations/linear"
	slackapi "github.com/shhac/agent-assistant/internal/integrations/slack"
	"github.com/shhac/agent-assistant/internal/integrations/worker"
)

// Run owns deterministic supervision. noDispatch is fixed at process boot;
// changing pause or live configuration cannot enable starts or resumes beneath it.
func (a *App) Run(ctx context.Context, noDispatch bool) error {
	if noDispatch {
		a.SetNoDispatch()
	}
	ctx, cancel := context.WithCancel(ctx)
	var listeners sync.WaitGroup
	defer func() { cancel(); listeners.Wait() }()
	if a.Demo {
		<-ctx.Done()
		return nil
	}
	pending, err := a.Core.PendingEvents(ctx)
	if err != nil {
		return err
	}
	if len(pending) > 0 {
		a.Core.RecordActivity(ctx, "", "recovery.pending", fmt.Sprintf("%d interrupted inbound or outbound operations need inspection. Uncertain effects were not replayed.", len(pending)))
	}
	var slackClient *slackapi.Client
	cfg := a.Config()
	if cfg.Slack.OwnerUserID != "" {
		slackClient, err = slackapi.New(slackapi.Config{BotTokenEnv: cfg.Slack.BotTokenEnv, AppTokenEnv: cfg.Slack.AppTokenEnv, OwnerUserID: cfg.Slack.OwnerUserID}, inbox{a.Core})
		if err != nil {
			a.Status("slack", "Slack", "error", err.Error())
		} else {
			a.Status("slack", "Slack", "configured", "Owner DM listener starting")
			listeners.Add(1)
			go func() {
				defer listeners.Done()
				err := slackClient.Run(ctx, func(c context.Context, m slackapi.Message) (string, error) {
					result, chatErr := a.Chat(c, m.Text)
					if chatErr != nil {
						return "", chatErr
					}
					if e := a.Core.CompleteEvent(c, "slack:"+m.ID); e != nil {
						return "", e
					}
					return result.Message, nil
				})
				if ctx.Err() == nil && err != nil {
					a.Status("slack", "Slack", "error", err.Error())
				}
			}()
		}
	}
	_ = a.SyncLinear(ctx)
	supervise := func() {
		if err := a.tick(ctx, noDispatch); err != nil && ctx.Err() == nil {
			a.Status("workers", "Worker runtimes", "error", err.Error())
		}
		if slackClient != nil {
			a.notify(ctx, slackClient.Notify)
		}
	}
	supervise()
	tick := time.NewTicker(15 * time.Second)
	defer tick.Stop()
	linearTick := time.NewTicker(5 * time.Minute)
	defer linearTick.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
			supervise()
		case <-linearTick.C:
			_ = a.SyncLinear(ctx)
		}
	}
}

type inbox struct{ s *core.Service }

func (i inbox) Claim(ctx context.Context, id string) (bool, error) {
	return i.s.ClaimEvent(ctx, "slack:"+id)
}

func (a *App) SyncLinear(ctx context.Context) error {
	if a.Demo {
		return nil
	}
	cfg := a.Config()
	cliErr := a.syncCLIConnections(ctx)
	if !cfg.LegacyLinearImportEnabled() {
		return cliErr
	}
	c, err := linearapi.New(linearapi.Config{APIKeyEnv: cfg.Linear.APIKeyEnv, TeamIDs: cfg.Linear.TeamIDs})
	if err == nil {
		err = a.syncLinear(ctx, c)
	}
	if err != nil {
		a.Status("linear", "Linear", "error", err.Error())
	}
	return errors.Join(cliErr, err)
}

type assignmentSource interface {
	Assigned(context.Context) (linearapi.Assignments, error)
}

func (a *App) syncLinear(ctx context.Context, c assignmentSource) error {
	result, err := c.Assigned(ctx)
	if err != nil {
		return err
	}
	for _, issue := range result.Issues {
		description := issue.Description + "\nSource: " + issue.URL + "\nLinear status: " + issue.State.Name
		_, err = a.Core.CreateProject(ctx, core.ProjectInput{Title: issue.Identifier + " · " + issue.Title, Description: description, SourceID: "linear:" + issue.ID, AcceptanceCriteria: "Deliver the source outcome: " + issue.Title + ". Define measurable acceptance checks from the source requirements before commissioning work. Source: " + issue.URL})
		if err != nil {
			return err
		}
	}
	a.Status("linear", "Linear", "connected", fmt.Sprintf("%d scoped assignments synced; no work starts without a commission", len(result.Issues)))
	return nil
}
func (a *App) broker(profileID string) (*worker.Client, error) {
	p, err := a.Core.GetProfile(profileID)
	if err != nil {
		return nil, err
	}
	return worker.New(worker.Config{Endpoint: p.Endpoint, APIKeyEnv: p.APIKeyEnv, Capabilities: p.Capabilities})
}
func (a *App) tick(ctx context.Context, noDispatch bool) error {
	if a.Demo {
		return nil
	}
	if err := a.Core.CheckIn(ctx); err != nil {
		return err
	}
	snap, err := a.Core.Snapshot(ctx)
	if err != nil {
		return err
	}
	var failures []error
	for _, agent := range snap.Agents {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if agent.Status == "completed" || agent.Status == "cancelled" {
			if !noDispatch && !snap.Paused && agent.ParentID != "" {
				if err := a.forwardProgress(ctx, agent, worker.Run{ID: agent.ExternalID, Status: agent.Status, Summary: agent.Summary, Evidence: agent.Evidence, UpdatedAt: agent.BrokerUpdatedAt}); err != nil {
					failures = append(failures, err)
				}
			}
			continue
		}
		if err := a.superviseAgent(ctx, agent, noDispatch || snap.Paused); err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", agent.Name, err))
		}
	}
	if !noDispatch && !snap.Paused {
		if err = a.propagateDecisions(ctx); err != nil {
			failures = append(failures, err)
		}
		if err = a.reviewFinished(ctx); err != nil {
			failures = append(failures, err)
		}
	}
	if len(failures) > 0 {
		return errors.Join(failures...)
	}
	if len(a.Config().Workers) > 0 {
		a.Status("workers", "Worker runtimes", "connected", "Supervision active; uncertain operations are reconciled before recovery")
	}
	return nil
}
func (a *App) superviseAgent(ctx context.Context, agent core.Agent, noDispatch bool) error {
	if agent.Status == "queued" && noDispatch {
		return nil
	}
	c, err := a.broker(agent.ProfileID)
	if err != nil {
		return err
	}
	if agent.Status == "queued" {
		// Validate credentials before recording intent; missing local setup is not an
		// ambiguous external effect and must not strand a launch in reconciliation.
		profile, _ := a.Core.GetProfile(agent.ProfileID)
		if profile.APIKeyEnv != "" && os.Getenv(profile.APIKeyEnv) == "" {
			return fmt.Errorf("worker credential environment variable %s is not set", profile.APIKeyEnv)
		}
		agent, err = a.Core.BeginDispatch(ctx, agent.ID)
		if err != nil {
			return err
		}
		role := agent.Role
		if role == "reviewer" || role == "researcher" {
			role = "worker"
		}
		run, startErr := c.Start(ctx, worker.StartRequest{DispatchKey: agent.DispatchKey, AgentID: agent.ID, ProjectID: agent.ProjectID, ParentID: agent.ParentID, Role: role, Task: agent.Task, AcceptanceCriteria: agent.AcceptanceCriteria, Capabilities: agent.Capabilities, CheckInDeadline: agent.NextCheckIn})
		if startErr != nil {
			a.Core.MarkUncertain(ctx, agent.ID, "Start was not acknowledged; reconcile by the saved dispatch key before repeating")
			return startErr
		}
		if err = a.Core.MarkDispatched(ctx, agent.ID, run.ID); err != nil {
			return err
		}
		agent.ExternalID = run.ID
		agent.Status = "running"
		return a.observeRun(ctx, agent, run, c, !noDispatch)
	}
	// Every saved dispatching/resuming intent is read back first, including after
	// restart. A missing run is uncertainty, never permission to call Start again.
	var run worker.Run
	if agent.ExternalID != "" {
		run, err = c.Get(ctx, agent.ExternalID)
	} else {
		run, err = c.Find(ctx, agent.DispatchKey)
	}
	if err != nil {
		reason := "Worker status unavailable; preserving session and checking again"
		if errors.Is(err, worker.ErrNotFound) {
			reason = "Broker has no matching run; inspect the saved dispatch intent before recovery"
		}
		a.Core.MarkUncertain(ctx, agent.ID, reason)
		return err
	}
	if agent.ExternalID == "" {
		if err = a.Core.MarkDispatched(ctx, agent.ID, run.ID); err != nil {
			return err
		}
		agent.ExternalID = run.ID
	}
	if err = a.observeRun(ctx, agent, run, c, !noDispatch); err != nil {
		return err
	}
	// Only a confirmed interrupted result may resume. A saved resuming intent
	// whose response was lost is never repeated, even if the broker still says
	// interrupted; its operation receipt requires inspection.
	if run.Status == "interrupted" && !noDispatch && agent.Status != "resuming" && !(agent.Status == "reconciling" && agent.ResumeKey != "") {
		prepared, resumeErr := a.Core.PrepareResume(ctx, agent.ID)
		if resumeErr != nil {
			return resumeErr
		}
		resumed, resumeErr := c.Resume(ctx, prepared.ExternalID, prepared.ResumeKey, "Resume this existing session with its saved context. Reconcile children before commissioning additional work; preserve the original acceptance criteria and prohibitions.")
		if resumeErr != nil {
			a.Core.MarkUncertain(ctx, agent.ID, "Resume outcome is uncertain; reconcile the existing session before further action")
			return resumeErr
		}
		if resumed.ID != prepared.ExternalID {
			a.Core.MarkUncertain(ctx, agent.ID, "Resume returned a different session; operator inspection required")
			return errors.New("resume returned a different external session")
		}
		if err = a.Core.MarkDispatched(ctx, agent.ID, resumed.ID); err != nil {
			return err
		}
		prepared.Status = "running"
		return a.observeRun(ctx, prepared, resumed, c, !noDispatch)
	}
	return nil
}
func (a *App) observeRun(ctx context.Context, agent core.Agent, run worker.Run, c *worker.Client, allowActions bool) error {
	if run.UpdatedAt.IsZero() || run.UpdatedAt.After(time.Now().Add(time.Minute)) {
		return errors.New("broker report needs a valid updated_at timestamp")
	}
	if run.Status == "interrupted" && agent.ResumeKey != "" && (agent.Status == "resuming" || agent.Status == "reconciling") {
		return a.Core.MarkUncertain(ctx, agent.ID, "Resume acknowledgement is unresolved; the broker still reports interruption. Inspect this operation before another resume.")
	}
	status := run.Status
	if status == "queued" {
		status = "running"
	}
	if status == "failed" {
		status = "blocked"
	}
	if strings.TrimSpace(run.Summary) == "" {
		return errors.New("broker report requires a substantive summary")
	}
	stale := time.Since(run.UpdatedAt) > time.Duration(a.Config().Limits.CheckInMinutes)*time.Minute
	if stale && (status == "running" || status == "waiting") {
		if !run.UpdatedAt.Equal(agent.BrokerUpdatedAt) {
			if _, err := a.Core.UpdateAgent(ctx, agent.ID, core.AgentUpdate{Status: status, Summary: run.Summary, Evidence: run.Evidence, ExternalID: run.ID, UpdatedAt: run.UpdatedAt}); err != nil {
				return err
			}
		}
		if err := a.Core.MarkUncertain(ctx, agent.ID, "Worker report is stale; its process may be alive without making progress"); err != nil {
			return err
		}
	} else if !run.UpdatedAt.Equal(agent.BrokerUpdatedAt) || status != agent.Status || run.Summary != agent.Summary || !reflect.DeepEqual(run.Evidence, agent.Evidence) {
		if _, err := a.Core.UpdateAgent(ctx, agent.ID, core.AgentUpdate{Status: status, Summary: run.Summary, Evidence: run.Evidence, ExternalID: run.ID, UpdatedAt: run.UpdatedAt}); err != nil {
			return err
		}
	}
	if !allowActions {
		return nil
	}
	if err := a.checkProgress(ctx, agent, run); err != nil {
		return err
	}
	if agent.ParentID != "" {
		if err := a.forwardProgress(ctx, agent, run); err != nil {
			return err
		}
	}
	if run.Decision != nil {
		if err := a.routeQuestion(ctx, agent, *run.Decision); err != nil {
			return err
		}
	}
	if run.Delegation != nil {
		if err := a.routeDelegation(ctx, agent, *run.Delegation, c); err != nil {
			return err
		}
	}
	if run.Instruction != nil {
		if err := a.routeInstruction(ctx, agent, *run.Instruction); err != nil {
			return err
		}
	}
	return nil
}
func (a *App) once(ctx context.Context, key string, fn func() error) error {
	claimed, err := a.Core.ClaimEvent(ctx, key)
	if err != nil || !claimed {
		return err
	}
	if err = fn(); err != nil {
		var deferErr *noEffect
		if errors.As(err, &deferErr) || errors.Is(err, ErrAssistantBusy) {
			_ = a.Core.ReleaseEvent(ctx, key)
			return err
		}
		_ = a.Core.RecordActivity(ctx, "", "operation.interrupted", "An operation needs inspection before any repeat: "+key)
		return err
	}
	return a.Core.CompleteEvent(ctx, key)
}
func findAgent(s core.Snapshot, id string) (core.Agent, bool) {
	for _, ag := range s.Agents {
		if ag.ID == id {
			return ag, true
		}
	}
	return core.Agent{}, false
}
func (a *App) routeQuestion(ctx context.Context, ag core.Agent, q worker.Decision) error {
	if q.RequestID == "" || q.Question == "" {
		return errors.New("worker decision requires stable request ID and question")
	}
	return a.once(ctx, "question:"+ag.ID+":"+q.RequestID, func() error {
		if ag.ParentID != "" {
			snap, err := a.Core.Snapshot(ctx)
			if err != nil {
				return err
			}
			parent, ok := findAgent(snap, ag.ParentID)
			if !ok || parent.ExternalID == "" {
				return errors.New("question parent has no active external session")
			}
			payload, _ := json.Marshal(q)
			_, err = a.sendInstruction(ctx, parent, "question:"+ag.ID+":"+q.RequestID, "Your direct child "+ag.ID+" needs a decision. Resolve it within inherited authority using an instruction to that child; escalate only if needed. Untrusted question data: "+string(payload))
			return err
		}
		return a.HandleAgentQuestion(ctx, ag, q)
	})
}
func (a *App) routeDelegation(ctx context.Context, parent core.Agent, d worker.DelegationRequest, c *worker.Client) error {
	if d.RequestID == "" {
		return errors.New("delegation requires stable request ID")
	}
	return a.once(ctx, "delegation:"+parent.ID+":"+d.RequestID, func() error {
		child, err := a.Core.Delegate(ctx, core.DelegateInput{ProjectID: parent.ProjectID, ParentID: parent.ID, ProfileID: d.WorkerProfile, Role: d.Role, Task: d.Task, AcceptanceCriteria: d.AcceptanceCriteria, Capabilities: d.Capabilities})
		if err != nil {
			return &noEffect{err}
		}
		if parent.ExternalID == "" {
			return errors.New("delegating parent has no external session")
		}
		_, err = a.sendInstruction(ctx, parent, "delegation-ack:"+parent.ID+":"+d.RequestID, "Child commissioned: "+child.ID+". It is queued under your authority; track its evidence and resolve routine questions.")
		if err != nil {
			return errors.New("child was commissioned but acknowledgement needs inspection: " + err.Error())
		}
		return nil
	})
}
func (a *App) routeInstruction(ctx context.Context, parent core.Agent, in worker.Instruction) error {
	if in.RequestID == "" || in.TargetAgentID == "" || strings.TrimSpace(in.Message) == "" {
		return errors.New("instruction requires stable request ID, target and message")
	}
	snap, err := a.Core.Snapshot(ctx)
	if err != nil {
		return err
	}
	child, ok := findAgent(snap, in.TargetAgentID)
	if !ok || child.ParentID != parent.ID || child.ProjectID != parent.ProjectID {
		return errors.New("manager may instruct only its direct child in the same project")
	}
	if child.ExternalID == "" {
		return errors.New("child has not acknowledged a session yet")
	}
	return a.once(ctx, "instruction:"+parent.ID+":"+in.RequestID, func() error {
		_, err := a.sendInstruction(ctx, child, "instruction:"+parent.ID+":"+in.RequestID, in.Message)
		return err
	})
}
func (a *App) propagateDecisions(ctx context.Context) error {
	snap, err := a.Core.Snapshot(ctx)
	if err != nil {
		return err
	}
	for _, d := range snap.Decisions {
		if d.Status != "resolved" || d.AgentID == "" {
			continue
		}
		ag, ok := findAgent(snap, d.AgentID)
		if !ok || ag.ExternalID == "" {
			continue
		}
		if err = a.once(ctx, "decision-answer:"+d.ID, func() error {
			_, err := a.sendInstruction(ctx, ag, "decision-answer:"+d.ID, "Owner decision: "+d.Title+"\nAnswer: "+d.Answer)
			return err
		}); err != nil {
			return err
		}
	}
	return nil
}
func (a *App) reviewFinished(ctx context.Context) error {
	snap, err := a.Core.Snapshot(ctx)
	if err != nil {
		return err
	}
	for _, p := range snap.Projects {
		if p.Status == "completed" {
			continue
		}
		count := 0
		ready := true
		for _, ag := range snap.Agents {
			if ag.ProjectID != p.ID {
				continue
			}
			count++
			if ag.Status != "completed" {
				ready = false
			}
		}
		for _, d := range snap.Decisions {
			if d.ProjectID == p.ID && d.Status == "open" {
				ready = false
			}
		}
		if count == 0 || !ready {
			continue
		}
		if err = a.once(ctx, "project-review:"+p.ID+":"+reviewRevision(snap, p.ID, a.Config().Model.Engine+"/"+a.Config().Model.Model+"/"+a.Config().Model.Effort), func() error {
			if a.Config().Model.Model == "" {
				return a.Core.RecordActivity(ctx, p.ID, "project.review_ready", "All commissioned workers supplied evidence. Configure the PA model or review acceptance evidence before closing this project.")
			}
			return a.ReviewProject(ctx, p.ID)
		}); err != nil {
			return err
		}
	}
	return nil
}
func (a *App) notify(ctx context.Context, send func(context.Context, string) error) {
	snap, err := a.Core.Snapshot(ctx)
	if err != nil {
		return
	}
	for _, d := range snap.Decisions {
		if d.Status != "open" {
			continue
		}
		key := "notify:decision:" + d.ID
		_ = a.once(ctx, key, func() error {
			return send(ctx, d.Title+"\nRecommendation: "+d.Recommendation+"\n"+d.Context+"\nResolve this decision in the dashboard.")
		})
	}
	for _, p := range snap.Projects {
		if p.Status != "completed" {
			continue
		}
		_ = a.once(ctx, "notify:completed:"+p.ID, func() error {
			return send(ctx, "Completed: "+p.Title+". Acceptance evidence is recorded in the dashboard.")
		})
	}
	for _, ag := range snap.Agents {
		if ag.Status != "reconciling" && ag.Status != "blocked" {
			continue
		}
		key := "notify:attention:" + ag.ID + ":" + ag.Status + ":" + ag.BrokerUpdatedAt.Format(time.RFC3339Nano)
		_ = a.once(ctx, key, func() error {
			return send(ctx, ag.Name+" needs attention: "+ag.Summary+". I preserved its session and have not launched a duplicate.")
		})
	}
}

func reviewRevision(s core.Snapshot, projectID, model string) string {
	agents := []core.Agent{}
	for _, ag := range s.Agents {
		if ag.ProjectID == projectID {
			agents = append(agents, ag)
		}
	}
	raw, _ := json.Marshal(struct {
		Agents []core.Agent
		Model  string
	}{agents, model})
	return fmt.Sprintf("%x", sha256.Sum256(raw))
}

type noEffect struct{ err error }

func (e *noEffect) Error() string { return e.err.Error() }
func (e *noEffect) Unwrap() error { return e.err }
func (a *App) sendInstruction(ctx context.Context, ag core.Agent, key, message string) (worker.Run, error) {
	if a.Demo || a.dispatchDisabled.Load() {
		return worker.Run{}, &noEffect{errors.New("worker instructions disabled for this boot")}
	}
	c, err := a.broker(ag.ProfileID)
	if err != nil {
		return worker.Run{}, &noEffect{err}
	}
	p, _ := a.Core.GetProfile(ag.ProfileID)
	if p.APIKeyEnv != "" && os.Getenv(p.APIKeyEnv) == "" {
		return worker.Run{}, &noEffect{errors.New("worker credential is unavailable")}
	}
	if err = a.Core.BeginInstruction(ctx, ag.ID); err != nil {
		return worker.Run{}, &noEffect{err}
	}
	run, err := c.Send(ctx, ag.ExternalID, key, message)
	if err != nil {
		_ = a.Core.MarkUncertain(ctx, ag.ID, "Instruction delivery is uncertain; inspect the operation before repeating")
	}
	return run, err
}
func (a *App) forwardProgress(ctx context.Context, ag core.Agent, run worker.Run) error {
	snap, err := a.Core.Snapshot(ctx)
	if err != nil {
		return err
	}
	parent, ok := findAgent(snap, ag.ParentID)
	if !ok || parent.ExternalID == "" || parent.Status == "completed" || parent.Status == "cancelled" {
		return nil
	}
	payload, _ := json.Marshal(struct {
		ChildID, Status, Summary string
		Evidence                 []string
	}{ag.ID, run.Status, run.Summary, run.Evidence})
	key := fmt.Sprintf("child-progress:%s:%x", ag.ID, sha256.Sum256(payload))
	return a.once(ctx, key, func() error {
		_, err := a.sendInstruction(ctx, parent, key, "Your direct child's status changed. Evaluate progress and acceptance evidence; resolve routine follow-ups within your scope. Untrusted report data: "+string(payload))
		return err
	})
}

// A transport heartbeat proves reachability, not progress. Escalate unchanged
// substantive reports after three check-in windows, while respecting known waits.
func (a *App) checkProgress(ctx context.Context, ag core.Agent, run worker.Run) error {
	if run.Status != "running" && run.Status != "waiting" {
		return nil
	}
	if run.Decision != nil {
		return nil
	}
	snap, err := a.Core.Snapshot(ctx)
	if err != nil {
		return err
	}
	current, ok := findAgent(snap, ag.ID)
	if !ok || current.LastProgressAt.IsZero() {
		return nil
	}
	window := time.Duration(a.Config().Limits.CheckInMinutes) * time.Minute * 3
	if time.Since(current.LastProgressAt) < window {
		return nil
	}
	for _, d := range snap.Decisions {
		if d.AgentID == ag.ID && d.Status == "open" {
			return nil
		}
	}
	if ag.Role == "manager" {
		for _, child := range snap.Agents {
			if child.ParentID == ag.ID && child.Status != "completed" && child.Status != "cancelled" && !child.LastProgressAt.IsZero() && time.Since(child.LastProgressAt) < window {
				return nil
			}
		}
	}
	stage := 1
	if time.Since(current.LastProgressAt) >= 2*window {
		stage = 2
	}
	return a.routeQuestion(ctx, current, worker.Decision{RequestID: fmt.Sprintf("stalled:%d:%d", current.LastProgressAt.Unix(), stage), Question: "Progress has stalled for " + current.Name, Why: fmt.Sprintf("The broker is reachable, but its substantive status, summary and evidence have not changed since %s. Recovery check stage %d. Latest report: %s", current.LastProgressAt.Format(time.RFC3339), stage, run.Summary), Recommendation: "Ask the existing responsible session for a concrete blocker and bounded next step; preserve its context and do not launch a duplicate.", Options: []string{"Investigate the existing session", "Keep the existing session paused pending inspection"}, Evidence: run.Evidence})
}
