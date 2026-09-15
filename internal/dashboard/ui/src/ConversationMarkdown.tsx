import Markdown from "react-markdown";
import remarkGfm from "remark-gfm";
import type { Project } from "./api";

/** No raw HTML or remote image loads; react-markdown retains its safe URL transform. */
export function ConversationMarkdown({
  content,
  projects = [],
  onProjectOpen,
}: {
  content: string;
  projects?: Project[];
  onProjectOpen?: (id: string) => void;
}) {
  return (
    <div className="conversation-markdown">
      <Markdown
        skipHtml
        remarkPlugins={[remarkGfm]}
        components={{
          a({ href, children }) {
            const project = projects.find(
              (p) =>
                href === `#/projects/${p.id}` ||
                href === `#/projects/${encodeURIComponent(p.id)}`,
            );
            return (
              <a
                href={href}
                rel="noopener noreferrer"
                target={href?.startsWith("http") ? "_blank" : undefined}
                onClick={
                  project && onProjectOpen
                    ? (e) => {
                        e.preventDefault();
                        onProjectOpen(project.id);
                      }
                    : undefined
                }
              >
                {project ? project.title : children}
              </a>
            );
          },
          img({ alt }) {
            return (
              <span className="message-image-description">
                {alt ? `[Image: ${alt}]` : "[Image]"}
              </span>
            );
          },
        }}
      >
        {content}
      </Markdown>
    </div>
  );
}
