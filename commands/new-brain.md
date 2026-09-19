---
description: Create a new empty named Zekra brain (optionally with a scoped token).
argument-hint: "<name> [description...]"
---

Create a new brain using the Zekra **brain_create** MCP tool.

- Take the brain name from the first token of `$ARGUMENTS`; use the rest as its description.
- Call `brain_create` with `{ name, description }`. It seeds a genesis marker so the namespace exists and is immediately connectable.
- Report the new brain and how to connect it: another session pastes an MCP config with `ZEKRA_DEFAULT_NAMESPACE=<name>`, or runs `zekra mcp:install <client> --brain <name>`.

If the user wants a teammate to have access to *only* this brain, tell them to mint a scoped token from the shell: `zekra brain create <name> --token` (mints a non-admin token granted read+write on just that brain and prints a paste-ready snippet).
