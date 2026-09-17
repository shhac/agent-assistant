import type { ChatToolEvent } from "./api";

/**
 * Tool activity, weighted by what still needs watching. Running and failed
 * operations stay visible; finished ones collapse into a count so a long
 * successful turn does not bury the reply it produced.
 */
/**
 * Tool activity, weighted by what still needs watching.
 *
 * Anything unfinished stays visible wherever it sits. So does the newest step
 * while it is the most recent thing in the thread: collapsing it would hide
 * what just happened at the moment the owner is watching for it. Once a reply
 * arrives, that step joins the others.
 *
 * A single remaining step is shown rather than hidden behind a summary that
 * would cost a row and a click to save one row.
 */
export function ToolActivity({
  events,
  live = false,
}: {
  events: ChatToolEvent[];
  /** This turn is the most recent thing in the thread and has not replied. */
  live?: boolean;
}) {
  const last = events[events.length - 1];
  const tail = live && last?.status === "completed" ? last : undefined;
  const settled = events.filter((e) => e.status === "completed" && e !== tail);
  const collapse = settled.length > 1;
  // One order-preserving pass: anything not folded away renders where it sits,
  // which is what "unfinished steps stay visible wherever they are" means.
  const folded = new Set(collapse ? settled : []);
  const visible = events.filter((e) => !folded.has(e));
  return (
    <div
      className="chat-tools"
      role="group"
      aria-label="Assistant tool activity"
    >
      {collapse && (
        <details className="chat-tools-settled">
          <summary>{settled.length} steps completed</summary>
          <ul>
            {settled.map((event) => (
              <ToolRow key={event.id} event={event} />
            ))}
          </ul>
        </details>
      )}
      {!!visible.length && (
        <ul>
          {visible.map((event) => (
            <ToolRow key={event.id} event={event} />
          ))}
        </ul>
      )}
    </div>
  );
}
const MARKER: Record<ChatToolEvent["status"], string> = {
  completed: "✓",
  failed: "!",
  interrupted: "!",
  running: "",
};

const PHRASE: Record<ChatToolEvent["status"], string> = {
  running: "In progress",
  completed: "Completed",
  interrupted: "Outcome unconfirmed",
  failed: "Failed",
};

function ToolRow({ event }: { event: ChatToolEvent }) {
  return (
    <li className={`chat-tool chat-tool-${event.status}`}>
      <span className="chat-tool-marker" aria-hidden="true">
        {MARKER[event.status]}
      </span>
      <span className="chat-tool-description">
        {event.label || "Using a tool"}
        <small>{PHRASE[event.status]}</small>
      </span>
      <details>
        <summary>Tool details</summary>
        <code>{event.tool}</code>
      </details>
    </li>
  );
}
