// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { WorkerFailureCard } from "./WorkerFailureCard";
import type { Agent } from "./api";

const base: Agent = {
  id: "agent-1",
  name: "Suggestions worker",
  role: "worker",
  status: "blocked",
  project_id: "project-1",
  summary:
    "The attempt stopped without a classified provider error. Inspect the preserved work and correct the problem before explicitly resuming; no automatic retry is scheduled.",
  evidence: ["Patch: /runs/1/artifacts/changes.patch"],
  last_update: "2026-09-16T16:53:00Z",
  recoveries: 1,
};

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("worker failure card", () => {
  it("says which evidence was missing instead of guessing a cause", () => {
    render(
      <WorkerFailureCard
        agent={{
          ...base,
          provider_failure_kind: "unknown",
          model_failure_engine: "claude",
          model_failure_evidence: "untyped_error",
        }}
      />,
    );
    expect(
      screen.getByText(/did not arrive as a provider error envelope/),
    ).toBeTruthy();
    expect(screen.getByText(/underlying cause is not established/)).toBeTruthy();
    expect(
      screen.getByText(/Do not assume a login, model or compaction problem/),
    ).toBeTruthy();
    expect(screen.queryByText(/log ?in failed/i)).toBeNull();
  });

  it("distinguishes an unrecognised provider classification from a missing envelope", () => {
    render(
      <WorkerFailureCard
        agent={{
          ...base,
          provider_failure_kind: "unknown",
          model_failure_evidence: "unclassified_kind",
        }}
      />,
    );
    expect(
      screen.getByText(/classification is not one this daemon recognises/),
    ).toBeTruthy();
  });

  it("does not claim evidence for an attempt recorded before it was captured", () => {
    render(
      <WorkerFailureCard
        agent={{ ...base, provider_failure_kind: "unknown" }}
      />,
    );
    expect(
      screen.getByText(/recorded before failure evidence was captured/),
    ).toBeTruthy();
  });

  it("names who acts next and keeps technical detail behind one disclosure", () => {
    render(
      <WorkerFailureCard
        agent={{
          ...base,
          provider_failure_kind: "structured_output_limit",
          model_failure_code: "error_max_structured_output_retries",
          model_failure_evidence: "typed_envelope",
          model_exit_code: 1,
        }}
      />,
    );
    expect(screen.getByText("You")).toBeTruthy();
    const technical = screen.getByText("Technical details");
    expect(technical.tagName).toBe("SUMMARY");
    expect(
      (technical.parentElement as HTMLDetailsElement).open,
    ).toBe(false);
    expect(
      screen.getByText("error_max_structured_output_retries"),
    ).toBeTruthy();
  });

  it("performs no request and no control action when it renders", () => {
    const fetch = vi.fn();
    vi.stubGlobal("fetch", fetch);
    render(
      <WorkerFailureCard
        agent={{ ...base, provider_failure_kind: "unknown" }}
        onInvestigate={vi.fn()}
      />,
    );
    expect(fetch).not.toHaveBeenCalled();
  });

  it("hands investigation to the conversation rather than resuming work", () => {
    const investigate = vi.fn();
    const fetch = vi.fn();
    vi.stubGlobal("fetch", fetch);
    render(
      <WorkerFailureCard
        agent={{ ...base, provider_failure_kind: "unknown" }}
        onInvestigate={investigate}
      />,
    );
    fireEvent.click(
      screen.getByRole("button", { name: "Ask assistant to investigate" }),
    );
    expect(investigate).toHaveBeenCalledTimes(1);
    expect(investigate.mock.calls[0][0]).toContain("inspect the preserved");
    expect(fetch).not.toHaveBeenCalled();
  });

  it("reports a scheduled retry as the worker's turn, not the owner's", () => {
    render(
      <WorkerFailureCard
        agent={{
          ...base,
          status: "retry_wait",
          provider_failure_kind: "overloaded",
          model_failure_evidence: "typed_envelope",
          retry_at: "2026-09-16T17:05:00Z",
        }}
      />,
    );
    expect(screen.getByText("The worker")).toBeTruthy();
    expect(screen.getByText(/A retry is scheduled/)).toBeTruthy();
  });

  it("does not present a daemon limit as a provider diagnosis", () => {
    render(<WorkerFailureCard agent={{ ...base, status: "interrupted" }} />);
    expect(screen.getByText("Execution was interrupted")).toBeTruthy();
    expect(
      screen.getByText(/not a model provider failure, so no provider diagnosis exists/),
    ).toBeTruthy();
  });

  it("renders nothing for a worker that has not stopped", () => {
    const { container } = render(
      <WorkerFailureCard agent={{ ...base, status: "running" }} />,
    );
    expect(container.innerHTML).toBe("");
  });

  // The daemon's own preflight measurement is not something the provider said,
  // and this is the branch carrying the "do not assume compaction" warning.
  it("distinguishes the daemon's own measurement from a provider report", () => {
    render(
      <WorkerFailureCard
        agent={{
          ...base,
          provider_failure_kind: "context_limit",
          model_failure_evidence: "local_preflight",
          model_failure_phase: "preflight",
          model_failure_code: "working_context_budget",
        }}
      />,
    );
    expect(
      screen.getByText(/measured by the daemon before the request was sent/),
    ).toBeTruthy();
    expect(
      screen.getByText(/working context reached its budget/),
    ).toBeTruthy();
  });

  it("accounts for a worker whose state is still being checked", () => {
    render(<WorkerFailureCard agent={{ ...base, status: "reconciling" }} />);
    expect(
      screen.getByText("The worker's state is being checked before any retry"),
    ).toBeTruthy();
    expect(screen.getByText("Your assistant")).toBeTruthy();
    expect(
      screen.getByText(/Silence alone does not confirm a blocker/),
    ).toBeTruthy();
  });

  // An evidence value this build does not recognise must not produce a card
  // that states neither what is known nor what is not.
  it("still says what is unestablished for an unrecognised evidence value", () => {
    render(
      <WorkerFailureCard
        agent={{
          ...base,
          provider_failure_kind: "unknown",
          model_failure_evidence: "some_future_value",
        }}
      />,
    );
    const card = screen.getByLabelText("What stopped Suggestions worker");
    expect(card.textContent).toContain("What is not known");
    expect(
      screen.getByText(/underlying cause is not established/),
    ).toBeTruthy();
    expect(
      screen.getByText(/a form this dashboard does not recognise/),
    ).toBeTruthy();
  });
});
