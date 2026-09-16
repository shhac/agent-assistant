# Worker visibility, owner controls and durable follow-up work

Date: 2026-09-16. As-of implementation following v0.6.0; code-internal except `lib-agent-harness` commit `090063ada468` and Claude Code 2.1.273.

The dashboard had described an outcome as in progress even when its only assignment had stopped during a CLI compatibility check. Outcome status was changed to derive from execution attempts, with explicit interrupted, blocked, waiting and paused states and a visible reason. A prepared profile remained distinct from a commissioned assignment.

The reported failure was reproduced without account inference. Claude made a bodyless discovery HEAD request and two valid model POST requests against the local rejecting probe. The harness had treated discovery as malformed inference and required exactly one model request. The fix handled discovery separately and bounded retries while validating every actual model request's tools, schema, instructions and effort. It retained the fail-closed boundary and added diagnostic reasons instead of a generic upgrade instruction. The installed CLI passed the corrected probe; a pre-request sentinel prevented real inference.

Owner controls became durable intents routed through the daemon. Pause waited for the current operation, withheld subsequent work, and became confirmed only after cleanup. Stop cancelled execution, also awaiting cleanup. Resume preserved the workspace and transcript. Requested and confirmed states differed deliberately: transport acknowledgement was not evidence that execution had stopped. Owner holds survived daemon and broker restarts and prevented automatic recovery. Capability discovery kept unsupported external-broker controls out of the UI.

Conversation history recorded application-visible communication, not private model reasoning or arbitrary native-tool payloads. Assignment text, directions, reports, questions and control events were paginated per worker. Intended delivery and confirmed broker acceptance were distinct; neither implied implementation. Entries were bounded and trimming was reported. No historical exchange was invented for older assignments.

Follow-up work gained an explicit predecessor and commissioning authorization. A queued outcome started only after the predecessor's exact reviewed revision was accepted, subject to existing capacity, usage and decision gates. An ordinary draft had no automatic-start authority. Withdrawal retained the draft while revoking commissioning, checked atomically when delegating. This moved commitments out of conversational memory without imposing a fixed manager hierarchy.

Tests used temporary state, fake models, runtimes and brokers. They exercised pause during model/tool operations, withheld later calls, cleanup uncertainty, explicit resume, persisted holds, conversation pagination and privacy, queue ordering and authorization withdrawal. No live project state or worker execution was changed while developing this feature.
