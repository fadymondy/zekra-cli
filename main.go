// Command zekra is the Zekra memory CLI + MCP installer.
//
// It does two jobs:
//
//  1. It IS a Model Context Protocol server (`zekra mcp`) — a thin stdio
//     adapter over a running Zekra app's REST API, so Claude Code, Claude
//     Desktop, Codex, Gemini CLI, Cursor, and any MCP client can use the brain.
//  2. It wires that server into those clients (`zekra install <client>`) and
//     drives the brain from the shell (`zekra brain create`, `recall`, …).
//
// Config resolution order (highest first): flags → environment → ~/.zekra/config.json.
//
//	ZEKRA_API_URL            base URL of the Zekra app (default https://app.zekra.dev)
//	ZEKRA_TOKEN              ACL token (X-Zekra-Token) → per-brain read/write
//	ZEKRA_AGENT_ID           this session's agent identity (X-Agent-Id)
//	ZEKRA_DEFAULT_NAMESPACE  bind the MCP session to one brain
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

// defaultURL is the Zekra app (console + REST + MCP API). zekra.dev is the marketing
// landing; legacyURLs maps configs saved against the landing or the pre-rename
// CaBrain hosts onto the app.
const defaultURL = "https://app.zekra.dev"

var legacyURLs = map[string]bool{
	"https://zekra.dev":                 true,
	"https://cabrain.fadymondy.com":     true, // compat: pre-rename landing
	"https://cabrain-app.fadymondy.com": true, // compat: pre-rename app
}

// version is stamped at build time: -ldflags "-X main.version=v0.1.0".
var version = "dev"

// Config is the persisted CLI/MCP configuration (~/.zekra/config.json).
type Config struct {
	URL       string `json:"url,omitempty"`
	Token     string `json:"token,omitempty"`
	AgentID   string `json:"agentId,omitempty"`
	Namespace string `json:"namespace,omitempty"` // optional default brain
}

func configPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".zekra", "config.json")
}

// legacyConfigPath is the pre-rename (CaBrain) config location, read as a fallback.
func legacyConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cabrain", "config.json")
}

// getenv returns ZEKRA_<name>, falling back to the pre-rename CABRAIN_<name>.
func getenv(name string) string {
	if v := os.Getenv("ZEKRA_" + name); v != "" {
		return v
	}
	return os.Getenv("CABRAIN_" + name)
}

// loadConfig reads the config file then overlays environment variables. Flags,
// where present, are layered on top by each command.
func loadConfig() Config {
	var c Config
	if b, err := os.ReadFile(configPath()); err == nil {
		_ = json.Unmarshal(b, &c)
	} else if b, err := os.ReadFile(legacyConfigPath()); err == nil {
		_ = json.Unmarshal(b, &c)
	}
	if v := getenv("API_URL"); v != "" {
		c.URL = v
	}
	if v := getenv("TOKEN"); v != "" {
		c.Token = v
	}
	if v := getenv("AGENT_ID"); v != "" {
		c.AgentID = v
	}
	if v := getenv("DEFAULT_NAMESPACE"); v != "" {
		c.Namespace = v
	}
	c.URL = strings.TrimRight(c.URL, "/")
	// Empty, or saved against the landing / pre-rename hosts.
	if c.URL == "" || legacyURLs[c.URL] {
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
		req.Header.Set("X-Zekra-Token", c.token)
		req.Header.Set("X-Cabrain-Token", c.token) // compat: pre-rename servers
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
		// `zekra mcp`           → run the stdio server (what clients invoke)
		// `zekra mcp install …` → wire it into a client
		if len(rest) > 0 {
			switch rest[0] {
			case "install", "add", "setup":
				err = cmdInstall(rest[1:])
			case "print", "config", "snippet":
				err = cmdMCPPrint(rest[1:])
			case "uninstall", "remove":
				err = cmdUninstall(rest[1:])
			default:
				err = fmt.Errorf("unknown: zekra mcp %s (try: install | print | uninstall)", rest[0])
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
		fmt.Printf("zekra %s\n", version)
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
		return fmt.Errorf("usage: zekra auth <login|logout|token|whoami> …")
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
	return fmt.Errorf("unknown: zekra auth %s", sub)
}

func usage() {
	fmt.Print(`zekra — connect any AI client to the Zekra memory system

QUICK START (new user)
  zekra auth login --token <cbt_…>          save your endpoint + token
  zekra mcp:install claude-desktop          wire the brain into your client — done
  # (also: claude-code · codex · gemini · cursor)

AUTH
  zekra auth login [--url URL] [--token TOKEN] [--agent ID] [--brain NS]
  zekra auth logout
  zekra auth whoami                          show endpoint + which brains you can reach
  zekra auth token new <agentId> [--admin] [--brain NAME]   mint a token (+grant a brain)
  zekra auth token list

MCP
  zekra mcp                                  run the stdio MCP server (clients invoke this)
  zekra mcp:install <client> [--brain N] [--name N] [--user]   wire into a client
  zekra mcp:print   <client> [--brain N]     print the config snippet, install nothing
  zekra mcp:uninstall <client> [--name N]    remove the zekra entry from a client

BRAINS
  zekra brain list
  zekra brain create <name> [--description D] [--token]   new empty named brain (+ optional scoped token)
  zekra brain delete <name> --confirm

MEMORY
  zekra recall <brain> <query...>            hybrid recall (vector + BM25 + rerank)
  zekra retain <brain> <content...>          store a memory

CLIENTS:  claude-code · claude-desktop · codex · gemini · cursor · print
Config:   flags > env (ZEKRA_API_URL/TOKEN/AGENT_ID/DEFAULT_NAMESPACE; legacy CABRAIN_* also read) > ~/.zekra/config.json
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
