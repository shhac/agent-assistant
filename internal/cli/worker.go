package cli

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/shhac/agent-assistant/internal/config"
	"github.com/shhac/agent-assistant/internal/diagnostics"
	"github.com/shhac/agent-assistant/internal/workerbroker"
	"github.com/spf13/cobra"
)

func registerWorker(root *cobra.Command, o *options) {
	var workspace, project, image, socket, state, addr, tokenEnv, model, engineName, effort string
	var turns, tokens, concurrency int
	worker := &cobra.Command{Use: "worker", Short: "Run an isolated coding-worker broker for an approved project"}
	cmd := &cobra.Command{Use: "serve", Short: "Serve a private worker endpoint backed by an offline Docker sandbox", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(o.configPath)
		if err != nil {
			return err
		}
		profile := cfg.WorkerModel
		if cmd.Flags().Changed("engine") {
			profile.Engine = engineName
		}
		if cmd.Flags().Changed("model") {
			profile.Model = model
		}
		if cmd.Flags().Changed("effort") {
			profile.Effort = effort
		}
		if cmd.Flags().Changed("max-output-tokens") {
			profile.MaxTokens = tokens
		}
		cfg.WorkerModel = profile
		if err := cfg.Validate(); err != nil {
			return err
		}
		if state == "" {
			state = o.statePath + ".workers"
		}
		host, _, err := net.SplitHostPort(addr)
		if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
			return errors.New("worker address must be a loopback IP and port")
		}
		listener, err := net.Listen("tcp", addr)
		if err != nil {
			return err
		}
		defer listener.Close()
		broker, err := workerbroker.New(workerbroker.Config{Diagnostics: diagnostics.New(cmd.ErrOrStderr()), StateDir: state, Workspace: workspace, ProjectID: project, Image: image, DockerSocket: socket, Engine: profile.Engine, Effort: profile.Effort, CodexBin: profile.CodexBin, CodexHome: profile.CodexHome, ClaudeBin: profile.ClaudeBin, ClaudeHome: profile.ClaudeHome, ModelEndpoint: strings.TrimRight(profile.BaseURL, "/") + "/chat/completions", Model: profile.Model, APIKeyEnv: profile.APIKeyEnv, TokenEnv: tokenEnv, MaxTurns: turns, MaxOutputTokens: profile.MaxTokens, MaxConcurrent: concurrency})
		if err != nil {
			return err
		}
		defer broker.Close()
		ctx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer cancel()
		web := &http.Server{Handler: broker.Handler(), ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20, BaseContext: func(net.Listener) context.Context { return ctx }}
		runtimeDone := make(chan error, 1)
		httpDone := make(chan error, 1)
		go func() { runtimeDone <- broker.Run(ctx) }()
		go func() { httpDone <- web.Serve(listener) }()
		info := broker.Info()
		info["url"] = "http://" + listener.Addr().String()
		info["token_env"] = tokenEnv
		info["state"] = state
		_ = o.emit(info)
		runtimeStopped := false
		select {
		case <-ctx.Done():
		case err = <-runtimeDone:
			runtimeStopped = true
		case err = <-httpDone:
			if errors.Is(err, http.ErrServerClosed) {
				err = nil
			}
		}
		cancel()
		shutdown, stop := context.WithTimeout(context.Background(), 15*time.Second)
		defer stop()
		if shutdownErr := web.Shutdown(shutdown); shutdownErr != nil {
			_ = web.Close()
			if err == nil {
				err = shutdownErr
			}
		}
		if !runtimeStopped {
			runtimeErr := <-runtimeDone
			if err == nil {
				err = runtimeErr
			}
		}
		return err
	}}
	cmd.Flags().StringVar(&workspace, "workspace", "", "Dedicated approved source workspace to copy (original remains untouched)")
	cmd.Flags().StringVar(&project, "project", "", "Exact assistant project ID this broker may work on")
	cmd.Flags().StringVar(&image, "image", "", "Locally installed image pinned by sha256 digest; never pulled automatically")
	cmd.Flags().StringVar(&socket, "docker-socket", "", "Local Docker Unix socket (defaults to /var/run/docker.sock)")
	cmd.Flags().StringVar(&state, "worker-state", "", "Private worker state directory (defaults beside assistant state)")
	cmd.Flags().StringVar(&addr, "http", "127.0.0.1:8350", "Loopback worker API address")
	cmd.Flags().StringVar(&tokenEnv, "token-env", "AGENT_ASSISTANT_WORKER_TOKEN", "Environment variable containing the broker API token")
	cmd.Flags().StringVar(&model, "model", "", "Worker model (defaults to worker_model.model)")
	cmd.Flags().StringVar(&engineName, "engine", "", "Worker engine (defaults to worker_model.engine)")
	cmd.Flags().StringVar(&effort, "effort", "", "Reasoning effort (defaults to worker_model.effort)")
	cmd.Flags().IntVar(&turns, "max-turns", 24, "Maximum cumulative model calls per worker session, including resumes")
	cmd.Flags().IntVar(&tokens, "max-output-tokens", 4096, "Maximum output tokens per worker model request (otherwise worker_model.max_tokens)")
	cmd.Flags().IntVar(&concurrency, "max-concurrent", 1, "Maximum simultaneously executing local workers")
	_ = cmd.MarkFlagRequired("workspace")
	_ = cmd.MarkFlagRequired("project")
	_ = cmd.MarkFlagRequired("image")
	worker.AddCommand(cmd)
	root.AddCommand(worker)
}
