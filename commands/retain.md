---
description: Store a memory into a Zekra brain (runs the write-decision + embeds).
argument-hint: "[brain] <content...>"
---

Use the Zekra **memory_retain** MCP tool to persist something durable.

- Parse `$ARGUMENTS`: an optional leading brain name → `namespace` (else the session default), the rest is the `content`.
- Distill the content to a clear, self-contained sentence or two before storing (the write-decision de-dupes; a crisp fact ranks and recalls better than a raw dump).
- Confirm what was stored (id + decision: add/update/noop) back to the user.

Retain when you produce something worth remembering: a decision and its rationale, a learned constraint or gotcha, a new fact about a person/venture/system, or an interface/contract detail.
