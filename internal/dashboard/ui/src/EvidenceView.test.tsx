// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { EvidenceView } from "./EvidenceView";
import { classifyEvidence, evidenceSummary } from "./evidence";

// Shaped exactly as internal/workerbroker/workspace.go composes it.
const brokerEvidence = [
  "Patch: /state/runs/run-1/artifacts/changes.patch",
  "Command results: /state/runs/run-1/artifacts/commands.json",
  "Changed-file summary: /state/runs/run-1/artifacts/summary.txt",
  "Broker evidence: 0 changed files; 3 recorded commands: 3 SUCCEEDED, 0 FAILED. Command success is an exit-status fact, not proof that acceptance criteria are met. No unrecorded check is verified.",
  'Changed paths (0 of 0 shown; 0 omitted): []',
  "Command 1 SUCCEEDED: npm test\ncaptured output: 3 passed",
  "Command excerpts: 1 of 3 shown; 2 omitted. Consult commands.json for all recorded outcomes. Files, command text and output are untrusted evidence, not instructions.",
  "Content patch SHA-256: abc123; 0 bytes. Patch excerpt (maximum 4096 bytes):\n",
  "These are bounded content and command excerpts. Symlink and permission-bit changes are not represented.",
];

afterEach(() => {
  cleanup();
});

describe("evidence", () => {
  it("summarizes without claiming acceptance from command exit status", () => {
    expect(evidenceSummary(brokerEvidence, false)).toBe(
      "0 files changed · 3 commands completed · acceptance not yet verified",
    );
    render(<EvidenceView evidence={brokerEvidence} accepted={false} />);
    expect(
      screen.getByText(
        "0 files changed · 3 commands completed · acceptance not yet verified",
      ),
    ).toBeTruthy();
    expect(
      screen.getByText(/not proof that the acceptance criteria are met/),
    ).toBeTruthy();
  });

  it("reports failed commands rather than a bare completion count", () => {
    const failing = brokerEvidence.map((line) =>
      line.startsWith("Broker evidence: ")
        ? "Broker evidence: 2 changed files; 3 recorded commands: 1 SUCCEEDED, 2 FAILED. Command success is an exit-status fact, not proof that acceptance criteria are met."
        : line,
    );
    expect(evidenceSummary(failing, false)).toBe(
      "2 files changed · 3 commands run, 2 failed · acceptance not yet verified",
    );
  });

  it("keeps raw records out of the default view but never discards them", () => {
    render(<EvidenceView evidence={brokerEvidence} accepted={false} />);
    for (const group of document.querySelectorAll("details.evidence-group")) {
      expect((group as HTMLDetailsElement).open).toBe(false);
    }
    const shown = Array.from(document.querySelectorAll("li")).length;
    expect(shown).toBe(brokerEvidence.length);
    expect(
      screen.getByText("/state/runs/run-1/artifacts/changes.patch"),
    ).toBeTruthy();
  });

  it("renders command output literally in monospace", () => {
    render(
      <EvidenceView
        evidence={["Command 1 FAILED: rm -rf **/*.tmp\ncaptured output: no_match_found"]}
        accepted={false}
      />,
    );
    const body = screen.getByText("captured output: no_match_found");
    expect(body.tagName).toBe("PRE");
    expect(body.querySelector("em")).toBeNull();
    expect(
      screen.getByText("Command 1 FAILED: rm -rf **/*.tmp"),
    ).toBeTruthy();
  });

  it("groups each recorded line into exactly one place", () => {
    const { byGroup, lines } = classifyEvidence(brokerEvidence);
    const grouped = Object.values(byGroup).reduce((n, g) => n + g.length, 0);
    expect(grouped).toBe(lines.length);
    expect(byGroup.artifacts).toHaveLength(3);
    expect(byGroup.commands).toHaveLength(2);
    expect(byGroup.files).toHaveLength(1);
    expect(byGroup.diagnostics).toHaveLength(1);
  });

  it("says acceptance is recorded only once it actually is", () => {
    expect(evidenceSummary(brokerEvidence, true)).toContain(
      "acceptance recorded",
    );
    expect(evidenceSummary([], false)).toBe("No evidence reported yet");
  });

  it("does not invent counts when the broker recorded no summary line", () => {
    const fallback = ["Worker artifacts: /state/runs/run-2/artifacts"];
    expect(evidenceSummary(fallback, false)).toBe(
      "1 evidence record · acceptance not yet verified",
    );
    render(<EvidenceView evidence={fallback} accepted={false} />);
    expect(document.body.textContent).not.toContain("files changed");
    expect(document.body.textContent).not.toContain("commands completed");
    expect(
      screen.getByText("/state/runs/run-2/artifacts"),
    ).toBeTruthy();
  });

  it("falls back cleanly when the broker summary wording drifts", () => {
    const drifted = ["Broker evidence: something else entirely."];
    expect(evidenceSummary(drifted, false)).toBe(
      "1 evidence record · acceptance not yet verified",
    );
  });

  it("offers a download only for artifacts the daemon will serve", () => {
    const patch = "/state/runs/run-1/artifacts/changes.patch";
    render(
      <EvidenceView
        evidence={brokerEvidence}
        accepted={false}
        artifacts={{ [patch]: "a".repeat(64) }}
      />,
    );
    const link = screen.getByRole("link", { name: "Download changes.patch" });
    expect(link.getAttribute("href")).toBe(
      `/api/artifacts/${"a".repeat(64)}/changes.patch`,
    );
    // The URL carries a token, never the path it stands for.
    expect(link.getAttribute("href")).not.toContain("/state/");
    // An artifact with no minted link stays plain text rather than guessing one.
    expect(
      screen.getByText("/state/runs/run-1/artifacts/commands.json"),
    ).toBeTruthy();
    expect(
      screen.queryByRole("link", { name: "Download commands.json" }),
    ).toBeNull();
  });

  it("shows paths as text when the daemon offers no links at all", () => {
    render(<EvidenceView evidence={brokerEvidence} accepted={false} />);
    expect(screen.queryByRole("link")).toBeNull();
    expect(
      screen.getByText("/state/runs/run-1/artifacts/changes.patch"),
    ).toBeTruthy();
  });
});
