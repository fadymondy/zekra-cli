// Command cabrain is the CaBrain memory CLI + MCP installer.
//
// It does two jobs:
//
//  1. It IS a Model Context Protocol server (`cabrain mcp`) — a thin stdio
//     adapter over a running CaBrain app's REST API, so Claude Code, Claude
//     Desktop, Codex, Gemini CLI, Cursor, and any MCP client can use the brain.
//  2. It wires that server into those clients (`cabrain install <client>`) and
//     drives the brain from the shell (`cabrain brain create`, `recall`, …).
//
// Config resolution order (highest first): flags → environment → ~/.cabrain/config.json.
//
//	CABRAIN_API_URL            base URL of the CaBrain app (default https://cabrain-app.fadymondy.com)
//	CABRAIN_TOKEN              ACL token (X-Cabrain-Token) → per-brain read/write
//	CABRAIN_AGENT_ID           this session's agent identity (X-Agent-Id)
//	CABRAIN_DEFAULT_NAMESPACE  bind the MCP session to one brain
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// defaultURL is the CaBrain app (console + REST + MCP API). cabrain.fadymondy.com is the
// marketing landing since 2026-09-14; legacyURL maps configs saved before then onto the app.
const (
	defaultURL = "https://cabrain-app.fadymondy.com"
	legacyURL  = "https://cabrain.fadymondy.com"
)

// version is stamped at build time: -ldflags "-X main.version=v0.1.0".
var version = "dev"

// Config is the persisted CLI/MCP configuration (~/.cabrain/config.json).
type Config struct {
	URL       string `json:"url,omitempty"`
	Token     string `json:"token,omitempty"`
	AgentID   string `json:"agentId,omitempty"`
	Namespace string `json:"namespace,omitempty"` // optional default brain
}

func configPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cabrain", "config.json")
}

// loadConfig reads the config file then overlays environment variables. Flags,
// where present, are layered on top by each command.
func loadConfig() Config {
	var c Config
	if b, err := os.ReadFile(configPath()); err == nil {
		_ = json.Unmarshal(b, &c)
	}
	if v := os.Getenv("CABRAIN_API_URL"); v != "" {
		c.URL = v
	}
	if v := os.Getenv("CABRAIN_TOKEN"); v != "" {
		c.Token = v
	}
	if v := os.Getenv("CABRAIN_AGENT_ID"); v != "" {
		c.AgentID = v
	}
	if v := os.Getenv("CABRAIN_DEFAULT_NAMESPACE"); v != "" {
		c.Namespace = v
	}
	c.URL = strings.TrimRight(c.URL, "/")
	// Empty, or saved by `cabrain auth login` before the app moved off the landing domain.
	if c.URL == "" || c.URL == legacyURL {
		c.URL = defaultURL
	}
	return c
}

func saveConfig(c Config) error {
	p := configPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(c, "", "  ")
	return os.WriteFile(p, append(b, '\n'), 0o600)
}

// --- HTTP client over the brain REST surface ---------------------------------

type client struct {
	base, token, agent string
	hc                 *http.Client
}

func newClient(c Config) *client {
	return &client{base: c.URL, token: c.Token, agent: c.AgentID, hc: &http.Client{Timeout: 60 * time.Second}}
}

func (c *client) do(method, path string, q url.Values, payload any) (map[string]any, int, error) {
	var body io.Reader
	if payload != nil {
		b, _ := json.Marshal(payload)
		body = bytes.NewReader(b)
	}
	u := c.base + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(context.Background(), method, u, body)
	if err != nil {
		return nil, 0, err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("X-Cabrain-Token", c.token)
	}
	if c.agent != "" {
		req.Header.Set("X-Agent-Id", c.agent)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var m map[string]any
	if len(raw) > 0 && json.Unmarshal(raw, &m) != nil {
		m = map[string]any{"raw": string(raw)}
	}
	return m, resp.StatusCode, nil
}

// apiErr extracts a human message from a structured error body.
func apiErr(m map[string]any, code int) error {
	if m != nil {
		if e, ok := m["error"].(map[string]any); ok {
			return fmt.Errorf("%v (%v)", e["message"], e["code"])
		}
		if s, ok := m["error"].(string); ok {
			return fmt.Errorf("%s", s)
		}
	}
	return fmt.Errorf("HTTP %d", code)
}

// --- command dispatch ---------------------------------------------------------

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		usage()
		os.Exit(0)
	}
	// Support artisan-style colon commands (`mcp:install`) alongside grouped
	// subcommands (`mcp install`). Normalise `group:sub` → `group sub …`.
	if strings.Contains(args[0], ":") {
		p := strings.SplitN(args[0], ":", 2)
		args = append([]string{p[0], p[1]}, args[1:]...)
	}
	cmd, rest := args[0], args[1:]
	var err error
	switch cmd {
	// --- MCP server + installer (the core helper) ---
	case "mcp":
		// `cabrain mcp`           → run the stdio server (what clients invoke)
		// `cabrain mcp install …` → wire it into a client
		if len(rest) > 0 {
			switch rest[0] {
			case "install", "add", "setup":
				err = cmdInstall(rest[1:])
			case "print", "config", "snippet":
				err = cmdMCPPrint(rest[1:])
			case "uninstall", "remove":
				err = cmdUninstall(rest[1:])
			default:
				err = fmt.Errorf("unknown: cabrain mcp %s (try: install | print | uninstall)", rest[0])
			}
		} else {
			runMCP(loadConfig()) // blocks until stdin closes
		}

	// --- auth group (login / token / whoami) ---
	case "auth":
		err = cmdAuth(rest)

	// --- brains + memory ---
	case "brain", "brains":
		err = cmdBrain(rest)
	case "recall":
		err = cmdRecall(rest)
	case "retain":
		err = cmdRetain(rest)

	// --- short top-level aliases (new-user friendly) ---
	case "install":
		err = cmdInstall(rest)
	case "login":
		err = cmdLogin(rest)
	case "logout":
		err = cmdLogout(rest)
	case "status", "ping", "whoami":
		err = cmdStatus(rest)
	case "token", "tokens":
		err = cmdToken(rest)

	// --- plugin hooks (read hook JSON on stdin, emit injected context) ---
	case "hook":
		err = cmdHook(rest)

	case "version", "--version", "-v":
		fmt.Printf("cabrain %s\n", version)
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", cmd)
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// cmdAuth routes the auth.* group.
func cmdAuth(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: cabrain auth <login|logout|token|whoami> …")
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "login":
		return cmdLogin(rest)
	case "logout":
		return cmdLogout(rest)
	case "whoami", "status":
		return cmdStatus(rest)
	case "token", "tokens":
		return cmdToken(rest)
	}
	return fmt.Errorf("unknown: cabrain auth %s", sub)
}

func usage() {
	fmt.Print(`cabrain — connect any AI client to the CaBrain memory system

QUICK START (new user)
  cabrain auth login --token <cbt_…>          save your endpoint + token
  cabrain mcp:install claude-desktop          wire the brain into your client — done
  # (also: claude-code · codex · gemini · cursor)

AUTH
  cabrain auth login [--url URL] [--token TOKEN] [--agent ID] [--brain NS]
  cabrain auth logout
  cabrain auth whoami                          show endpoint + which brains you can reach
  cabrain auth token new <agentId> [--admin] [--brain NAME]   mint a token (+grant a brain)
  cabrain auth token list

MCP
  cabrain mcp                                  run the stdio MCP server (clients invoke this)
  cabrain mcp:install <client> [--brain N] [--name N] [--user]   wire into a client
  cabrain mcp:print   <client> [--brain N]     print the config snippet, install nothing
  cabrain mcp:uninstall <client> [--name N]    remove the cabrain entry from a client

BRAINS
  cabrain brain list
  cabrain brain create <name> [--description D] [--token]   new empty named brain (+ optional scoped token)
  cabrain brain delete <name> --confirm

MEMORY
  cabrain recall <brain> <query...>            hybrid recall (vector + BM25 + rerank)
  cabrain retain <brain> <content...>          store a memory

CLIENTS:  claude-code · claude-desktop · codex · gemini · cursor · print
Config:   flags > env (CABRAIN_API_URL/TOKEN/AGENT_ID/DEFAULT_NAMESPACE) > ~/.cabrain/config.json
`)
}

// --- flag helpers (stdlib flag is awkward for "cmd sub --flag arg") -----------

// parseFlags splits positional args from --key value / --key=value / --bool flags.
func parseFlags(args []string) (pos []string, flags map[string]string) {
	flags = map[string]string{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "--") {
			k := strings.TrimPrefix(a, "--")
			if eq := strings.IndexByte(k, '='); eq >= 0 {
				flags[k[:eq]] = k[eq+1:]
				continue
			}
			// boolean flag unless the next arg is a value
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
				flags[k] = args[i+1]
				i++
			} else {
				flags[k] = "true"
			}
		} else {
			pos = append(pos, a)
		}
	}
	return pos, flags
}

func pretty(m any) string { b, _ := json.MarshalIndent(m, "", "  "); return string(b) }
