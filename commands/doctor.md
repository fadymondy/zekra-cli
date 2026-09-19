---
description: Diagnose the CaBrain setup — CLI installed, on PATH, logged in, MCP reachable.
---

Diagnose the CaBrain plugin setup and fix what's broken. Check, in order:

1. **CLI present** — run `command -v cabrain` and `cabrain version`. If missing, tell the user to install it: `npm i -g cabrain-cli` (or `curl -fsSL https://cabrain-app.fadymondy.com/install.sh | sh`). The plugin's MCP server *is* the `cabrain` binary, so it must be on PATH.
2. **Up to date** — compare `cabrain version` against the latest; if behind, suggest `curl -fsSL https://cabrain-app.fadymondy.com/upgrade.sh | sh`.
3. **Authenticated** — run `cabrain auth whoami`. If no token/endpoint, the plugin's `userConfig` (api_token) may be unset — tell them to reconfigure the plugin or run `cabrain auth login --token <cbt_…>`.
4. **MCP live** — call the **brain_list** tool. If it returns brains, the end-to-end path (client → cabrain mcp → API) works. If it fails with `unauthorized`, the token is missing or wrong.

Report each check as ✓/✗ with the exact fix command for any failure.
