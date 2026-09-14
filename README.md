# agent-assistant

A personal AI assistant that remembers context, coordinates project agents, follows up on stalled work, and brings its owner clear decisions. Go daemon and CLI, with a dark dashboard and optional private Tailscale access.

The assistant coordinates implementation by approved workers. It does not write project code, deploy, access production data, or buy things. Model inference and approved agent execution are operating usage within configured limits.

Implementation is in progress. See [design docs](design-docs/README.md) and the [current direction](design-docs/2026-09-14-implementation-direction.md).

## License

[PolyForm Perimeter 1.0.0](LICENSE), matching the agent CLI family.
