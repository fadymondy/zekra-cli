---
description: Wire the Zekra MCP into another client (Claude Desktop, Codex, Gemini, Cursor).
argument-hint: "[claude-desktop|claude-code|codex|gemini|cursor|print] [--brain NAME]"
---

Help the user connect another AI client to Zekra via the `zekra` CLI.

- If `$ARGUMENTS` names a client, run `zekra mcp:install $ARGUMENTS` via Bash (it merges the MCP config idempotently, preserving other servers).
- If no client is given, run `zekra mcp:print claude-desktop` to show the config snippet and list the supported clients: `claude-desktop`, `claude-code`, `codex`, `gemini`, `cursor`, `print`.
- Remind them to run `zekra auth login --token <cbt_…>` first if they haven't saved a token, and to restart the target client afterward.

For a brain-scoped setup, pass `--brain <name>` through to the CLI.
