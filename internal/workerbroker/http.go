package workerbroker

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/shhac/agent-assistant/internal/integrations/worker"
)

func (b *Broker) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /runs", b.start)
	mux.HandleFunc("GET /runs", func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Query().Get("dispatch_key")
		b.mu.Lock()
		defer b.mu.Unlock()
		for _, run := range b.state.Runs {
			if key != "" && run.Run.DispatchKey == key {
				respond(w, 200, run.Run)
				return
			}
		}
		respond(w, 404, map[string]string{"error": "run not found"})
	})
	mux.HandleFunc("GET /runs/{id}", func(w http.ResponseWriter, r *http.Request) {
		run, err := b.snapshot(r.PathValue("id"))
		if err != nil {
			respond(w, 404, map[string]string{"error": "run not found"})
			return
		}
		respond(w, 200, run.Run)
	})
	for _, action := range []string{"resume", "messages", "cancel"} {
		mux.HandleFunc("POST /runs/{id}/"+action, b.control)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		expected := b.cfg.AuthToken
		if expected == "" {
			expected = os.Getenv(b.cfg.TokenEnv)
		}
		got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if expected == "" || len(got) != len(expected) || subtle.ConstantTimeCompare([]byte(got), []byte(expected)) != 1 {
			respond(w, 401, map[string]string{"error": "broker authentication required"})
			return
		}
		mux.ServeHTTP(w, r)
	})
}
func respond(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func body(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 256*1024))
	if err != nil {
		respond(w, 400, map[string]string{"error": "request exceeds 256 KiB"})
		return nil, err
	}
	return raw, nil
}
func (b *Broker) start(w http.ResponseWriter, r *http.Request) {
	raw, err := body(w, r)
	if err != nil {
		return
	}
	var in worker.StartRequest
	if strict(raw, &in) != nil {
		respond(w, 400, map[string]string{"error": "invalid start request"})
		return
	}
	if in.ProjectID != b.cfg.ProjectID || in.Role != "worker" || in.AgentID == "" || in.DispatchKey == "" || strings.TrimSpace(in.Task) == "" || strings.TrimSpace(in.AcceptanceCriteria) == "" || len(in.Capabilities) == 0 || len(in.DelegationCapabilities) > 0 {
		respond(w, 403, map[string]string{"error": "this broker accepts only direct workers for its configured project"})
		return
	}
	for _, cap := range in.Capabilities {
		if cap != "implement" && cap != "review" {
			respond(w, 403, map[string]string{"error": "this broker permits implement and review capabilities only"})
			return
		}
	}
	if !contains(in.Prohibitions, "deployment") || !contains(in.Prohibitions, "production_data_access") || !contains(in.Prohibitions, "purchases") {
		respond(w, 403, map[string]string{"error": "immutable prohibitions are required"})
		return
	}
	key := r.Header.Get("Idempotency-Key")
	if key == "" || key != in.DispatchKey || len(key) > 256 {
		respond(w, 400, map[string]string{"error": "start requires the stable dispatch idempotency key"})
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	digest := digest(r, raw)
	if previous, ok := b.state.Receipts[key]; ok {
		if previous.Digest != digest {
			respond(w, 409, map[string]string{"error": "idempotency key was reused with different contents"})
			return
		}
		respond(w, 200, b.state.Runs[previous.RunID].Run)
		return
	}
	if len(b.state.Runs) >= 1000 {
		respond(w, 409, map[string]string{"error": "broker run retention limit reached; archive this state before commissioning more work"})
		return
	}
	id := uid()
	run := &storedRun{Run: worker.Run{ID: id, DispatchKey: in.DispatchKey, Status: "queued", Summary: "Accepted into the isolated worker queue", UpdatedAt: now(), Evidence: []string{}}, Request: in, Container: "agent-assistant-" + id, Messages: []string{}, Transcript: []modelMessage{}, Commands: []commandRecord{}}
	b.state.Runs[id] = run
	b.state.Receipts[key] = receipt{Digest: digest, RunID: id}
	if err = b.saveLocked(); err != nil {
		if !saveCommitted(err) {
			delete(b.state.Runs, id)
			delete(b.state.Receipts, key)
		}
		respond(w, 500, map[string]string{"error": "could not persist worker start"})
		return
	}
	b.signal()
	respond(w, 201, run.Run)
}
func (b *Broker) control(w http.ResponseWriter, r *http.Request) {
	raw, err := body(w, r)
	if err != nil {
		return
	}
	key := r.Header.Get("Idempotency-Key")
	if key == "" || len(key) > 256 {
		respond(w, 400, map[string]string{"error": "stable operation idempotency key required"})
		return
	}
	action := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
	message := ""
	switch action {
	case "resume":
		var in struct {
			Instruction string `json:"instruction"`
		}
		if strict(raw, &in) != nil {
			respond(w, 400, map[string]string{"error": "invalid resume instruction"})
			return
		}
		message = in.Instruction
	case "messages":
		var in struct {
			Message string `json:"message"`
		}
		if strict(raw, &in) != nil || strings.TrimSpace(in.Message) == "" {
			respond(w, 400, map[string]string{"error": "nonempty message required"})
			return
		}
		message = in.Message
	case "cancel":
		if strict(raw, &struct{}{}) != nil {
			respond(w, 400, map[string]string{"error": "invalid cancellation body"})
			return
		}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	run := b.state.Runs[r.PathValue("id")]
	if run == nil {
		respond(w, 404, map[string]string{"error": "run not found"})
		return
	}
	digest := digest(r, raw)
	if old, ok := b.state.Receipts[key]; ok {
		if old.Digest != digest {
			respond(w, 409, map[string]string{"error": "idempotency key reused with different contents"})
			return
		}
		respond(w, 200, b.state.Runs[old.RunID].Run)
		return
	}
	if action != "cancel" && run.PendingStatus == "cancelled" {
		respond(w, 409, map[string]string{"error": "cancellation is pending; wait for confirmed cleanup"})
		return
	}
	if action != "cancel" && (run.Run.Status == "completed" || run.Run.Status == "cancelled") {
		respond(w, 409, map[string]string{"error": "terminal worker cannot resume"})
		return
	}
	if action == "messages" && (run.Run.Status == "interrupted" || (run.Run.Status == "blocked" && run.Run.Decision == nil)) {
		respond(w, 409, map[string]string{"error": "interrupted workers require explicit resume"})
		return
	}
	if action == "resume" && run.Run.Status != "interrupted" && run.Run.Status != "blocked" {
		respond(w, 409, map[string]string{"error": "resume requires interrupted or blocked worker"})
		return
	}
	before, _ := json.Marshal(run)
	if action == "cancel" {
		if run.Run.Status == "running" {
			run.PendingStatus = "cancelled"
			run.PendingSummary = "Cancelled by the owner after confirmed container cleanup"
			run.Run.Status = "running"
			run.Run.Summary = "Cancellation requested; waiting for confirmed container cleanup"
		} else if run.Run.Status != "completed" && run.Run.Status != "cancelled" {
			run.Run.Status = "cancelled"
			run.Run.Summary = "Cancelled before further execution"
			run.PendingStatus = ""
			run.PendingSummary = ""
		}
	} else {
		run.Messages = append(run.Messages, message)
		if run.Run.Status == "blocked" || run.Run.Status == "interrupted" {
			run.Run.Status = "queued"
			run.Run.Decision = nil
			run.Run.Summary = "Existing isolated workspace queued to continue"
		}
	}
	run.Run.UpdatedAt = now()
	b.state.Receipts[key] = receipt{Digest: digest, RunID: run.Run.ID}
	if err = b.saveLocked(); err != nil {
		if !saveCommitted(err) {
			var restored storedRun
			_ = json.Unmarshal(before, &restored)
			*run = restored
			delete(b.state.Receipts, key)
		}
		respond(w, 500, map[string]string{"error": "could not persist worker operation"})
		return
	}
	if action == "cancel" {
		if cancel := b.active[run.Run.ID]; cancel != nil {
			cancel()
		}
	} else {
		b.signal()
	}
	respond(w, 200, run.Run)
}
func digest(r *http.Request, raw []byte) string {
	return fmt.Sprintf("%x", sha256.Sum256(append([]byte(r.Method+" "+r.URL.Path+"\n"), raw...)))
}
func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
