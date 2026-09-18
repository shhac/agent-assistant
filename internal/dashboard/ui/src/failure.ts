import type { Agent } from "./api";
import { actorLabel, isHeldUp, type Actor } from "./states";

/**
 * An owner-readable account of a stopped worker. Every field here reports what
 * was recorded. Where evidence is missing this says which evidence is missing,
 * and never substitutes a reconstructed cause such as a login, model or
 * compaction problem.
 */
export interface FailureExplanation {
  /** Short statement of what stopped. */
  headline: string;
  /** What the recorded evidence does and does not establish. */
  known: string;
  /** Present only when something is genuinely unestablished. */
  unknown: string;
  /** Whether recovery is scheduled, held, or waiting on the owner. */
  recovery: string;
  /** Who acts next, in the daemon's vocabulary. */
  owner: Actor;
  /** The suggested next step, phrased as a choice rather than an action taken. */
  nextStep: string;
}

const kindPhrases: Record<string, string> = {
  authentication: "The model provider rejected the worker's credentials",
  context_limit: "The worker's working context reached its budget",
  model_unavailable: "The selected model was unavailable",
  structured_output_limit:
    "The model did not return usable structured output within the allowed retries",
  permission_denied: "The model provider refused the request",
  timeout: "The model provider did not respond in time",
  overloaded: "The model provider reported it was overloaded",
  rate_limited: "The model provider rate-limited the request",
  unavailable: "The model provider was unavailable",
  unknown: "The attempt stopped without a classified provider error",
};

const evidencePhrases: Record<string, string> = {
  typed_envelope:
    "The provider returned a classified error, recorded in the technical details.",
  untyped_error:
    "The failure did not arrive as a provider error envelope, so no phase or provider code was available to record.",
  unclassified_kind:
    "The provider returned an error whose classification is not one this daemon recognises, so it was recorded as unknown rather than guessed at.",
  application_validation:
    "The daemon rejected a model response during local validation. The summary and diagnostic code describe what failed.",
  local_process:
    "The local CLI process failed. The technical details record the available process evidence; a provider rejection was not established.",
  local_preflight:
    "This was measured by the daemon before the request was sent, not reported by the provider.",
};

/**
 * What the recorded evidence establishes. An unrecognised value is reported as
 * unrecognised rather than as an empty account: this daemon and the dashboard
 * are versioned together but need not be deployed together.
 */
function evidencePhrase(evidence?: string): string {
  if (!evidence)
    return "This attempt was recorded before failure evidence was captured, so what the provider returned is not known.";
  return (
    evidencePhrases[evidence] ||
    "The failure was recorded in a form this dashboard does not recognise, so what it establishes is not known here."
  );
}

function failureKindPhrase(kind?: string): string {
  if (!kind) return "The attempt stopped";
  return kindPhrases[kind] || "The attempt stopped with a provider error";
}

export function explainFailure(agent: Agent): FailureExplanation | null {
  const modelFailure = !!agent.provider_failure_kind;
  if (!isHeldUp(agent.status)) return null;

  if (agent.status === "retry_wait") {
    return {
      headline: failureKindPhrase(agent.provider_failure_kind),
      known: evidencePhrase(agent.model_failure_evidence),
      unknown: "",
      recovery: agent.retry_at
        ? "A retry is scheduled. Saved work is preserved and completed tool calls are not repeated."
        : "A retry is scheduled.",
      owner: "worker",
      nextStep: "Nothing is needed from you unless the wait continues.",
    };
  }

  if (agent.status === "reconciling") {
    return {
      headline: "The worker's state is being checked before any retry",
      known:
        "A report was expected and has not arrived. Silence alone does not confirm a blocker.",
      unknown: "Whether the worker is still making progress is not established.",
      recovery: "The assistant is checking worker state. No retry has started.",
      owner: "assistant",
      nextStep: "Wait for the check, or ask your assistant to investigate now.",
    };
  }

  if (!modelFailure) {
    return {
      headline:
        agent.status === "interrupted"
          ? "Execution was interrupted"
          : "Execution was stopped by a daemon limit",
      known:
        "This was not a model provider failure, so no provider diagnosis exists. The recorded summary states the limit or interruption.",
      unknown: "",
      recovery: "No automatic recovery is scheduled.",
      owner: "owner",
      nextStep:
        "Read the summary and preserved evidence, then decide whether to resume or change the assignment.",
    };
  }

  const toolBoundary = {
    unexpected_native_tool: {
      headline: "The worker response failed the native-tool safety check",
      known: "The harness detected an unexpected native tool. This older diagnostic does not distinguish an advertised tool from an attempted call; it does not establish a login or provider outage.",
    },
    unexpected_native_tool_catalog: {
      headline: "Claude exposed tools outside the worker's allowed interface",
      known: "The CLI advertised a native tool during constrained completion. The harness rejected the response; check CLI tool isolation before resuming.",
    },
    unexpected_native_tool_call: {
      headline: "Claude attempted a tool outside the worker's allowed interface",
      known: "The harness could not verify that the CLI rejected the tool as unavailable. No application actions from this response were accepted; inspect the integration before resuming.",
    },
  }[agent.model_failure_code as "unexpected_native_tool" | "unexpected_native_tool_catalog" | "unexpected_native_tool_call"];
  const evidence = agent.model_failure_evidence;
  return {
    headline: toolBoundary?.headline ?? failureKindPhrase(agent.provider_failure_kind),
    known: toolBoundary?.known ?? evidencePhrase(evidence),
    unknown:
      agent.model_failure_code || evidence === "typed_envelope"
        ? ""
        : "The underlying cause is not established. Do not assume a login, model or compaction problem.",
    recovery:
      "Automatic retry is not scheduled. Progress is preserved and the session was not duplicated.",
    owner: "owner",
    nextStep:
      "Inspect the preserved work, or ask your assistant to investigate before resuming.",
  };
}

export function ownerLabel(owner: FailureExplanation["owner"]): string {
  return actorLabel(owner);
}
