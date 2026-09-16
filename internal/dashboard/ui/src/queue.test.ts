import { describe, expect, it } from "vitest";
import { isQueued, orderQueue, startCondition } from "./queue";
import type { WorkItem } from "./api";

function item(partial: Partial<WorkItem> & { id: string }): WorkItem {
  return {
    project_id: "p1",
    title: partial.id,
    objective: "",
    acceptance_criteria: "",
    status: "queued",
    created_at: "2026-09-16T12:00:00Z",
    updated_at: "2026-09-16T12:00:00Z",
    review_revision: "rev",
    ...partial,
  };
}

describe("queue order", () => {
  // The dependency chain from the owner's review: suggestions, then the asset
  // work that waits on it, then the lifecycle work that waits on that.
  it("places each outcome after the one it waits on", () => {
    const ordered = orderQueue([
      item({ id: "lifecycle", after_work_item_id: "assets" }),
      item({ id: "assets", after_work_item_id: "suggestions" }),
      item({ id: "suggestions" }),
    ]);
    expect(ordered.map((w) => w.id)).toEqual([
      "suggestions",
      "assets",
      "lifecycle",
    ]);
  });

  it("leads with outcomes whose predecessor is not itself queued", () => {
    const ordered = orderQueue([
      item({ id: "second", after_work_item_id: "already-accepted" }),
      item({ id: "third", after_work_item_id: "second" }),
    ]);
    expect(ordered.map((w) => w.id)).toEqual(["second", "third"]);
  });

  it("never drops work it cannot sequence", () => {
    const cyclic = [
      item({ id: "a", after_work_item_id: "b" }),
      item({ id: "b", after_work_item_id: "a" }),
    ];
    expect(orderQueue(cyclic)).toHaveLength(2);
    expect(orderQueue([])).toEqual([]);

    // A self-referencing item can never satisfy the placement rule; it must
    // still be listed, in the order it was recorded.
    const selfReferencing = [
      item({ id: "first" }),
      item({ id: "loop", after_work_item_id: "loop" }),
    ];
    const ordered = orderQueue(selfReferencing);
    expect(ordered.map((w) => w.id)).toEqual(["first", "loop"]);
  });

  it("states the start condition and whether it may start automatically", () => {
    expect(
      startCondition(
        item({ id: "assets", after_work_item_id: "s", commission_requested: true }),
        "Suggestions",
      ),
    ).toBe(
      "Waits for Suggestions to be accepted · automatic coordination authorized",
    );
    expect(
      startCondition(item({ id: "assets", after_work_item_id: "s" }), "Suggestions"),
    ).toBe(
      "Waits for Suggestions to be accepted · not authorized to start automatically",
    );
    expect(startCondition(item({ id: "solo" }), undefined)).toBe(
      "Recorded only; arrange a start with your assistant",
    );
  });

  it("treats an outcome with attempts as work in progress, not a queue entry", () => {
    expect(isQueued(item({ id: "a" }), false)).toBe(true);
    expect(isQueued(item({ id: "a" }), true)).toBe(false);
    expect(isQueued(item({ id: "a", status: "active" }), false)).toBe(false);
  });
});
