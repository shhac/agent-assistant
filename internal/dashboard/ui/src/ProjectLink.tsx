import type { Project } from "./api";

export function ProjectLink({ project }: { project: Project }) {
  return (
    <a
      className="project-name-link"
      href={`#/projects/${encodeURIComponent(project.id)}`}
    >
      {project.title}
    </a>
  );
}
