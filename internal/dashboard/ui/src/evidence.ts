/**
 * Evidence arrives as bounded prose lines composed by the broker. This groups
 * them for reading without rewriting or dropping any of them: the canonical
 * lines stay the payload that acceptance records, so what an owner accepts is
 * exactly what they were shown.
 *
 * The prefixes matched here are pinned by a Go test in internal/workerbroker so
 * the grouping breaks loudly rather than silently degrading to "other".
 */
export type EvidenceGroup =
  | "files"
  | "commands"
  | "artifacts"
  | "diagnostics"
  | "notes";

export interface EvidenceCounts {
  changedFiles: number | null;
  commandsTotal: number | null;
  commandsFailed: number | null;
}

export interface EvidenceLine {
  text: string;
  group: EvidenceGroup;
  /** Command and patch lines carry literal output after the first line. */
  literal: boolean;
}

export interface ClassifiedEvidence {
  lines: EvidenceLine[];
  counts: EvidenceCounts;
  byGroup: Record<EvidenceGroup, EvidenceLine[]>;
}

const artifactPrefixes = [
  "Patch: ",
  "Command results: ",
  "Changed-file summary: ",
  "Worker artifacts: ",
];

function groupOf(line: string): { group: EvidenceGroup; literal: boolean } {
  if (artifactPrefixes.some((p) => line.startsWith(p)))
    return { group: "artifacts", literal: false };
  if (line.startsWith("Changed paths ("))
    return { group: "files", literal: false };
  if (/^Command \d+ (SUCCEEDED|FAILED): /.test(line))
    return { group: "commands", literal: true };
  if (line.startsWith("Command excerpts: "))
    return { group: "commands", literal: false };
  if (line.startsWith("Content patch SHA-256: "))
    return { group: "diagnostics", literal: true };
  if (line.startsWith("Broker evidence: "))
    return { group: "notes", literal: false };
  return { group: "notes", literal: false };
}

/** Reads the counts the broker already computed; never recomputes them. */
export function evidenceCounts(lines: string[]): EvidenceCounts {
  const summary = lines.find((line) => line.startsWith("Broker evidence: "));
  const empty: EvidenceCounts = {
    changedFiles: null,
    commandsTotal: null,
    commandsFailed: null,
  };
  if (!summary) return empty;
  const match =
    /^Broker evidence: (\d+) changed files; (\d+) recorded commands: (\d+) SUCCEEDED, (\d+) FAILED\./.exec(
      summary,
    );
  if (!match) return empty;
  return {
    changedFiles: Number(match[1]),
    commandsTotal: Number(match[2]),
    commandsFailed: Number(match[4]),
  };
}

export function classifyEvidence(lines: string[]): ClassifiedEvidence {
  const classified: EvidenceLine[] = lines.map((text) => ({
    text,
    ...groupOf(text),
  }));
  const byGroup: Record<EvidenceGroup, EvidenceLine[]> = {
    files: [],
    commands: [],
    artifacts: [],
    diagnostics: [],
    notes: [],
  };
  for (const line of classified) byGroup[line.group].push(line);
  return { lines: classified, counts: evidenceCounts(lines), byGroup };
}

/**
 * The one-line summary. Acceptance is never asserted here: command exit status
 * is an exit-status fact, and only a recorded acceptance can say otherwise.
 */
export function evidenceSummary(
  lines: string[],
  accepted: boolean,
): string {
  if (!lines.length)
    return accepted ? "Accepted · no evidence recorded" : "No evidence reported yet";
  const { changedFiles, commandsTotal, commandsFailed } = evidenceCounts(lines);
  const parts: string[] = [];
  if (changedFiles !== null)
    parts.push(`${changedFiles} ${changedFiles === 1 ? "file" : "files"} changed`);
  if (commandsTotal !== null) {
    const failed = commandsFailed ?? 0;
    parts.push(
      failed
        ? `${commandsTotal} commands run, ${failed} failed`
        : `${commandsTotal} ${commandsTotal === 1 ? "command" : "commands"} completed`,
    );
  }
  if (!parts.length)
    parts.push(`${lines.length} evidence ${lines.length === 1 ? "record" : "records"}`);
  parts.push(accepted ? "acceptance recorded" : "acceptance not yet verified");
  return parts.join(" · ");
}

/** Splits a command or patch line into its heading and its literal body. */
export function splitLiteral(text: string): { heading: string; body: string } {
  const at = text.indexOf("\n");
  return at < 0
    ? { heading: text, body: "" }
    : { heading: text.slice(0, at), body: text.slice(at + 1) };
}

export const evidenceGroupLabels: Record<EvidenceGroup, string> = {
  files: "Changed files",
  commands: "Commands and results",
  artifacts: "Artifacts",
  diagnostics: "Diagnostic metadata",
  notes: "What this evidence does and does not establish",
};
