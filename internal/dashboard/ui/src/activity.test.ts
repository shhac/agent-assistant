import { describe, expect, it } from "vitest";
import { activityLabel, groupActivity, isRoutineActivity } from "./activity";

describe("activity presentation", () => {
  it("describes recorded kinds without leaking internal identifiers", () => {
    expect(activityLabel("memory.created")).toBe("Remembered something new");
    expect(activityLabel("agent.missed_check_in")).toBe("Worker missed a check-in");
    expect(activityLabel("work_item.review_ready")).toBe("Outcome ready for review");
    expect(activityLabel("memory.corrected")).toBe("Memory corrected");
    expect(activityLabel(undefined)).toBe("Workspace");
  });

  it("keeps the dynamic worker status family readable", () => {
    expect(activityLabel("agent.blocked")).toBe("Worker blocked");
    expect(activityLabel("agent.some_future_state")).toBe("Worker some future state");
    expect(activityLabel("agent.some_future_state")).not.toContain(".");
  });

  it("collapses an adjacent run of routine updates and keeps the newest entry", () => {
    const groups = groupActivity([
      { id: "a4", kind: "agent.blocked", summary: "Worker stopped", project_id: "p1" },
      { id: "a3", kind: "agent.running", summary: "still running", project_id: "p1" },
      { id: "a2", kind: "agent.running", summary: "running", project_id: "p1" },
      { id: "a1", kind: "project.created", summary: "Project added", project_id: "p1" },
    ]);
    expect(groups).toHaveLength(3);
    expect(groups[0]).toMatchObject({ count: 1, label: "Worker blocked" });
    expect(groups[1]).toMatchObject({ count: 2, label: "Worker running" });
    expect(groups[1].entry.id).toBe("a3");
    expect(groups[2]).toMatchObject({ count: 1, label: "Project added" });
  });

  it("never collapses across projects or across meaningful state changes", () => {
    const groups = groupActivity([
      { id: "b2", kind: "agent.running", summary: "running", project_id: "p1" },
      { id: "b1", kind: "agent.running", summary: "running", project_id: "p2" },
    ]);
    expect(groups).toHaveLength(2);
    expect(isRoutineActivity("agent.blocked")).toBe(false);
    expect(isRoutineActivity("decision.opened")).toBe(false);
    expect(isRoutineActivity("agent.running")).toBe(true);
  });
});
