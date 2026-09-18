package workerbroker

import (
	"errors"
	"os"
	"strings"
	"testing"
)

// The daemon has two ways to say something to a worker: the prompt that opens a
// turn, and direction handed to one already running. Both can fail in the same
// three ways, and the difference between them is the whole point of recording a
// handover at all.
//
// "No acknowledgement" is not "never received". A harness that took the prompt,
// ran tools and then lost its process leaves exactly as little behind as one
// that never heard it. Repeating the first duplicates work; dropping the second
// loses the assignment. So a refusal this process saw happen before anything was
// sent is replayable, and anything else stops for a person.

// blockForOwner puts the run where an uncertain delivery leaves it: stopped,
// waiting for a person, and resumable.
func blockForOwner(t *testing.T, b *Broker, id string) {
	t.Helper()
	if err := b.update(id, func(run *storedRun) error {
		run.Run.Status = "blocked"
		run.Run.Summary = "The daemon could not establish whether this worker received its last direction"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func withDirection(t *testing.T, b *Broker, id string, direction ...string) {
	t.Helper()
	if err := b.update(id, func(run *storedRun) error {
		run.Messages = append(run.Messages, direction...)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestATurnPromptIsReplayedOnlyWhenItWasNeverSent(t *testing.T) {
	b, _ := newFixture(t, "https://model.test", &fakeDocker{})
	defer b.Close()
	id := runningRun(t, b)
	withDirection(t, b, id, "prefer the smaller change")

	first, err := b.nextTurnInput(id)
	if err != nil || !strings.Contains(first, "prefer the smaller change") {
		t.Fatalf("the opening turn did not carry the direction: %q %v", first, err)
	}
	taken, _ := b.snapshot(id)
	if taken.InFlight == nil || taken.InFlight.Delivery != deliveryUnsent || len(taken.Messages) != 0 {
		t.Fatalf("direction was dequeued before it was handed over: %+v", taken.InFlight)
	}
	// Refused before anything was sent, so the same words are owed again.
	again, err := b.nextTurnInput(id)
	if err != nil || again != first {
		t.Fatalf("a prompt that was never sent was not replayed: %q %v", again, err)
	}

	// Handed over, outcome unknown. Now it must not repeat itself.
	if err = b.deliveryAttempted(id); err != nil {
		t.Fatal(err)
	}
	b.deliveryUncertain(id)
	blockForOwner(t, b, id)
	if _, err = b.nextTurnInput(id); !errors.Is(err, errUncertainDelivery) {
		t.Fatalf("an unacknowledged prompt was replayed anyway: %v", err)
	}

	// The owner's explicit resume is what resolves it, and the worker is told
	// that it may already have this.
	if code := request(t, b, "/runs/"+id+"/resume", "resume-one", map[string]string{"instruction": "Continue"}).Code; code != 200 {
		t.Fatal("resume refused", code)
	}
	resolved, _ := b.snapshot(id)
	if resolved.InFlight != nil {
		t.Fatalf("the uncertain record survived an explicit resume: %+v", resolved.InFlight)
	}
	queued := strings.Join(resolved.Messages, "\n")
	if !strings.Contains(queued, "prefer the smaller change") {
		t.Fatalf("the direction was lost: %v", resolved.Messages)
	}
	if !strings.Contains(queued, "may already have reached you") {
		t.Errorf("the repeat was not marked as one: %v", resolved.Messages)
	}
}

// Direction handed to a running turn needs the same distinction. A steer whose
// acknowledgement was lost may well have been applied.
func TestUncertainSteeringStopsRatherThanRepeatingItself(t *testing.T) {
	b, _ := newFixture(t, "https://model.test", &fakeDocker{})
	defer b.Close()
	id := runningRun(t, b)
	withDirection(t, b, id, "stop and check the schema")

	pending := b.pendingSteer(id)
	if !strings.Contains(pending, "stop and check the schema") {
		t.Fatalf("queued direction was not offered to the running turn: %q", pending)
	}
	if second := b.pendingSteer(id); second != "" {
		t.Fatalf("the same direction was offered twice: %q", second)
	}
	if err := b.steerAttempted(id); err != nil {
		t.Fatal(err)
	}
	taken, _ := b.snapshot(id)
	if taken.Steering == nil || taken.Steering.Delivery != deliverySending {
		t.Fatalf("the handover was not recorded before it was sent: %+v", taken.Steering)
	}
	b.steerUncertain(id)
	blockForOwner(t, b, id)

	// Nothing replays it: not the next turn, and not a restart.
	if _, err := b.nextTurnInput(id); !errors.Is(err, errUncertainDelivery) {
		t.Fatalf("an unacknowledged steer was folded back into the next turn: %v", err)
	}
	restarted, _ := b.snapshot(id)
	reconcileDelivery(&restarted)
	if restarted.Steering == nil || restarted.Steering.Delivery != deliveryUnknown {
		t.Fatalf("restart lost the uncertainty: %+v", restarted.Steering)
	}
	if code := request(t, b, "/runs/"+id+"/resume", "resume-one", map[string]string{"instruction": "Continue"}).Code; code != 200 {
		t.Fatal("resume refused", code)
	}
	resolved, _ := b.snapshot(id)
	if resolved.Steering != nil {
		t.Fatalf("the uncertain steer survived an explicit resume: %+v", resolved.Steering)
	}
	if !strings.Contains(strings.Join(resolved.Messages, "\n"), "may already have reached you") {
		t.Errorf("the repeat was not marked as one: %v", resolved.Messages)
	}
}

// A steer that the session refused before sending anything is owed again, and
// the turn it was meant for carries on.
func TestARefusedSteerGoesBackToTheQueueWithoutAReceipt(t *testing.T) {
	b, _ := newFixture(t, "https://model.test", &fakeDocker{})
	defer b.Close()
	id := runningRun(t, b)
	withDirection(t, b, id, "stop and check the schema")
	_ = b.pendingSteer(id)
	b.steerUndelivered(id)
	returned, _ := b.snapshot(id)
	if returned.Steering != nil || len(returned.Messages) != 1 || !strings.Contains(returned.Messages[0], "stop and check the schema") {
		t.Fatalf("refused direction was lost: %+v %v", returned.Steering, returned.Messages)
	}
	if len(returned.Run.SteeringAcknowledgements) != 0 {
		t.Error("a refused steer produced a read receipt")
	}
}

// A restart during a handover cannot tell a delivered prompt from a lost one, so
// it says so rather than guessing either way.
func TestRestartTurnsAnUnfinishedHandoverIntoUncertainty(t *testing.T) {
	sending := &inFlightInput{Text: "do the thing", Delivery: deliverySending}
	steering := &inFlightInput{Text: "do the other thing", Delivery: deliverySending}
	run := storedRun{InFlight: sending, Steering: steering}
	reconcileDelivery(&run)
	if run.InFlight.Delivery != deliveryUnknown || run.Steering.Delivery != deliveryUnknown {
		t.Fatalf("an unfinished handover survived restart as settled: %+v %+v", run.InFlight, run.Steering)
	}
	// One that was never sent stays replayable.
	unsent := storedRun{InFlight: &inFlightInput{Text: "never sent", Delivery: deliveryUnsent}}
	reconcileDelivery(&unsent)
	if unsent.InFlight.Delivery != deliveryUnsent {
		t.Fatalf("a prompt that was never sent became uncertain: %+v", unsent.InFlight)
	}
}

// If the record of a handover cannot be written, the handover must not happen.
// A restart would read the prompt as never attempted and say it again, to a
// worker that has already acted on it.
func TestAHandoverThatCannotBeRecordedIsNotAttempted(t *testing.T) {
	b, _ := newFixture(t, "https://model.test", &fakeDocker{})
	defer b.Close()
	id := runningRun(t, b)
	withDirection(t, b, id, "prefer the smaller change")
	if _, err := b.nextTurnInput(id); err != nil {
		t.Fatal(err)
	}
	_ = b.pendingSteer(id)

	if err := os.Chmod(b.cfg.StateDir, 0500); err != nil {
		t.Fatal(err)
	}
	turnErr := b.deliveryAttempted(id)
	steerErr := b.steerAttempted(id)
	if err := os.Chmod(b.cfg.StateDir, 0700); err != nil {
		t.Fatal(err)
	}
	if turnErr == nil {
		t.Error("a turn handover that could not be recorded reported success")
	}
	if steerErr == nil {
		t.Error("a steer handover that could not be recorded reported success")
	}
}
