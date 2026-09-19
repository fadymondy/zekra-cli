# cabrain-cli

**Connect any AI client to the [CaBrain](https://cabrain.fadymondy.com) memory system in one command.**

`cabrain` is a single, dependency-free Go binary that is *both* the MCP server and its
installer. It speaks the [Model Context Protocol](https://modelcontextprotocol.io) over
stdio (a thin adapter over the CaBrain REST API), and it wires that server into every
MCP host — Claude Desktop, Claude Code, Codex, Gemini CLI, Cursor — so a new user goes
from zero to a working brain in two commands.

```
┌────────────┐   spawns    ┌──────────────┐   HTTPS + X-Cabrain-Token   ┌──────────────┐
│ MCP client │ ──────────▶ │ cabrain mcp  │ ──────────────────────────▶ │ CaBrain API  │
│ (Claude…)  │  (stdio)    │ (this binary)│                             │ brains + ACL │
└────────────┘             └──────────────┘                             └──────────────┘
```

## Install

```sh
curl -fsSL https://cabrain-app.fadymondy.com/install.sh | sh
```

or, with Go:

```sh
go install github.com/togo-framework/cabrain-cli@latest   # installs `cabrain`
```

or, with npm:

```sh
npm i -g cabrain-cli
```

**Upgrade** to the latest anytime (npm / go / binary aware):

```sh
curl -fsSL https://cabrain-app.fadymondy.com/upgrade.sh | sh
```

## Use it as a Claude Code plugin

This repo is **also a Claude Code plugin** — it bundles the CaBrain MCP, memory-first
slash commands, two agents, and a skill. Install:

```
/plugin marketplace add togo-framework/cabrain-cli
/plugin install cabrain@cabrain
```

At enable time it prompts for your **CaBrain token** (and optional endpoint / default
brain) and wires the MCP automatically — no `.mcp.json` editing. Requires the `cabrain`
binary on PATH (any install method above), since the plugin's MCP server *is* `cabrain mcp`.

**What the plugin adds**

| kind | name | what it does |
|---|---|---|
| command | `/cabrain:recall [brain] <query>` | hybrid recall, answer from memory |
| command | `/cabrain:retain [brain] <content>` | store a memory |
| command | `/cabrain:brains [name]` | list brains / brain details |
| command | `/cabrain:new-brain <name>` | create a new brain |
| command | `/cabrain:connect [client]` | wire the MCP into another client |
| command | `/cabrain:status` | endpoint + reachable brains |
| command | `/cabrain:doctor` | diagnose CLI / auth / MCP |
| agent | `cabrain-curator` | memory-first Q&A + retention (recall→answer→retain) |
| agent | `cabrain-connector` | onboarding: install, tokens, brains, datasources |
| skill | `memory-first` | the recall→answer/act→retain discipline |
| hook | `SessionStart` | auto-injects the memory-first rules into every session |
| hook | `UserPromptSubmit` | opt-in auto-recall — pulls relevant memories for each prompt |

### Hooks — auto push/pull

The plugin ships two hooks (backed by `cabrain hook …`, so no `jq`/`python` needed — the
binary parses the hook JSON itself, and every hook **fails open**: any error → no output,
never blocks you):

- **Memory-first rules (SessionStart)** — injects the recall→answer→retain discipline at
  the start of every session, so the model reaches for the brain automatically. **On by
  default;** turn off with the `inject_rules` plugin option or `CABRAIN_HOOK_RULES=0`.
- **Auto-recall / pull (UserPromptSubmit)** — uses each prompt to recall the top matching
  memories from your default brain and injects them as context. **Opt-in** (it hits the
  brain every prompt): enable the `auto_recall` plugin option or `CABRAIN_HOOK_AUTORECALL=1`,
  and set a default brain (`CABRAIN_AUTORECALL_BRAIN`, `CABRAIN_DEFAULT_NAMESPACE`, or the
  plugin's default-brain option). Needs a saved token (`cabrain auth login`) or the plugin's
  `api_token`.

**Push** (writing to the brain) stays model-driven on purpose — retention should be
selective and distilled, not a firehose. The injected rules tell the model to `memory_retain`
what's durable; `/cabrain:retain` and the `cabrain-curator` agent do it explicitly.

Standalone (outside the plugin) you can wire the same hooks in your own `settings.json`:

```json
{
  "hooks": {
    "SessionStart":     [{ "hooks": [{ "type": "command", "command": "cabrain hook rules" }] }],
    "UserPromptSubmit": [{ "hooks": [{ "type": "command", "command": "cabrain hook recall" }] }]
  }
}
```

One-liner that installs, logs in, and wires a client in a single shot:

```sh
CABRAIN_TOKEN=cbt_… CABRAIN_CLIENT=claude-desktop \
  sh -c "$(curl -fsSL https://cabrain-app.fadymondy.com/install.sh)"
```

## Quick start

```sh
cabrain auth login --token cbt_…          # save endpoint + token to ~/.cabrain/config.json
cabrain mcp:install claude-desktop        # wire the brain into your client — done
# restart the client; the brain tools (memory_recall, memory_retain, …) appear automatically
```

Get a token from an admin: `cabrain auth token new my-laptop` (admin), or ask the brain owner.

## Commands

| command | what it does |
|---|---|
| `cabrain auth login [--url U] [--token T] [--agent ID] [--brain NS]` | save credentials (verifies reachability) |
| `cabrain auth logout` | forget the saved token |
| `cabrain auth whoami` | show endpoint + which brains your token can reach |
| `cabrain auth token new <agentId> [--admin] [--brain NS]` | mint a token, optionally grant it a brain |
| `cabrain auth token list` | list tokens + grants (admin) |
| `cabrain mcp` | run the stdio MCP server (this is what clients invoke) |
| `cabrain mcp:install <client> [--brain NS] [--name N] [--user]` | wire the MCP into a client |
| `cabrain mcp:print <client> [--brain NS]` | print the config snippet, write nothing |
| `cabrain mcp:uninstall <client> [--name N]` | remove the cabrain entry from a client |
| `cabrain brain list` | list brains you can read |
| `cabrain brain create <name> [--description D] [--token]` | create a new empty named brain (+ optional scoped token) |
| `cabrain brain delete <name> --confirm` | delete a brain and all its memories |
| `cabrain recall <brain> <query…>` | hybrid recall (vector + BM25 + rerank) |
| `cabrain retain <brain> <content…>` | store a memory |

Colon (`mcp:install`) and space (`mcp install`) forms are equivalent.

## Supported clients

| client | config file it writes | format |
|---|---|---|
| `claude-code` | `./.mcp.json` (project) or `~/.claude.json` with `--user` | JSON |
| `claude-desktop` | `~/Library/Application Support/Claude/claude_desktop_config.json` (mac), `%APPDATA%\Claude\…` (win), `~/.config/Claude/…` (linux) | JSON |
| `codex` | `~/.codex/config.toml` `[mcp_servers.cabrain]` | TOML |
| `gemini` | `~/.gemini/settings.json` | JSON |
| `cursor` | `~/.cursor/mcp.json` | JSON |
| `print` | stdout only | JSON |

Installs **merge** into existing config (other MCP servers are preserved) and are
**idempotent** — re-running replaces just the `cabrain` entry.

## Creating and sharing a brain

```sh
cabrain brain create research --description "market + competitor notes" --token
```

This seeds the namespace, mints a **non-admin token scoped to just that brain**, grants
it read+write, and prints a ready-to-paste MCP snippet you can hand to a teammate — they
paste it into their client (or run `cabrain mcp:install <client> --brain research`) and
they're in, with access to *only* that brain.

> Brains are enumerated from stored memories, so `brain create` writes one genesis marker
> to make the namespace exist and be connectable.

## Configuration

Resolution order (highest wins): **flags → environment → `~/.cabrain/config.json`**.

| env var | meaning |
|---|---|
| `CABRAIN_API_URL` | base URL of the CaBrain app (default `https://cabrain-app.fadymondy.com`) |
| `CABRAIN_TOKEN` | ACL token, sent as `X-Cabrain-Token` (per-brain read/write; admin bypasses grants) |
| `CABRAIN_AGENT_ID` | this session's agent identity, sent as `X-Agent-Id` |
| `CABRAIN_DEFAULT_NAMESPACE` | bind the MCP session to one brain (tools default `namespace` to it) |

The installer bakes these into each client's `env` block, so the client launches
`cabrain mcp` fully configured.

## MCP tools exposed

`memory_recall`, `memory_retain`, `memory_get`, `memory_forget`, `memory_edit`,
`memory_gaps`, `memory_resolve_gap`, `brain_list`, `brain_details`, `brain_create`,
`brain_delete`, `brain_grant`, `brain_revoke_grant`, `brain_create_token`,
`brain_tokens`, `brain_chat`, `secret_list/store/reveal/delete`,
`datasource_list/create/sync/delete` — 24 tools mirroring the CaBrain REST contract.

## Build

```sh
go build -ldflags "-X main.version=$(git describe --tags)" -o cabrain .
```

Zero external dependencies (stdlib only) → one static binary per platform.
