---
description: Show Zekra connection status — endpoint + which brains you can reach.
---

Report the Zekra connection.

Run `zekra auth whoami` via Bash to show the configured endpoint, token, and reachable brains. If the `zekra` CLI isn't found, tell the user to install it (`npm i -g zekra-cli`) and run `/zekra:doctor`.

Then call the **brain_list** MCP tool and confirm the same brains resolve through the live MCP connection — that verifies the token in the plugin config is valid, not just the CLI's saved config.
