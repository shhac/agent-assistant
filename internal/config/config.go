// Package config defines owner-controlled configuration. Credentials are environment references.
package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const DefaultAssistantName = "Milo"

type Config struct {
	Assistant Assistant `json:"assistant"`
	Dashboard Dashboard `json:"dashboard"`
	Model     Model     `json:"model"`
	Slack     Slack     `json:"slack"`
	Linear    Linear    `json:"linear"`
	Limits    Limits    `json:"limits"`
	Workers   []Worker  `json:"workers"`
}
type Assistant struct {
	Name        string `json:"name"`
	Personality string `json:"personality"`
}
type Dashboard struct {
	Addr          string   `json:"addr"`
	Tailscale     string   `json:"tailscale"`
	TailscalePort int      `json:"tailscale_port"`
	AllowedUsers  []string `json:"allowed_users"`
}
type Model struct {
	BaseURL   string `json:"base_url"`
	Model     string `json:"model"`
	APIKeyEnv string `json:"api_key_env"`
	MaxTokens int    `json:"max_tokens"`
}
type Slack struct {
	BotTokenEnv string `json:"bot_token_env"`
	AppTokenEnv string `json:"app_token_env"`
	OwnerUserID string `json:"owner_user_id"`
}
type Linear struct {
	APIKeyEnv string   `json:"api_key_env"`
	TeamIDs   []string `json:"team_ids"`
}
type Limits struct {
	MaxModelCallsPerDay int `json:"max_model_calls_per_day"`
	MaxModelTurns       int `json:"max_model_turns"`
	MaxAgents           int `json:"max_agents"`
	MaxDepth            int `json:"max_depth"`
	MaxRecoveries       int `json:"max_recoveries"`
	CheckInMinutes      int `json:"check_in_minutes"`
}
type Worker struct {
	ProjectID    string   `json:"project_id,omitempty"`
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Endpoint     string   `json:"endpoint"`
	APIKeyEnv    string   `json:"api_key_env"`
	Capabilities []string `json:"capabilities"`
}
type FilePaths struct {
	Config string
	State  string
}

func Default() Config {
	return Config{
		Assistant: Assistant{Name: DefaultAssistantName, Personality: "Calm, concise and proactive. Bring clear recommendations and evidence; handle the chasing."},
		Dashboard: Dashboard{Addr: "127.0.0.1:8340", Tailscale: "off", TailscalePort: 8443, AllowedUsers: []string{}},
		Model:     Model{BaseURL: "https://api.openai.com/v1", APIKeyEnv: "OPENAI_API_KEY", MaxTokens: 4096},
		Slack:     Slack{BotTokenEnv: "SLACK_BOT_TOKEN", AppTokenEnv: "SLACK_APP_TOKEN"},
		Linear:    Linear{APIKeyEnv: "LINEAR_API_KEY", TeamIDs: []string{}},
		Limits:    Limits{MaxModelCallsPerDay: 100, MaxModelTurns: 8, MaxAgents: 4, MaxDepth: 3, MaxRecoveries: 2, CheckInMinutes: 30}, Workers: []Worker{},
	}
}

func Paths() (FilePaths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return FilePaths{}, err
	}
	configRoot := os.Getenv("XDG_CONFIG_HOME")
	if configRoot == "" {
		configRoot = filepath.Join(home, ".config")
	}
	stateRoot := os.Getenv("XDG_STATE_HOME")
	if stateRoot == "" {
		stateRoot = filepath.Join(home, ".local", "state")
	}
	return FilePaths{Config: filepath.Join(configRoot, "agent-assistant", "config.json"), State: filepath.Join(stateRoot, "agent-assistant", "state.db")}, nil
}
func Load(path string) (Config, error) {
	c := Default()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err = dec.Decode(&c); err != nil {
		return c, fmt.Errorf("decode config: %w", err)
	}
	if err = dec.Decode(new(any)); err != io.EOF {
		return c, errors.New("config must contain one JSON object")
	}
	return c, c.Validate()
}
func Save(path string, c Config) error {
	if err := c.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".config-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

var envName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func (c Config) Validate() error {
	if strings.TrimSpace(c.Assistant.Name) == "" || len(c.Assistant.Name) > 80 {
		return errors.New("assistant.name must contain 1–80 characters")
	}
	host, port, err := net.SplitHostPort(c.Dashboard.Addr)
	if err != nil {
		return errors.New("dashboard.addr must be a loopback host:port")
	}
	portNumber, portErr := strconv.Atoi(port)
	if portErr != nil || portNumber < 1 || portNumber > 65535 {
		return errors.New("dashboard.addr port must be between 1 and 65535")
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return errors.New("dashboard.addr must use a loopback IP")
	}
	if c.Dashboard.Tailscale != "off" && c.Dashboard.Tailscale != "serve" {
		return errors.New("dashboard.tailscale must be off or serve; public Funnel is not supported")
	}
	if c.Dashboard.TailscalePort != 443 && c.Dashboard.TailscalePort != 8443 && c.Dashboard.TailscalePort != 10000 {
		return errors.New("dashboard.tailscale_port must be 443, 8443 or 10000")
	}
	if c.Dashboard.Tailscale == "serve" && len(c.Dashboard.AllowedUsers) == 0 {
		return errors.New("dashboard.allowed_users is required for Tailscale Serve")
	}
	if c.Limits.MaxModelCallsPerDay < 1 || c.Limits.MaxModelCallsPerDay > 100000 {
		return errors.New("limits.max_model_calls_per_day must be between 1 and 100000")
	}
	if c.Limits.MaxModelTurns < 1 || c.Limits.MaxModelTurns > 32 {
		return errors.New("limits.max_model_turns must be between 1 and 32")
	}
	if c.Limits.MaxAgents < 1 || c.Limits.MaxAgents > 64 {
		return errors.New("limits.max_agents must be between 1 and 64")
	}
	if c.Limits.MaxDepth < 1 || c.Limits.MaxDepth > 10 {
		return errors.New("limits.max_depth must be between 1 and 10")
	}
	if c.Limits.MaxRecoveries < 0 || c.Limits.MaxRecoveries > 10 {
		return errors.New("limits.max_recoveries must be between 0 and 10")
	}
	if c.Limits.CheckInMinutes < 1 || c.Limits.CheckInMinutes > 1440 {
		return errors.New("limits.check_in_minutes must be between 1 and 1440")
	}
	if c.Model.MaxTokens < 128 || c.Model.MaxTokens > 131072 {
		return errors.New("model.max_tokens must be between 128 and 131072")
	}
	if err = validateEndpoint(c.Model.BaseURL); err != nil {
		return fmt.Errorf("model.base_url: %w", err)
	}
	envs := []string{c.Model.APIKeyEnv, c.Slack.BotTokenEnv, c.Slack.AppTokenEnv, c.Linear.APIKeyEnv}
	ids := map[string]bool{}
	for _, w := range c.Workers {
		if w.ID == "" || ids[w.ID] {
			return errors.New("workers must have unique nonempty IDs")
		}
		ids[w.ID] = true
		if err = validateEndpoint(w.Endpoint); err != nil {
			return fmt.Errorf("worker %s endpoint: %w", w.ID, err)
		}
		for _, cap := range w.Capabilities {
			if cap != "coordinate" && cap != "implement" && cap != "review" && cap != "research" {
				return fmt.Errorf("worker %s has prohibited or unknown capability %q", w.ID, cap)
			}
		}
		envs = append(envs, w.APIKeyEnv)
	}
	for _, e := range envs {
		if e != "" && !envName.MatchString(e) {
			return errors.New("credential references must be environment variable names")
		}
	}
	return nil
}
func validateEndpoint(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("use an absolute URL without credentials, query or fragment")
	}
	if u.Scheme == "https" {
		return nil
	}
	ip := net.ParseIP(u.Hostname())
	if u.Scheme == "http" && (u.Hostname() == "localhost" || (ip != nil && ip.IsLoopback())) {
		return nil
	}
	return errors.New("HTTPS is required except on loopback")
}
