package worker

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// RejectionError is a confirmed refusal before this operation took effect.
// It deliberately excludes transport errors, timeouts, server failures and
// idempotency conflicts whose original operation might already have happened.
type RejectionError struct{ StatusCode int }

func (e *RejectionError) Error() string {
	return fmt.Sprintf("worker refused this operation (HTTP %d); inspect its current state before choosing a lifecycle control", e.StatusCode)
}

func confirmedRejection(resp *http.Response) bool {
	if resp.StatusCode < 400 || resp.StatusCode >= 500 || resp.StatusCode == 408 {
		return false
	}
	if resp.StatusCode != 409 {
		return true
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8193))
	if err != nil || len(raw) > 8192 {
		return false
	}
	var body struct {
		Code  string `json:"code"`
		Error string `json:"error"`
	}
	if json.Unmarshal(raw, &body) != nil {
		return false
	}
	if body.Code == "operation_rejected" {
		return true
	}
	// Compatibility with existing local brokers; do not infer safety from an
	// arbitrary error string or a generic HTTP conflict from an external broker.
	switch body.Error {
	case "cancellation is pending; wait for confirmed cleanup", "terminal worker cannot resume", "interrupted workers require explicit resume", "resume requires paused, interrupted or blocked worker", "provider retry is not due":
		return true
	}
	return false
}
