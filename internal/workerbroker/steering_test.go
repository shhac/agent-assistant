package workerbroker

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/shhac/agent-assistant/internal/integrations/worker"
)

func steeringCall(arguments string) toolCall {
	call := toolCall{ID: "fixture-steering", Type: "function"}
	call.Function.Name = "acknowledge_steering"
	call.Function.Arguments = arguments
	return call
}

func TestSteeringReceiptsSurviveRestartAndDeduplicate(t *testing.T) {
	b, cfg := newFixture(t, "https://provider.test/v1", &fakeDocker{})
	response := request(t, b, "/runs", "dispatch-one", startRequest())
	var run worker.Run
	if err := json.Unmarshal(response.Body.Bytes(), &run); err != nil {
		t.Fatal(err)
	}
	if err := b.update(run.ID, func(r *storedRun) error { r.Run.Status = "running"; return nil }); err != nil {
		t.Fatal(err)
	}
	for _, args := range []string{
		`{"message_ids":["first","first"]}`,
		`{"message_ids":["first","second"]}`,
	} {
		_, finished, err := b.tool(context.Background(), run.ID, storedRun{}, steeringCall(args))
		if err != nil || finished {
			t.Fatal(finished, err)
		}
	}
	// Completing the run must not discard receipts that the daemon has not
	// observed yet. They are transported with the final report after restart.
	b.finalize(run.ID, "completed", "Synthetic outcome", []string{"synthetic evidence"})
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	got, err := restarted.snapshot(run.ID)
	if err != nil || !reflect.DeepEqual(got.Run.SteeringAcknowledgements, []string{"first", "second"}) {
		t.Fatal(got.Run, err)
	}
	if got.Run.Status != "completed" {
		t.Fatal("receipt changed execution status", got.Run)
	}
}

func TestSteeringReceiptsRejectInvalidOrExcessClaims(t *testing.T) {
	b, _ := newFixture(t, "https://provider.test/v1", &fakeDocker{})
	defer b.Close()
	response := request(t, b, "/runs", "dispatch-one", startRequest())
	var run worker.Run
	json.Unmarshal(response.Body.Bytes(), &run)
	b.update(run.ID, func(r *storedRun) error { r.Run.Status = "running"; return nil })
	for _, args := range []string{
		`{}`, `{"message_ids":[]}`, `{"message_ids":[""]}`,
		`{"message_ids":[" wrong "]}`,
		`{"message_ids":["one"],"agent_id":"another-agent"}`,
	} {
		if _, _, err := b.tool(context.Background(), run.ID, storedRun{}, steeringCall(args)); err == nil {
			t.Fatalf("accepted invalid receipt: %s", args)
		}
	}
	ids := make([]string, 200)
	for i := range ids {
		ids[i] = fmt.Sprintf("steering-%d", i)
	}
	args, _ := json.Marshal(map[string]any{"message_ids": ids})
	if _, _, err := b.tool(context.Background(), run.ID, storedRun{}, steeringCall(string(args))); err != nil {
		t.Fatal(err)
	}
	if _, _, err := b.tool(context.Background(), run.ID, storedRun{}, steeringCall(`{"message_ids":["overflow"]}`)); err == nil {
		t.Fatal("unbounded cumulative receipts")
	}
	got, _ := b.snapshot(run.ID)
	if !reflect.DeepEqual(got.Run.SteeringAcknowledgements, ids) {
		t.Fatal("rejected receipt changed persisted acknowledgements")
	}
	b.finalize(run.ID, "cancelled", "Stopped", nil)
	if _, _, err := b.tool(context.Background(), run.ID, storedRun{}, steeringCall(`{"message_ids":["late"]}`)); err == nil {
		t.Fatal("cancelled execution created a receipt")
	}
}
