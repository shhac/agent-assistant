# Configured model homes — 2026-09-15

As of this implementation; code-internal, no external version change.

Runtime homes became typed assistant configuration rather than launch-time shell setup. `model.codex_home` and `worker_model.codex_home` defaulted to the same directory under the app namespace in XDG state. Explicit absolute paths supported existing dedicated logins and separate model accounts. Ambient `CODEX_HOME` no longer selected accounts for daemon or broker calls; it was supplied only to child processes from their configuration snapshot.

`agent-assistant model login [--profile assistant|worker]` ran Codex's login in the selected directory. The operator invoked this directly; it was not available as an assistant tool. The command created a private home but never read or copied credentials. Inference and doctor used the same selected home. Existing shell-based setups could retain their login by configuring its directory explicitly.

The separate home still served instruction isolation. Global AGENTS files remained rejected. Claude's `CLAUDE_CONFIG_DIR` was expected to follow the same pattern when a Claude driver was added; no unused Claude setting or placeholder engine was shipped.
