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
		respond(w, http.StatusOK, struct {
			Turns []core.ChatTurn `json:"turns"`
		}{turns})
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
}
