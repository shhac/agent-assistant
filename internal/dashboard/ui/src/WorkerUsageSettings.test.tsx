// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { WorkerUsageSettings } from "./WorkerUsageSettings";
import type { Config } from "./api";

afterEach(cleanup);

it("preserves the other engine and capacity settings when disabling a gate", () => {
  const config = {
    assistant: { name: "Juniper" },
    limits: {
      max_agents: 4,
      worker_usage: {
        codex_max_used_percent: 90,
        claude_max_used_percent: 85,
        on_unavailable: "allow",
      },
    },
  } as Config;
  const changed = vi.fn();
  const { rerender } = render(
    <WorkerUsageSettings config={config} onChange={changed} />,
  );
  fireEvent.change(screen.getByLabelText("Codex usage limit (%)"), {
    target: { value: "0" },
  });
  const next = changed.mock.calls[0][0];
  expect(next).toEqual({
    ...config,
    limits: {
      max_agents: 4,
      worker_usage: {
        codex_max_used_percent: 0,
        claude_max_used_percent: 85,
        on_unavailable: "allow",
      },
    },
  });
  rerender(<WorkerUsageSettings config={next} onChange={changed} />);
  expect(
    (screen.getByLabelText("Codex usage limit (%)") as HTMLInputElement).value,
  ).toBe("0");
  fireEvent.change(screen.getByLabelText("When usage is unavailable"), {
    target: { value: "pause" },
  });
  expect(changed.mock.calls[1][0].limits.worker_usage).toEqual({
    codex_max_used_percent: 0,
    claude_max_used_percent: 85,
    on_unavailable: "pause",
  });
});
