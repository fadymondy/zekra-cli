---
description: Diagnose the Zekra setup — CLI installed, on PATH, logged in, MCP reachable.
---

Diagnose the Zekra plugin setup and fix what's broken. Check, in order:

1. **CLI present** — run `command -v zekra` and `zekra version`. If missing, tell the user to install it: `npm i -g zekra-cli` (or `curl -fsSL https://app.zekra.dev/install.sh | sh`). The plugin's MCP server *is* the `zekra` binary, so it must be on PATH.
2. **Up to date** — compare `zekra version` against the latest; if behind, suggest `curl -fsSL https://app.zekra.dev/upgrade.sh | sh`.
3. **Authenticated** — run `zekra auth whoami`. If no token/endpoint, the plugin's `userConfig` (api_token) may be unset — tell them to reconfigure the plugin or run `zekra auth login --token <cbt_…>`.
4. **MCP live** — call the **brain_list** tool. If it returns brains, the end-to-end path (client → zekra mcp → API) works. If it fails with `unauthorized`, the token is missing or wrong.

Report each check as ✓/✗ with the exact fix command for any failure.
