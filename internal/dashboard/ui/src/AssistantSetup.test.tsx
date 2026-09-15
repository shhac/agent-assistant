// @vitest-environment jsdom
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { AssistantSetup } from "./AssistantSetup";
import { ConnectionsSettings } from "./ConnectionsSettings";
import { useState } from "react";
import type { Connection } from "./ConnectionsSettings";

const recommendation = {
  id: "proposal-1",
  name: "Rowan",
  personality: "Calm, direct, and thoughtful.",
  theme: "graphite-sage",
  avatar: { shape: "leaf", background: "#202424", accent: "#aacbbb" },
  rationale: "A quiet style to match your preferences.",
};
let requests: { path: string; options?: RequestInit }[];
let response: (path: string) => unknown;
beforeEach(() => {
  requests = [];
  response = () => ({ messages: [], questions: [], recommendation });
  vi.stubGlobal(
    "fetch",
    vi.fn(async (path: string, options?: RequestInit) => {
      requests.push({ path, options });
      return { ok: true, status: 200, json: async () => response(path) };
    }),
  );
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});
it("keeps a proposed identity unchanged until the owner applies it", async () => {
  const applied = vi.fn(async () => {});
  response = (path) =>
    path.endsWith("/apply")
      ? recommendation
      : { messages: [], questions: [], recommendation };
  render(
    <AssistantSetup currentName="Iris" demo={false} onApplied={applied} />,
  );
  const button = await screen.findByRole("button", {
    name: "Use this identity",
  });
  expect(applied).not.toHaveBeenCalled();
  expect(requests.some((r) => r.path.endsWith("/apply"))).toBe(false);
  fireEvent.click(button);
  await screen.findByRole("button", { name: "Applied" });
  expect(applied).toHaveBeenCalledTimes(1);
  expect(
    JSON.parse(
      requests.find((r) => r.path.endsWith("/apply"))!.options!.body as string,
    ),
  ).toEqual({ recommendation_id: "proposal-1", accepted: true });
});
it("restores a previously applied recommendation without offering to apply it again", async () => {
  response = () => ({
    messages: [],
    questions: [],
    recommendation: { ...recommendation, applied: true },
  });
  render(
    <AssistantSetup currentName="Rowan" demo={false} onApplied={vi.fn()} />,
  );
  expect(await screen.findByRole("button", { name: "Applied" })).toHaveProperty(
    "disabled",
    true,
  );
  expect(
    screen.queryByText("Nothing changes until you apply this suggestion."),
  ).toBeNull();
});
it("keeps named account bindings distinct and prevents unsupported Notion drafts", async () => {
  response = () => ({
    tool: "agent-slack",
    available: true,
    selectable: true,
    profiles: [{ name: "work" }, { name: "personal" }],
  });
  let latest: Connection[] = [];
  function Harness() {
    const [connections, setConnections] = useState<Connection[]>([
      {
        id: "work-slack",
        name: "Work conversations",
        tool: "agent-slack",
        profiles: ["work"],
      },
      {
        id: "personal-slack",
        name: "Personal conversations",
        tool: "agent-slack",
        profiles: ["personal"],
      },
    ]);
    latest = connections;
    return (
      <ConnectionsSettings
        connections={connections}
        onChange={setConnections}
      />
    );
  }
  render(<Harness />);
  await waitFor(() =>
    expect(screen.getAllByLabelText("work")[0]).toHaveProperty(
      "disabled",
      false,
    ),
  );
  expect(screen.getAllByLabelText("work")[0]).toHaveProperty("checked", true);
  expect(screen.getAllByLabelText("work")[1]).toHaveProperty("checked", false);
  fireEvent.click(screen.getAllByLabelText("work")[1]);
  expect(latest[0].profiles).toEqual(["work"]);
  expect(latest[1].profiles).toEqual(["personal", "work"]);
  expect(
    screen.getAllByRole("option", {
      name: "Notion (account selection unavailable)",
    })[0],
  ).toHaveProperty("disabled", true);
  expect(screen.getAllByLabelText("Connection name")[0]).toHaveProperty(
    "maxLength",
    80,
  );
});
