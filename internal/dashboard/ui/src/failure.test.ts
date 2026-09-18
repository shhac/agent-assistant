import { describe, expect, it } from "vitest";
import { explainFailure } from "./failure";
import type { Agent } from "./api";

describe("native-tool boundary diagnostics", () => {
  it.each([
    ["unexpected_native_tool", "native-tool safety check", "older diagnostic"],
    ["unexpected_native_tool_catalog", "exposed tools", "advertised a native tool"],
    ["unexpected_native_tool_call", "attempted a tool", "could not verify"],
  ])("explains %s without inventing a provider failure", (code, headline, known) => {
    const failure = explainFailure({
      status: "blocked", provider_failure_kind: "unknown",
      model_failure_code: code, model_failure_evidence: "typed_envelope",
    } as Agent);
    expect(failure?.headline).toContain(headline);
    expect(failure?.known).toContain(known);
    expect(failure?.known).not.toContain("provider returned a classified error");
    expect(failure?.recovery).toContain("not scheduled");
  });
});
