package server

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/shhac/agent-assistant/internal/app"
	"github.com/shhac/agent-assistant/internal/config"
	"github.com/shhac/agent-assistant/internal/core"
	"github.com/shhac/agent-assistant/internal/dashboard"
)

func New(a *app.App, auth *Auth) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/state", func(w http.ResponseWriter, r *http.Request) {
		s, err := a.Snapshot(r.Context())
		if err != nil {
			problem(w, err)
			return
		}
		respond(w, 200, struct {
			core.Snapshot
			Demo bool `json:"demo"`
		}{s, a.Demo})
	})
	mux.HandleFunc("POST /api/operations/{id}/acknowledge", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Note string `json:"note"`
		}
		if decode(w, r, &in) != nil {
			return
		}
		if err := a.Core.AcknowledgeEvent(r.Context(), r.PathValue("id"), in.Note); err != nil {
			problem(w, err)
			return
		}
		respond(w, 200, map[string]bool{"acknowledged": true})
	})
	mux.HandleFunc("GET /api/config", func(w http.ResponseWriter, r *http.Request) { respond(w, 200, a.Config()) })
	mux.HandleFunc("PUT /api/config", func(w http.ResponseWriter, r *http.Request) {
		var c config.Config
		if decode(w, r, &c) != nil {
			return
		}
		if err := a.UpdateConfig(c); err != nil {
			problem(w, err)
			return
		}
		respond(w, 200, c)
	})
	mux.HandleFunc("GET /api/connection-profiles", func(w http.ResponseWriter, r *http.Request) {
		result, err := a.DiscoverConnectionProfiles(r.Context(), r.URL.Query().Get("tool"))
		if err != nil {
			problem(w, err)
			return
		}
		respond(w, 200, result)
	})
	mux.HandleFunc("GET /api/setup", func(w http.ResponseWriter, r *http.Request) {
		state, err := a.IdentitySetup()
		if err != nil {
			problem(w, err)
			return
		}
		respond(w, 200, state)
	})
	mux.HandleFunc("POST /api/setup/interview", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Message string `json:"message"`
		}
		if decode(w, r, &in) != nil {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
		defer cancel()
		state, err := a.InterviewIdentity(ctx, in.Message)
		if err != nil {
			problem(w, err)
			return
		}
		respond(w, 200, state)
	})
	mux.HandleFunc("POST /api/setup/apply", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			RecommendationID string `json:"recommendation_id"`
			Accepted         bool   `json:"accepted"`
		}
		if decode(w, r, &in) != nil {
			return
		}
		identity, err := a.ApplyIdentity(r.Context(), in.RecommendationID, in.Accepted)
		if err != nil {
			problem(w, err)
			return
		}
		respond(w, 200, identity)
	})

	mux.HandleFunc("POST /api/chat", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Message string `json:"message"`
		}
		if decode(w, r, &in) != nil {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
		defer cancel()
		v, err := a.Chat(ctx, in.Message)
		if err != nil {
			problem(w, err)
			return
		}
		respond(w, 200, v)
	})
	mux.HandleFunc("POST /api/projects", func(w http.ResponseWriter, r *http.Request) {
		var in core.ProjectInput
		if decode(w, r, &in) != nil {
			return
		}
		v, err := a.Core.CreateProject(r.Context(), in)
		if err != nil {
			problem(w, err)
			return
		}
		respond(w, 201, v)
	})
	mux.HandleFunc("POST /api/projects/{id}/coordinate", func(w http.ResponseWriter, r *http.Request) {
		s, err := a.Snapshot(r.Context())
		if err != nil {
			problem(w, err)
			return
		}
		for _, p := range s.Projects {
			if p.ID == r.PathValue("id") {
				v, err := a.Chat(r.Context(), "Please coordinate project "+p.ID+" ("+p.Title+") to meet its recorded acceptance criteria. Delegate through approved profiles and keep me informed of unresolved decisions.")
				if err != nil {
					problem(w, err)
					return
				}
				respond(w, 200, v)
				return
			}
		}
		problem(w, core.ErrNotFound)
	})
	mux.HandleFunc("POST /api/decisions/{id}/resolve", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Choice string `json:"choice"`
		}
		if decode(w, r, &in) != nil {
			return
		}
		v, err := a.Core.ResolveDecision(r.Context(), r.PathValue("id"), in.Choice)
		if err != nil {
			problem(w, err)
			return
		}
		respond(w, 200, v)
	})
	mux.HandleFunc("POST /api/memories", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Content string `json:"content"`
			Key     string `json:"key,omitempty"`
		}
		if decode(w, r, &in) != nil {
			return
		}
		if in.Key == "" {
			in.Key = in.Content
		}
		v, err := a.Core.Remember(r.Context(), in.Key, in.Content)
		if err != nil {
			problem(w, err)
			return
		}
		respond(w, 201, v)
	})
	mux.HandleFunc("DELETE /api/memories/{id}", func(w http.ResponseWriter, r *http.Request) {
		if err := a.Core.Forget(r.Context(), r.PathValue("id")); err != nil {
			problem(w, err)
			return
		}
		respond(w, 200, map[string]bool{"deleted": true})
	})
	mux.HandleFunc("POST /api/control", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Paused bool `json:"paused"`
		}
		if decode(w, r, &in) != nil {
			return
		}
		if err := a.Core.SetPaused(r.Context(), in.Paused); err != nil {
			problem(w, err)
			return
		}
		respond(w, 200, in)
	})
	mux.HandleFunc("POST /api/sync", func(w http.ResponseWriter, r *http.Request) {
		if err := a.SyncLinear(r.Context()); err != nil {
			problem(w, err)
			return
		}
		respond(w, 200, map[string]bool{"synced": true})
	})
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) { fail(w, 404, "unknown API endpoint") })
	assets, err := fs.Sub(dashboard.Assets, "assets")
	if err != nil {
		panic(err)
	}
	files := http.FileServer(http.FS(assets))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" && r.Method != "HEAD" {
			w.WriteHeader(405)
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		if _, err := fs.Stat(assets, path); err != nil {
			if strings.Contains(path, ".") {
				http.NotFound(w, r)
				return
			}
			r.URL.Path = "/"
		}
		files.ServeHTTP(w, r)
	})
	return auth.Middleware(mux)
}
func problem(w http.ResponseWriter, err error) {
	status := 400
	if errors.Is(err, core.ErrNotFound) {
		status = 404
	}
	if errors.Is(err, core.ErrConflict) {
		status = 409
	}
	fail(w, status, err.Error())
}
