// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { ChatSettings } from "./ChatSettings";
import type { Config } from "./api";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});
const config = {
  model: { engine: "codex", codex_home: "/synthetic/login" },
  chat: {
    other: "preserved",
    loading_phrases: { enabled: true, model: "", effort: "low" },
  },
} as Config;
const catalog = {
  available: true,
  engine: "codex",
  detail: "Reported by test CLI",
  models: [
    {
      id: "gpt-5.6-luna",
      name: "Luna",
      default_effort: "low",
      efforts: [{ id: "low" }, { id: "high" }],
    },
    {
      id: "test-small",
      name: "Small test model",
      default_effort: "medium",
      efforts: [{ id: "medium" }],
    },
  ],
};
function mockCatalog(data: unknown = catalog) {
  const fetch = vi.fn().mockResolvedValue({ ok: true, json: async () => data });
  vi.stubGlobal("fetch", fetch);
  return fetch;
}
function openOptions() {
  const details = screen.getByText("Loading phrase model", {
    selector: "summary",
  }).parentElement!;
  details.setAttribute("open", "");
  fireEvent(details, new Event("toggle"));
}
it("defaults to shared CLI loading phrases without fetching an unused catalog", () => {
  const fetch = mockCatalog();
  const changed = vi.fn();
  render(<ChatSettings config={config} onChange={changed} />);
  expect((screen.getByRole("checkbox") as HTMLInputElement).checked).toBe(true);
  expect(
    screen.getByText(/Automatic model: gpt-5.6-luna, low effort/),
  ).toBeTruthy();
  expect(fetch).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("checkbox"));
  expect(changed).toHaveBeenCalledWith({
    ...config,
    chat: {
      other: "preserved",
      loading_phrases: { enabled: false, model: "", effort: "low" },
    },
  });
});
it("uses catalog models and adjusts effort only on an explicit model selection", async () => {
  const fetch = mockCatalog();
  const changed = vi.fn();
  render(<ChatSettings config={config} onChange={changed} />);
  openOptions();
  await screen.findByRole("option", { name: "Small test model" });
  expect(fetch).toHaveBeenCalledWith(
    "/api/models?profile=assistant",
    expect.anything(),
  );
  expect(changed).not.toHaveBeenCalled();
  fireEvent.change(screen.getByLabelText("Loading phrase model"), {
    target: { value: "test-small" },
  });
  expect(changed).toHaveBeenCalledWith({
    ...config,
    chat: {
      other: "preserved",
      loading_phrases: { enabled: true, model: "test-small", effort: "medium" },
    },
  });
});
it("preserves unknown selections when discovery is unavailable and can restore automatic", async () => {
  mockCatalog({
    ...catalog,
    available: false,
    models: [],
    detail: "Test login unavailable",
  });
  const saved = {
    ...config,
    chat: {
      loading_phrases: { enabled: true, model: "custom", effort: "max" },
    },
  };
  const changed = vi.fn();
  render(<ChatSettings config={saved} onChange={changed} />);
  openOptions();
  await screen.findByText("Test login unavailable");
  expect(
    (screen.getByLabelText("Loading phrase model") as HTMLSelectElement).value,
  ).toBe("custom");
  expect(
    (
      screen.getByLabelText(
        "Loading phrase reasoning effort",
      ) as HTMLSelectElement
    ).value,
  ).toBe("max");
  expect(changed).not.toHaveBeenCalled();
  fireEvent.change(screen.getByLabelText("Loading phrase model"), {
    target: { value: "" },
  });
  expect(changed).toHaveBeenCalledWith({
    ...saved,
    chat: { loading_phrases: { enabled: true, model: "", effort: "max" } },
  });
});
it("uses Haiku automatically with a shared Claude login", async () => {
  mockCatalog({
    ...catalog,
    engine: "claude",
    models: [
      {
        id: "haiku",
        name: "Haiku",
        default_effort: "low",
        efforts: [{ id: "low" }],
      },
    ],
  });
  render(
    <ChatSettings
      config={{ ...config, model: { engine: "claude" } }}
      onChange={() => {}}
    />,
  );
  expect(screen.getByText(/Automatic model: haiku, low effort/)).toBeTruthy();
  openOptions();
  await screen.findByRole("option", { name: "Haiku" });
  expect(screen.queryByLabelText(/home|API key|login/i)).toBeNull();
});
it("does not discover or configure models for API assistants", () => {
  const fetch = mockCatalog();
  render(
    <ChatSettings
      config={{ ...config, model: { engine: "openai-compatible" } }}
      onChange={() => {}}
    />,
  );
  expect(screen.getByText(/make no additional model requests/)).toBeTruthy();
  expect(screen.queryByText("Loading phrase model")).toBeNull();
  expect(fetch).not.toHaveBeenCalled();
});
it("does not use a stale saved engine catalog for an unsaved engine change", async () => {
  mockCatalog(catalog);
  render(
    <ChatSettings
      config={{ ...config, model: { engine: "claude" } }}
      onChange={() => {}}
    />,
  );
  openOptions();
  await screen.findByText(/Save your assistant engine change/);
  expect(screen.queryByRole("option", { name: "Luna" })).toBeNull();
});
