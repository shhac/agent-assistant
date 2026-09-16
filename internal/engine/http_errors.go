package engine

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/shhac/lib-agent-harness/completion"
)

// Only bounded structured codes and status metadata classify failures. Remote
// messages, request bodies and credential-bearing errors never enter our errors.
func httpRequestError(resp *http.Response, now time.Time) error {
	const limit = 64 * 1024
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	kind := completion.ErrorUnknown
	retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"), now)
	var payload struct {
		Error struct {
			Type string `json:"type"`
			Code string `json:"code"`
		} `json:"error"`
	}
	valid := readErr == nil && len(body) <= limit && json.Unmarshal(body, &payload) == nil
	code, typ := strings.ToLower(payload.Error.Code), strings.ToLower(payload.Error.Type)
	is := func(values ...string) bool {
		for _, value := range values {
			if code == value || typ == value {
				return true
			}
		}
		return false
	}
	switch {
	case is("insufficient_quota", "billing_hard_limit_reached", "quota_exceeded", "usage_limit_exceeded", "billing_error"):
		kind = completion.ErrorUnknown
	case resp.StatusCode == 401 || resp.StatusCode == 403 || is("authentication_error", "invalid_api_key", "permission_error"):
		kind = completion.ErrorAuthentication
	case is("context_length_exceeded", "context_window_exceeded", "request_too_large"):
		kind = completion.ErrorContextLimit
	case resp.StatusCode == 429 && (retryAfter > 0 || (valid && is("rate_limit_error", "rate_limit_exceeded", "rate_limited", "rate_limit"))):
		kind = completion.ErrorRateLimited
	case resp.StatusCode == 503:
		kind = completion.ErrorUnavailable
	case resp.StatusCode == 529:
		kind = completion.ErrorOverloaded
	}
	// An unreadable/truncated response is ambiguous even when its status appears
	// transient. A complete empty 503/529 response is an explicit rejection.
	if readErr != nil || len(body) > limit || (len(strings.TrimSpace(string(body))) > 0 && !valid) {
		kind = completion.ErrorUnknown
	}
	return fmt.Errorf("model returned HTTP %d: %w", resp.StatusCode, &completion.RequestError{Kind: kind, RetryAfter: retryAfter})
}
func parseRetryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if seconds, err := strconv.ParseInt(value, 10, 32); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	if at, err := http.ParseTime(value); err == nil && at.After(now) {
		return at.Sub(now)
	}
	return 0
}
