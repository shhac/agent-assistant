# First release pipeline — 2026-09-15

As of the v0.1.0 preparation; code-internal changes had no separate dependency version to pin. The shared release workflow was inspected at Git blob `a32342e1e866909cc30e5fa8acdc995e9aa1cc0b` and referenced through its maintained `main` branch, matching the sibling CLI pattern.

Cobra already generated shell completion scripts. This change added semantic completions for configuration, engine profiles, and locally configured worker projects and models. Suggestions used only the command schema and local configuration. No account discovery, database access, daemon requests, or model invocation occurred during completion. Configured values containing control characters were excluded from the shell completion protocol.

The repository adopted the family tap's reusable Go release workflow. It packaged macOS and Linux arm64/amd64 binaries and a Windows amd64 binary, published checksums, and generated the Homebrew formula with Bash, Zsh, and Fish completion installation. License detection used the repository's existing PolyForm Perimeter license.

The release entry point accepted version tags only and ran the ordinary CI jobs first. The tap write credential lived exclusively in the `homebrew-tap` GitHub environment, whose deployment policy allowed only `v*` tags. Its name was `TAP_DEPLOY_KEY`; its value was never captured in project files or tool output. A private temporary directory held the generated SSH key only until upload, then both key files were deleted. Repository-level secrets and branch/PR jobs did not receive that credential.

Local release preparation required a clean tree, an unused version tag both locally and remotely, dashboard freshness, Go tests/races/vet, and frontend types/tests. Publishing remained an explicit owner action: tag and push after those checks. The shared workflow owned artifact packaging and formula updates.

The Windows release target exposed Unix-only engine process control. Platform-specific lifecycle helpers preserved Unix process groups and placed Windows engines in a kill-on-close Job Object before resuming them. Native Windows CI exercised normal startup and cancellation of a helper with a descendant; it invoked no real model. The built-in Docker worker broker remained unavailable on Windows.
