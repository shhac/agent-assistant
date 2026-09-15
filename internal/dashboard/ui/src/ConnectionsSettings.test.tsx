// @vitest-environment jsdom
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { ConnectionsSettings, type Connection } from "./ConnectionsSettings";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});
it("enables Notion with its native default account and no profile selection", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({
        tool: "agent-notion",
        profiles: [],
        available: true,
        selectable: false,
        detail: "Uses the CLI default account",
      }),
    }),
  );
  const changed = vi.fn();
  render(
    <ConnectionsSettings
      connections={[
        { id: "docs", name: "Documents", tool: "agent-notion", profiles: [] },
      ]}
      onChange={changed}
    />,
  );
  await waitFor(() =>
    expect(screen.getByText("Uses the CLI default account")).toBeTruthy(),
  );
  expect(
    screen.getByRole("option", { name: "Notion" }).hasAttribute("disabled"),
  ).toBe(false);
  expect(screen.getByText("CLI default account")).toBeTruthy();
  expect(screen.queryAllByRole("checkbox")).toHaveLength(0);
  expect(screen.queryByText(/No profiles found/)).toBeNull();
});
it("shows discovered Slack aliases and emits the selected profile", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({
        tool: "agent-slack",
        profiles: [
          { name: "work", detail: "Stored credential unavailable" },
          { name: "personal" },
        ],
        available: true,
        selectable: true,
        detail: "CLI accounts",
      }),
    }),
  );
  const changed = vi.fn();
  render(
    <ConnectionsSettings
      connections={[
        { id: "slack", name: "Slack", tool: "agent-slack", profiles: [] },
      ]}
      onChange={changed}
    />,
  );
  await waitFor(() => expect(screen.getAllByRole("checkbox")).toHaveLength(2));
  fireEvent.click(screen.getByRole("checkbox", { name: /personal/ }));
  expect(changed).toHaveBeenCalledWith([
    { id: "slack", name: "Slack", tool: "agent-slack", profiles: ["personal"] },
  ]);
  expect(screen.getByText("Stored credential unavailable")).toBeTruthy();
});

it.each([null, undefined])(
  "renders Notion with profiles %s from JSON config",
  async (profiles) => {
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockResolvedValue({
          ok: true,
          json: async () => ({
            tool: "agent-notion",
            profiles: [],
            available: true,
            selectable: false,
            detail: "Native default account",
          }),
        }),
    );
    const connection = {
      id: "docs",
      name: "Documents",
      tool: "agent-notion",
      profiles,
    } as unknown as Connection;
    render(
      <ConnectionsSettings connections={[connection]} onChange={vi.fn()} />,
    );
    await waitFor(() =>
      expect(screen.getByText("Native default account")).toBeTruthy(),
    );
    expect(screen.getByText("CLI default account")).toBeTruthy();
    expect(screen.queryAllByRole("checkbox")).toHaveLength(0);
  },
);
