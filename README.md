# zekra-cli

**Connect any AI client to the [Zekra](https://zekra.dev) memory system in one command.**

`zekra` is a single, dependency-free Go binary that is *both* the MCP server and its
installer. It speaks the [Model Context Protocol](https://modelcontextprotocol.io) over
stdio (a thin adapter over the Zekra REST API), and it wires that server into every
MCP host — Claude Desktop, Claude Code, Codex, Gemini CLI, Cursor — so a new user goes
from zero to a working brain in two commands.

```
┌────────────┐   spawns    ┌──────────────┐   HTTPS + X-Zekra-Token   ┌──────────────┐
│ MCP client │ ──────────▶ │ zekra mcp  │ ──────────────────────────▶ │ Zekra API  │
│ (Claude…)  │  (stdio)    │ (this binary)│                             │ brains + ACL │
└────────────┘             └──────────────┘                             └──────────────┘
```

## Install

```sh
curl -fsSL https://app.zekra.dev/install.sh | sh
```

or, with Go:

```sh
go install github.com/fadymondy/zekra-cli@latest   # installs `zekra`
```

or, with npm:

```sh
npm i -g zekra-cli
```

**Upgrade** to the latest anytime (npm / go / binary aware):

```sh
curl -fsSL https://app.zekra.dev/upgrade.sh | sh
```

## Use it as a Claude Code plugin

This repo is **also a Claude Code plugin** — it bundles the Zekra MCP, memory-first
slash commands, two agents, and a skill. Install:

```
/plugin marketplace add fadymondy/zekra-cli
/plugin install zekra@zekra
```

At enable time it prompts for your **Zekra token** (and optional endpoint / default
brain) and wires the MCP automatically — no `.mcp.json` editing. Requires the `zekra`
binary on PATH (any install method above), since the plugin's MCP server *is* `zekra mcp`.

**What the plugin adds**

| kind | name | what it does |
|---|---|---|
| command | `/zekra:recall [brain] <query>` | hybrid recall, answer from memory |
| command | `/zekra:retain [brain] <content>` | store a memory |
| command | `/zekra:brains [name]` | list brains / brain details |
| command | `/zekra:new-brain <name>` | create a new brain |
| command | `/zekra:connect [client]` | wire the MCP into another client |
| command | `/zekra:status` | endpoint + reachable brains |
| command | `/zekra:doctor` | diagnose CLI / auth / MCP |
| agent | `zekra-curator` | memory-first Q&A + retention (recall→answer→retain) |
| agent | `zekra-connector` | onboarding: install, tokens, brains, datasources |
| skill | `memory-first` | the recall→answer/act→retain discipline |
| hook | `SessionStart` | auto-injects the memory-first rules into every session |
| hook | `UserPromptSubmit` | opt-in auto-recall — pulls relevant memories for each prompt |

### Hooks — auto push/pull

The plugin ships two hooks (backed by `zekra hook …`, so no `jq`/`python` needed — the
binary parses the hook JSON itself, and every hook **fails open**: any error → no output,
never blocks you):

- **Memory-first rules (SessionStart)** — injects the recall→answer→retain discipline at
  the start of every session, so the model reaches for the brain automatically. **On by
  default;** turn off with the `inject_rules` plugin option or `ZEKRA_HOOK_RULES=0`.
- **Auto-recall / pull (UserPromptSubmit)** — uses each prompt to recall the top matching
  memories from your default brain and injects them as context. **Opt-in** (it hits the
  brain every prompt): enable the `auto_recall` plugin option or `ZEKRA_HOOK_AUTORECALL=1`,
  and set a default brain (`ZEKRA_AUTORECALL_BRAIN`, `ZEKRA_DEFAULT_NAMESPACE`, or the
  plugin's default-brain option). Needs a saved token (`zekra auth login`) or the plugin's
  `api_token`.

**Push** (writing to the brain) stays model-driven on purpose — retention should be
selective and distilled, not a firehose. The injected rules tell the model to `memory_retain`
what's durable; `/zekra:retain` and the `zekra-curator` agent do it explicitly.

Standalone (outside the plugin) you can wire the same hooks in your own `settings.json`:

```json
{
  "hooks": {
    "SessionStart":     [{ "hooks": [{ "type": "command", "command": "zekra hook rules" }] }],
    "UserPromptSubmit": [{ "hooks": [{ "type": "command", "command": "zekra hook recall" }] }]
  }
}
```

One-liner that installs, logs in, and wires a client in a single shot:

```sh
ZEKRA_TOKEN=cbt_… ZEKRA_CLIENT=claude-desktop \
  sh -c "$(curl -fsSL https://app.zekra.dev/install.sh)"
```

## Quick start

```sh
zekra auth login --token cbt_…          # save endpoint + token to ~/.zekra/config.json
zekra mcp:install claude-desktop        # wire the brain into your client — done
# restart the client; the brain tools (memory_recall, memory_retain, …) appear automatically
```

Get a token from an admin: `zekra auth token new my-laptop` (admin), or ask the brain owner.

## Commands

| command | what it does |
|---|---|
| `zekra auth login [--url U] [--token T] [--agent ID] [--brain NS]` | save credentials (verifies reachability) |
| `zekra auth logout` | forget the saved token |
| `zekra auth whoami` | show endpoint + which brains your token can reach |
| `zekra auth token new <agentId> [--admin] [--brain NS]` | mint a token, optionally grant it a brain |
| `zekra auth token list` | list tokens + grants (admin) |
| `zekra mcp` | run the stdio MCP server (this is what clients invoke) |
| `zekra mcp:install <client> [--brain NS] [--name N] [--user]` | wire the MCP into a client |
| `zekra mcp:print <client> [--brain NS]` | print the config snippet, write nothing |
| `zekra mcp:uninstall <client> [--name N]` | remove the zekra entry from a client |
| `zekra brain list` | list brains you can read |
| `zekra brain create <name> [--description D] [--token]` | create a new empty named brain (+ optional scoped token) |
| `zekra brain delete <name> --confirm` | delete a brain and all its memories |
| `zekra recall <brain> <query…>` | hybrid recall (vector + BM25 + rerank) |
| `zekra retain <brain> <content…>` | store a memory |

Colon (`mcp:install`) and space (`mcp install`) forms are equivalent.

## Supported clients

| client | config file it writes | format |
|---|---|---|
| `claude-code` | `./.mcp.json` (project) or `~/.claude.json` with `--user` | JSON |
| `claude-desktop` | `~/Library/Application Support/Claude/claude_desktop_config.json` (mac), `%APPDATA%\Claude\…` (win), `~/.config/Claude/…` (linux) | JSON |
| `codex` | `~/.codex/config.toml` `[mcp_servers.zekra]` | TOML |
| `gemini` | `~/.gemini/settings.json` | JSON |
| `cursor` | `~/.cursor/mcp.json` | JSON |
| `print` | stdout only | JSON |

Installs **merge** into existing config (other MCP servers are preserved) and are
**idempotent** — re-running replaces just the `zekra` entry.

## Creating and sharing a brain

```sh
zekra brain create research --description "market + competitor notes" --token
```

This seeds the namespace, mints a **non-admin token scoped to just that brain**, grants
it read+write, and prints a ready-to-paste MCP snippet you can hand to a teammate — they
paste it into their client (or run `zekra mcp:install <client> --brain research`) and
they're in, with access to *only* that brain.

> Brains are enumerated from stored memories, so `brain create` writes one genesis marker
> to make the namespace exist and be connectable.

## Configuration

Resolution order (highest wins): **flags → environment → `~/.zekra/config.json`**.

| env var | meaning |
|---|---|
| `ZEKRA_API_URL` | base URL of the Zekra app (default `https://app.zekra.dev`) |
| `ZEKRA_TOKEN` | ACL token, sent as `X-Zekra-Token` (per-brain read/write; admin bypasses grants) |
| `ZEKRA_AGENT_ID` | this session's agent identity, sent as `X-Agent-Id` |
| `ZEKRA_DEFAULT_NAMESPACE` | bind the MCP session to one brain (tools default `namespace` to it) |

The installer bakes these into each client's `env` block, so the client launches
`zekra mcp` fully configured.

## MCP tools exposed

`memory_recall`, `memory_retain`, `memory_get`, `memory_forget`, `memory_edit`,
`memory_gaps`, `memory_resolve_gap`, `brain_list`, `brain_details`, `brain_create`,
`brain_delete`, `brain_grant`, `brain_revoke_grant`, `brain_create_token`,
`brain_tokens`, `brain_chat`, `secret_list/store/reveal/delete`,
`datasource_list/create/sync/delete` — 24 tools mirroring the Zekra REST contract.

## Build

```sh
go build -ldflags "-X main.version=$(git describe --tags)" -o zekra .
```

Zero external dependencies (stdlib only) → one static binary per platform.
