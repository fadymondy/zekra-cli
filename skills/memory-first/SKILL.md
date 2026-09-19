---
name: memory-first
description: The recall→answer/act→retain discipline for working with a Zekra brain. Use whenever a task touches durable knowledge — questions about people/projects/decisions, researching before acting, or capturing something learned. Makes the brain the source of truth instead of guessing.
---

# Memory-first

You have a Zekra brain available through MCP tools. Treat it as your source of truth. The loop, every turn: **recall → answer/act → retain.**

## 1. Recall BEFORE you answer or act
For any question or task touching durable knowledge (a person, project, venture, issue, decision, learning, or a "who/what/why"), call **`memory_recall`** first — even if you think you already know. Before writing/planning/drafting on a topic, recall its context so you build on what's known.

- Query style: concise and keyword-forward (`"Sentra"`, `"PDPL kit"`, `"auth gate learning"`) — these rank cleaner than full sentences.
- If the first query is thin, try another phrasing, or recall a second brain and merge. Use **`brain_list`** if you're unsure which brain holds it.

## 2. Answer FROM what recall returns, and cite it
Base the answer on the recalled memories and point to the ones you used. If recall returns nothing relevant, **say so plainly** ("the brain has no memory of X") — do not invent facts to fill the gap. A truthful "not in the brain" is more valuable than a confident guess.

## 3. Retain what's new
After you produce something durable — a decision and its rationale ("chose X over Y because Z"), a correction, a learned constraint or gotcha, a new fact about a person/system, or an interface/contract detail — call **`memory_retain`** so the brain grows.

- Distill first: store a crisp, self-contained sentence or two. The write-decision de-dupes automatically, so a clean fact recalls far better than a raw dump.
- When unsure whether something is worth keeping, retain a short distilled line rather than nothing.

## 4. Prefer the brain over asking
If information is likely already in the brain, recall it instead of asking the user to repeat it. Only ask for what the brain genuinely lacks — then retain what you learn.

## Namespaces
Pick the one brain that matches the question; don't mix scopes in a single query. Respect a session's default brain when one is configured (`ZEKRA_DEFAULT_NAMESPACE`). Recall more than one brain only when it's genuinely ambiguous, then merge.

**Order of operations, always: recall → answer/act → retain.**
