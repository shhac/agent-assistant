package server

import (
	"errors"
	"github.com/shhac/agent-assistant/internal/app"
	"github.com/shhac/agent-assistant/internal/core"
	"net/http"
)

func registerChatQueue(mux *http.ServeMux, a *app.App) {
	mux.HandleFunc("POST /api/chat/messages", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ID      string `json:"id"`
			Message string `json:"message"`
		}
		if decode(w, r, &in) != nil {
			return
		}
		turn, err := a.EnqueueChat(r.Context(), in.ID, in.Message)
		if err != nil {
			switch {
			case errors.Is(err, core.ErrChatValidation), errors.Is(err, core.ErrConflict), errors.Is(err, core.ErrChatQueueFull):
				problem(w, err)
			case errors.Is(err, app.ErrChatQueueUnavailable):
				fail(w, http.StatusServiceUnavailable, err.Error())
			default:
				fail(w, http.StatusInternalServerError, "Could not confirm whether the message was accepted. Retry using the same message ID.")
			}
			return
		}
		respond(w, http.StatusAccepted, turn)
	})
	mux.HandleFunc("GET /api/chat/turns", func(w http.ResponseWriter, r *http.Request) {
		turns, err := a.Core.ChatTurns(r.Context())
		if err != nil {
			fail(w, http.StatusInternalServerError, "Could not read the message queue.")
			return
		}
		hold, revision, err := a.ChatQueueState(r.Context())
		if err != nil {
			fail(w, http.StatusInternalServerError, "Could not read the message queue.")
			return
		}
		respond(w, http.StatusOK, struct {
			Turns    []core.ChatTurn `json:"turns"`
			Hold     *core.ChatHold  `json:"hold,omitempty"`
			Revision int             `json:"revision"`
		}{turns, hold, revision})
	})
	mux.HandleFunc("DELETE /api/chat/messages/{id}", func(w http.ResponseWriter, r *http.Request) {
		turn, err := a.CancelChat(r.Context(), r.PathValue("id"))
		if err != nil {
			if errors.Is(err, core.ErrNotFound) || errors.Is(err, core.ErrConflict) {
				problem(w, err)
			} else {
				fail(w, http.StatusInternalServerError, "Could not confirm cancellation. Refresh the queue before trying again.")
			}
			return
		}
		respond(w, http.StatusOK, turn)
	})
	// Holding names a queued message the owner is changing. It blocks that
	// message and everything after it, and lapses on its own so a browser that
	// disappears cannot strand the queue.
	mux.HandleFunc("POST /api/chat/messages/{id}/hold", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Reason string `json:"reason"`
		}
		if decode(w, r, &in) != nil {
			return
		}
		hold, err := a.Core.HoldChat(r.Context(), r.PathValue("id"), in.Reason, core.MaxChatHold)
		if err != nil {
			problem(w, err)
			return
		}
		respond(w, http.StatusOK, hold)
	})
	mux.HandleFunc("DELETE /api/chat/messages/{id}/hold", func(w http.ResponseWriter, r *http.Request) {
		if err := a.Core.ReleaseChatHold(r.Context(), r.PathValue("id")); err != nil {
			problem(w, err)
			return
		}
		respond(w, http.StatusOK, map[string]bool{"released": true})
	})
	// An edit carries the revision its editor was opened against, so an edit
	// that lost a race with the daemon starting the turn is refused.
	mux.HandleFunc("PATCH /api/chat/messages/{id}", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Message  string `json:"message"`
			Revision int    `json:"revision"`
		}
		if decode(w, r, &in) != nil {
			return
		}
		turn, err := a.Core.EditChatMessage(r.Context(), r.PathValue("id"), in.Message, in.Revision)
		if err != nil {
			problem(w, err)
			return
		}
		respond(w, http.StatusOK, turn)
	})
	// Reordering names the whole intended order and the revision it was decided
	// against, so a queue that changed underneath is refused rather than merged.
	mux.HandleFunc("PUT /api/chat/queue", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Order    []string `json:"order"`
			Revision int      `json:"revision"`
		}
		if decode(w, r, &in) != nil {
			return
		}
		turns, err := a.Core.ReorderChat(r.Context(), in.Order, in.Revision)
		if err != nil {
			problem(w, err)
			return
		}
		respond(w, http.StatusOK, struct {
			Turns []core.ChatTurn `json:"turns"`
		}{turns})
	})
}
