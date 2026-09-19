---
description: Recall memories from a Zekra brain (hybrid vector + BM25 + rerank).
argument-hint: "[brain] <query...>"
---

Use the Zekra **memory_recall** MCP tool to search a brain and answer from what it returns.

- Parse `$ARGUMENTS`: if the first token is a known brain name (e.g. `avo`, `flowos`, `cabrain`, `think-os`), use it as `namespace`; otherwise use the session's default brain and treat the whole input as the query.
- Call `memory_recall` with a concise, keyword-forward `query` (that ranks best).
- Answer the user **from the recalled memories** and cite them briefly. If recall returns nothing, say so plainly — do not invent facts.

If you're unsure which brain, call **brain_list** first, or recall the two most likely brains and merge.
