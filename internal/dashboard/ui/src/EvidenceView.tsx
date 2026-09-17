import {
  classifyEvidence,
  evidenceGroups,
  evidenceSummary,
  splitLiteral,
} from "./evidence";

function basename(path: string): string {
  const at = path.lastIndexOf("/");
  return at < 0 ? path : path.slice(at + 1);
}

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
  artifacts = {},
}: {
  evidence: string[];
  accepted: boolean;
  /** Paths the daemon will serve, by opaque token. */
  artifacts?: Record<string, string>;
}) {
  if (!evidence.length)
    return <p className="muted">No evidence reported yet.</p>;
  const classified = classifyEvidence(evidence);
  return (
    <div className="evidence">
      <p className="evidence-summary">{evidenceSummary(evidence, accepted)}</p>
      {!accepted && (
        <p className="field-hint">
          Command success is an exit-status fact, not proof that the acceptance
          criteria are met.
        </p>
      )}
      {evidenceGroups.map(({ group, label }) => {
        const lines = classified.byGroup[group];
        if (!lines.length) return null;
        return (
          <details className="evidence-group" key={group}>
            <summary>
              {label} · {lines.length}
            </summary>
            <ul>
              {lines.map((line, i) => {
                if (group === "artifacts") {
                  const { label, path } = artifactParts(line.text);
                  const token = artifacts[path];
                  return (
                    <li key={i} className="evidence-artifact">
                      <span>{label}</span>
                      {token ? (
                        <a
                          href={`/api/artifacts/${token}/${encodeURIComponent(basename(path))}`}
                          download={basename(path)}
                        >
                          Download {basename(path)}
                        </a>
                      ) : (
                        <code title={path}>{path || line.text}</code>
                      )}
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
