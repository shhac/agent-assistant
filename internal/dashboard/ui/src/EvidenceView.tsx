import {
  classifyEvidence,
  evidenceGroupLabels,
  evidenceSummary,
  splitLiteral,
  type EvidenceGroup,
} from "./evidence";

const order: EvidenceGroup[] = [
  "files",
  "commands",
  "artifacts",
  "diagnostics",
  "notes",
];

/** Recognises the artifact path at the end of an artifact evidence line. */
function artifactParts(text: string): { label: string; path: string } {
  const at = text.indexOf(": ");
  return at < 0
    ? { label: text, path: "" }
    : { label: text.slice(0, at), path: text.slice(at + 2) };
}

/**
 * Evidence, summarized first and inspectable after. The canonical lines are
 * never rewritten or dropped — grouping only decides what is shown by default,
 * so the audit trail stays intact and acceptance still records what was shown.
 */
export function EvidenceView({
  evidence,
  accepted,
}: {
  evidence: string[];
  accepted: boolean;
}) {
  const classified = classifyEvidence(evidence);
  if (!evidence.length)
    return <p className="muted">No evidence reported yet.</p>;
  return (
    <div className="evidence">
      <p className="evidence-summary">{evidenceSummary(evidence, accepted)}</p>
      {!accepted && (
        <p className="field-hint">
          Command success is an exit-status fact, not proof that the acceptance
          criteria are met.
        </p>
      )}
      {order.map((group) => {
        const lines = classified.byGroup[group];
        if (!lines.length) return null;
        return (
          <details className="evidence-group" key={group}>
            <summary>
              {evidenceGroupLabels[group]} · {lines.length}
            </summary>
            <ul>
              {lines.map((line, i) => {
                if (group === "artifacts") {
                  const { label, path } = artifactParts(line.text);
                  return (
                    <li key={i} className="evidence-artifact">
                      <span>{label}</span>
                      <code title={path}>{path || line.text}</code>
                    </li>
                  );
                }
                if (!line.literal) return <li key={i}>{line.text}</li>;
                const { heading, body } = splitLiteral(line.text);
                return (
                  <li key={i}>
                    <span className="evidence-heading">{heading}</span>
                    {body && <pre>{body}</pre>}
                  </li>
                );
              })}
            </ul>
          </details>
        );
      })}
    </div>
  );
}
