# Assistant-owned worker setup

Captured 2026-09-16. Version: code-internal implementation following v0.2.1.

The project page had exposed worker infrastructure before asking what the owner wanted to accomplish. This made the owner assemble a broker, choose technical identifiers, and explain the same context back to the assistant. The implementation moved setup behind one constrained coordination action and put the next intended outcome first.

As of this change, the PA could prepare a worker for a directory already linked to a project. Go selected the fixed installation commands, created a private broker, and saved its project scope. The model had no general host shell. Preparation did not create a run or authorize an invented objective; delegation remained a separate recorded action.

Local Codex and Claude CLIs were the normal inference transports. Assistant and worker settings remained independent, while all workers inherited the shared worker login by default. An optional complete per-worker model profile selected a different model or credential home. Credentials stayed under their CLI’s ownership and never entered worker containers. Claude used its native home; Codex retained the existing app-owned default to avoid global instruction contamination. Existing configured homes were preserved, and both default Codex profiles used the same home. No per-worker login ceremony was required. API endpoints remained an explicit advanced choice, with no fallback from CLI failure.

The dashboard discovered model names and effort choices through each installed CLI, without inference. A missing catalog preserved the saved choice and reported a problem rather than inventing supported models. CLI inference used a structured proposal protocol, disabled native execution tools, and verified the actual outbound capability surface against a local rejecting provider before a paid request.

The managed runtime used a local container service. macOS preparation could install free Homebrew tooling and start a dedicated Colima profile; Linux required an existing local Docker service. Preparation built a fixed Go/Node image without reading a project Dockerfile. A separate dependency step copied only validated manifests into a bounded network-enabled container, downloaded public Go/npm dependencies with scripts disabled, and retained no host credentials. Execution containers had no network; npm dependencies were copied per run so build caches could be written without changing shared caches.

This first implementation did not handle every project automatically. Private registries, local replacements, git packages, npm workspaces, install scripts, and other toolchains needed a deliberately prepared external environment. These were reported as blockers. Source files stayed unchanged: workers produced a patch and evidence for review. Applying a result to the original checkout remained outside this change.

Chat rendered Markdown safely, displayed the user message while inference was pending, and used Enter to submit with Shift+Enter for a newline. Errors retained a recoverable message without replaying actions. Project links used names; technical IDs stayed in explicit diagnostic details. The project page accepted a next outcome or asked the assistant to help choose one before commissioning work.

Verification used synthetic stores, fake CLIs, fake container commands, and local rejecting providers. No live user project, external integration, paid inference, or real runtime installation was used by the tests.
