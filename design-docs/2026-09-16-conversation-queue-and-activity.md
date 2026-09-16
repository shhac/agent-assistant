# Durable conversation queue and tool activity

Captured 2026-09-16. Version: code-internal implementation following commit 6b98aa6.

The dashboard previously held one pending HTTP request and disabled further submissions. It showed a general loading animation but could not distinguish model work from worker preparation or other tools. This change treated accepted owner messages as durable turns with client-generated idempotency keys.

As of this implementation, the daemon processed a bounded FIFO queue independently of the browser connection. A message entered the model’s conversation only when its turn started. The owner could cancel a queued message; an active action was not represented as cancelled just because the browser disconnected. After daemon interruption, the active turn became interrupted without automatic replay, while unstarted messages survived. Completion and its assistant reply were recorded together.

Tool start and finish events belonged to the turn. Their UI payload contained a daemon-owned label, tool name, status and timestamps, with no raw arguments, outputs or credential values. Polling made long-running preparation visible before its result returned. Events were separate from conversational text and were not interpreted as instructions.

Personalized loading captions used one optional small model invocation through the same local CLI account as the assistant. Codex selected Luna at low effort by default; Claude selected Haiku and used low effort only when supported. The caption model saw at most the latest two dialogue messages, each bounded, and had no tools. Its request counted toward the shared daily limit, with one slot left for substantive work. A short timeout and turn cancellation bounded optional work; errors preserved the static fallback. Captions were decorative and did not assert tool progress.

Tests used synthetic state, fake providers and injected caption completion. No test needed a real CLI login, paid inference, project worker, or external integration.
