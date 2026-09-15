# Existing projects and host filesystem selection — 2026-09-15

As of the implementation after v0.1.0. The dashboard used React 19.3.0, Headless Tree 1.7.0 and TanStack Virtual 3.14.13.

Project intake had required an acceptance contract even when the owner only wanted to register an existing directory. Intake was changed to permit tracking first, with directory references persisted in the project record. Commissioning retained its contract gate. The assistant received those references through its existing project context; registering a path did not give the assistant a shell, source-content access, or implementation authority.

Each project received private scratch space beneath the selected state directory. Model invocation scratch also moved beneath state and remained transient. Source directories were canonical references, never a location for assistant state. Worker brokers kept their independently configured workspace enforcement.

The browser could not return a server filesystem path from a native file-upload picker. A reusable picker therefore used an authenticated server endpoint for one-level directory metadata. The component supported directory, file and mixed selection, including multiple selections. It did not expose file contents, uploads, or write operations.

[React Arborist](https://github.com/jameskerr/react-arborist) offered virtualization, keyboard navigation and selection in a complete tree. [Headless Tree's asynchronous data loader](https://headless-tree.lukasbach.com/features/async-dataloader/) more directly matched directory expansion backed by requests, while its flat visible-item list composed with [TanStack Virtual](https://headless-tree.lukasbach.com/recipe/virtualization/). The latter combination was selected for lazy expansion, existing keyboard behavior, custom workspace styling, and rendering only visible rows. No claim of being universally fastest was made; the choice followed this application's asynchronous data shape and was checked with fixture-driven interaction tests.

The server paged at 250 rows. Short-lived snapshots kept pagination stable across filesystem changes and were bounded by entry, byte, age and count limits. Concurrent scans were limited, links resolved canonically, non-directory paths could not be opened as blocking pipes, and errors remained visible for retry. No recursive indexing occurred. Demo mode refused host filesystem browsing.

Slack account discovery was corrected to unpack the CLI's `workspaces` envelope and use aliases with `--workspace`. Missing-credential metadata became a safe generated hint, never raw credential output. Notion gained an explicit default-account mode represented by an empty profile list. This mode used the CLI's native default authentication, did not require a named workspace, and never changed the CLI default. Named-profile selection remained mandatory for the other supported CLIs.
