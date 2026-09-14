// Package linear reads explicitly scoped assignments. It never writes to Linear.
package linear

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"
)

type Config struct {
	Endpoint   string
	APIKeyEnv  string
	TeamIDs    []string
	HTTPClient *http.Client
	Timeout    time.Duration
}
type Client struct {
	cfg  Config
	http *http.Client
}
type Issue struct {
	ID          string `json:"id"`
	Identifier  string `json:"identifier"`
	Title       string `json:"title"`
	Description string `json:"description"`
	URL         string `json:"url"`
	UpdatedAt   string `json:"updatedAt"`
	Team        struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"team"`
	Project *struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		URL  string `json:"url"`
	} `json:"project"`
	State struct {
		Name string `json:"name"`
		Type string `json:"type"`
	} `json:"state"`
}
type Assignments struct {
	OwnerID   string    `json:"owner_id"`
	Issues    []Issue   `json:"issues"`
	FetchedAt time.Time `json:"fetched_at"`
}

func New(cfg Config) (*Client, error) {
	if cfg.Endpoint == "" {
		cfg.Endpoint = "https://api.linear.app/graphql"
	}
	u, err := url.Parse(cfg.Endpoint)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("invalid Linear endpoint")
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost" || u.Hostname() == "::1")) {
		return nil, errors.New("Linear requires HTTPS except on loopback")
	}
	if cfg.APIKeyEnv == "" {
		return nil, errors.New("Linear credential environment reference is required")
	}
	if len(cfg.TeamIDs) == 0 {
		return nil, errors.New("configure at least one Linear team ID to bound assignment discovery")
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.Timeout < 0 {
		return nil, errors.New("Linear timeout must be positive")
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: cfg.Timeout}
	}
	copyClient := *client
	copyClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Client{cfg: cfg, http: &copyClient}, nil
}

// Assigned discovers active issues assigned to the credential's viewer, within
// configured teams. UpdatedAt is freshness only; assignment time is not inferred.
func (c *Client) Assigned(ctx context.Context) (Assignments, error) {
	result := Assignments{Issues: []Issue{}, FetchedAt: time.Now().UTC()}
	var viewer struct {
		Viewer struct {
			ID string `json:"id"`
		} `json:"viewer"`
	}
	if err := c.query(ctx, "query AssistantViewer { viewer { id } }", nil, &viewer); err != nil {
		return result, err
	}
	if viewer.Viewer.ID == "" {
		return result, errors.New("Linear viewer identity was missing")
	}
	result.OwnerID = viewer.Viewer.ID
	after := any(nil)
	seenCursors := map[string]bool{}
	seenIDs := map[string]bool{}
	teamSet := map[string]bool{}
	for _, id := range c.cfg.TeamIDs {
		teamSet[id] = true
	}
	for page := 0; page < 100; page++ {
		var data struct {
			Issues struct {
				Nodes    []Issue `json:"nodes"`
				PageInfo struct {
					HasNextPage bool   `json:"hasNextPage"`
					EndCursor   string `json:"endCursor"`
				} `json:"pageInfo"`
			} `json:"issues"`
		}
		variables := map[string]any{"owner": result.OwnerID, "teams": c.cfg.TeamIDs, "after": after}
		if err := c.query(ctx, assignmentsQuery, variables, &data); err != nil {
			return result, err
		}
		for _, issue := range data.Issues.Nodes {
			if !teamSet[issue.Team.ID] {
				return result, errors.New("Linear returned an issue outside the configured team scope")
			}
			if issue.ID == "" {
				return result, errors.New("Linear returned an issue without an ID")
			}
			if !seenIDs[issue.ID] {
				result.Issues = append(result.Issues, issue)
				seenIDs[issue.ID] = true
			}
		}
		if !data.Issues.PageInfo.HasNextPage {
			return result, nil
		}
		cursor := data.Issues.PageInfo.EndCursor
		if cursor == "" || seenCursors[cursor] {
			return result, errors.New("Linear pagination did not advance")
		}
		seenCursors[cursor] = true
		after = cursor
	}
	return result, errors.New("Linear assignment discovery exceeded 100 pages; narrow the team scope")
}

const assignmentsQuery = `query AssistantAssignments($owner: ID!, $teams: [ID!]!, $after: String) {
 issues(first: 50, after: $after, filter: {assignee: {id: {eq: $owner}}, team: {id: {in: $teams}}, state: {type: {nin: ["completed", "canceled"]}}}) {
  nodes { id identifier title description url updatedAt team { id name } project { id name url } state { name type } }
  pageInfo { hasNextPage endCursor }
 }
}`

func (c *Client) query(ctx context.Context, query string, variables map[string]any, out any) error {
	key := os.Getenv(c.cfg.APIKeyEnv)
	if key == "" {
		return fmt.Errorf("Linear credential environment variable %s is not set", c.cfg.APIKeyEnv)
	}
	data, _ := json.Marshal(map[string]any{"query": query, "variables": variables})
	callCtx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(callCtx, http.MethodPost, c.cfg.Endpoint, bytes.NewReader(data))
	if err != nil {
		return errors.New("cannot create Linear request")
	}
	req.Header.Set("Authorization", key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("Linear request failed or timed out")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Linear returned HTTP %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024+1))
	if err != nil || len(raw) > 4*1024*1024 {
		return errors.New("Linear response could not be read or exceeded limit")
	}
	var envelope struct {
		Data   json.RawMessage   `json:"data"`
		Errors []json.RawMessage `json:"errors"`
	}
	if json.Unmarshal(raw, &envelope) != nil {
		return errors.New("Linear returned malformed JSON")
	}
	if len(envelope.Errors) > 0 {
		return errors.New("Linear GraphQL query failed; check token permissions and team scope")
	}
	if len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return errors.New("Linear returned no data")
	}
	if json.Unmarshal(envelope.Data, out) != nil {
		return errors.New("Linear returned invalid data")
	}
	return nil
}
