package diagnostics

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shhac/lib-agent-harness/completion"
)

func TestFailuresAreStructuredAndDoNotCopySensitiveErrorText(t *testing.T) {
	var buf bytes.Buffer
	logger := New(&buf)
	secret := "secret-token-and-private-prompt"
	logger.Failure(Event{Component: "worker", Stage: "context_preparation", RunID: "run-1"}, fmt.Errorf("%s: %w", secret, &os.PathError{Op: "open", Path: secret, Err: os.ErrPermission}))
	var event Event
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &event); err != nil {
		t.Fatal(err)
	}
	if event.Code != "permission_denied" || event.RunID != "run-1" || event.Time.IsZero() || len(event.ErrorTypes) != 3 || event.FixableBy == "retry" {
		t.Fatalf("unexpected event: %+v", event)
	}
	if strings.Contains(buf.String(), secret) || strings.Contains(buf.String(), `"Path"`) {
		t.Fatalf("sensitive data leaked: %s", &buf)
	}
}

func TestTypedFailureKeepsProviderFactsAndRetryDisposition(t *testing.T) {
	for _, scheduled := range []bool{false, true} {
		var buf bytes.Buffer
		at := time.Now().Add(time.Minute)
		event := Event{Component: "worker", Stage: "model_completion"}
		if scheduled {
			event.RetryAt = &at
		}
		code := 1
		New(&buf).Failure(event, &completion.RequestError{Kind: completion.ErrorOverloaded, Engine: "claude", Phase: completion.PhaseResponse, Code: "overloaded", ExitCode: &code})
		var got Event
		if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &got); err != nil {
			t.Fatal(err)
		}
		if got.Engine != "claude" || got.Phase != "response" || got.Code != "overloaded" || got.ExitCode == nil || *got.ExitCode != 1 || (got.FixableBy == "retry") != scheduled {
			t.Fatalf("unexpected event: %+v", got)
		}
	}
}

func TestConcurrentFailuresRemainSingleJSONRecords(t *testing.T) {
	var buf bytes.Buffer
	logger := New(&buf)
	var wg sync.WaitGroup
	for range 30 {
		wg.Go(func() { logger.Failure(Event{Component: "worker", Stage: "test"}, errors.New("private payload")) })
	}
	wg.Wait()
	lines := bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n"))
	if len(lines) != 30 {
		t.Fatalf("got %d lines", len(lines))
	}
	for _, line := range lines {
		if !json.Valid(line) {
			t.Fatalf("invalid line: %s", line)
		}
	}
}
