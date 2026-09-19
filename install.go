package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

// A client describes where an MCP host keeps its config and in what format.
type clientSpec struct {
	name   string
	format string // "json" (mcpServers map) or "toml" ([mcp_servers.NAME])
	// path returns the config file path; userScope picks the global vs project file.
	path func(userScope bool) (string, error)
	note string
}

func clients() map[string]clientSpec {
	home, _ := os.UserHomeDir()
	return map[string]clientSpec{
		"claude-code": {
			name: "Claude Code", format: "json",
			path: func(user bool) (string, error) {
				if user {
					return filepath.Join(home, ".claude.json"), nil
				}
				wd, _ := os.Getwd()
				return filepath.Join(wd, ".mcp.json"), nil // project-scoped by default
			},
			note: "project .mcp.json by default; --user writes ~/.claude.json",
		},
		"claude-desktop": {
			name: "Claude Desktop", format: "json",
			path: func(bool) (string, error) { return claudeDesktopPath(home) },
			note: "restart Claude Desktop after install",
		},
		"codex": {
			name: "Codex CLI", format: "toml",
			path: func(bool) (string, error) { return filepath.Join(home, ".codex", "config.toml"), nil },
			note: "OpenAI Codex CLI — TOML [mcp_servers.*]",
		},
		"gemini": {
			name: "Gemini CLI", format: "json",
			path: func(user bool) (string, error) {
				if user {
					return filepath.Join(home, ".gemini", "settings.json"), nil
				}
				wd, _ := os.Getwd()
				return filepath.Join(wd, ".gemini", "settings.json"), nil
			},
			note: "Gemini CLI settings.json",
		},
		"cursor": {
			name: "Cursor", format: "json",
			path: func(user bool) (string, error) {
				if user {
					return filepath.Join(home, ".cursor", "mcp.json"), nil
				}
				wd, _ := os.Getwd()
				return filepath.Join(wd, ".cursor", "mcp.json"), nil
			},
			note: "Cursor mcp.json",
		},
	}
}

func claudeDesktopPath(home string) (string, error) {
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Claude", "claude_desktop_config.json"), nil
	case "windows":
		if ad := os.Getenv("APPDATA"); ad != "" {
			return filepath.Join(ad, "Claude", "claude_desktop_config.json"), nil
		}
		return filepath.Join(home, "AppData", "Roaming", "Claude", "claude_desktop_config.json"), nil
	default: // linux
		return filepath.Join(home, ".config", "Claude", "claude_desktop_config.json"), nil
	}
}

// serverEnv builds the env block for the MCP entry from the active config.
func serverEnv(c Config, brain string) map[string]string {
	env := map[string]string{"ZEKRA_API_URL": c.URL}
	if c.Token != "" {
		env["ZEKRA_TOKEN"] = c.Token
	}
	if c.AgentID != "" {
		env["ZEKRA_AGENT_ID"] = c.AgentID
	}
	if brain == "" {
		brain = c.Namespace
	}
	if brain != "" {
		env["ZEKRA_DEFAULT_NAMESPACE"] = brain
	}
	return env
}

// zekra mcp:install <client> [--brain N] [--name N] [--user] [--command PATH]
func cmdInstall(args []string) error {
	pos, f := parseFlags(args)
	if len(pos) == 0 {
		return fmt.Errorf("usage: zekra mcp:install <claude-code|claude-desktop|codex|gemini|cursor|print> [--brain N] [--name N] [--user]")
	}
	target := pos[0]
	c := loadConfig()
	name := f["name"]
	if name == "" {
		name = "zekra"
	}
	command := f["command"]
	if command == "" {
		command, _ = os.Executable() // point clients at THIS binary so `zekra mcp` resolves
		if command == "" {
			command = "zekra"
		}
	}
	env := serverEnv(c, f["brain"])

	if target == "print" {
		fmt.Println(jsonEntry(name, command, env))
		return nil
	}
	spec, ok := clients()[target]
	if !ok {
		return fmt.Errorf("unknown client %q (try: claude-code, claude-desktop, codex, gemini, cursor, print)", target)
	}
	if c.Token == "" {
		fmt.Println("⚠  no token saved — run `zekra auth login --token <cbt_…>` first (installing anyway).")
	}
	p, err := spec.path(f["user"] == "true")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	if spec.format == "toml" {
		err = writeTOML(p, name, command, env)
	} else {
		err = writeJSON(p, name, command, env)
	}
	if err != nil {
		return err
	}
	fmt.Printf("✓ installed \"%s\" MCP into %s\n  file: %s\n", name, spec.name, p)
	if spec.note != "" {
		fmt.Printf("  note: %s\n", spec.note)
	}
	fmt.Printf("  brain: %s\n", orAll(env["ZEKRA_DEFAULT_NAMESPACE"]))
	return nil
}

// zekra mcp:print <client> — show the snippet without writing.
func cmdMCPPrint(args []string) error {
	pos, _ := parseFlags(args)
	if len(pos) == 0 {
		pos = []string{"print"}
	}
	if pos[0] == "codex" {
		c := loadConfig()
		_, f := parseFlags(args)
		fmt.Print(tomlBlock(nameOr(f["name"]), execOr(f["command"]), serverEnv(c, f["brain"])))
		return nil
	}
	return cmdInstall(append([]string{"print"}, args[1:]...))
}

// zekra mcp:uninstall <client> [--name N] [--user]
func cmdUninstall(args []string) error {
	pos, f := parseFlags(args)
	if len(pos) == 0 {
		return fmt.Errorf("usage: zekra mcp:uninstall <client> [--name N]")
	}
	spec, ok := clients()[pos[0]]
	if !ok {
		return fmt.Errorf("unknown client %q", pos[0])
	}
	name := nameOr(f["name"])
	p, err := spec.path(f["user"] == "true")
	if err != nil {
		return err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return fmt.Errorf("nothing to remove (%s not found)", p)
	}
	if spec.format == "toml" {
		out := removeTOMLBlock(string(b), name)
		if err := os.WriteFile(p, []byte(out), 0o644); err != nil {
			return err
		}
	} else {
		var root map[string]any
		if json.Unmarshal(b, &root) != nil {
			return fmt.Errorf("could not parse %s", p)
		}
		if ms, ok := root["mcpServers"].(map[string]any); ok {
			delete(ms, name)
		}
		nb, _ := json.MarshalIndent(root, "", "  ")
		if err := os.WriteFile(p, append(nb, '\n'), 0o644); err != nil {
			return err
		}
	}
	fmt.Printf("✓ removed \"%s\" from %s (%s)\n", name, spec.name, p)
	return nil
}

// --- JSON config merge (Claude Code/Desktop, Gemini, Cursor) ------------------

func writeJSON(path, name, command string, env map[string]string) error {
	root := map[string]any{}
	if b, err := os.ReadFile(path); err == nil && len(strings.TrimSpace(string(b))) > 0 {
		if json.Unmarshal(b, &root) != nil {
			return fmt.Errorf("existing %s is not valid JSON — fix or move it, then retry", path)
		}
	}
	ms, ok := root["mcpServers"].(map[string]any)
	if !ok {
		ms = map[string]any{}
	}
	ms[name] = entryMap(command, env)
	root["mcpServers"] = ms
	b, _ := json.MarshalIndent(root, "", "  ")
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

func entryMap(command string, env map[string]string) map[string]any {
	return map[string]any{"command": command, "args": []string{"mcp"}, "env": env}
}

func jsonEntry(name, command string, env map[string]string) string {
	return pretty(map[string]any{"mcpServers": map[string]any{name: entryMap(command, env)}})
}

// --- TOML config merge (Codex) ------------------------------------------------

func writeTOML(path, name, command string, env map[string]string) error {
	existing := ""
	if b, err := os.ReadFile(path); err == nil {
		existing = removeTOMLBlock(string(b), name)
	}
	existing = strings.TrimRight(existing, "\n")
	block := tomlBlock(name, command, env)
	sep := "\n\n"
	if existing == "" {
		sep = ""
	}
	return os.WriteFile(path, []byte(existing+sep+block), 0o644)
}

func tomlBlock(name, command string, env map[string]string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "[mcp_servers.%s]\n", name)
	fmt.Fprintf(&sb, "command = %q\n", command)
	fmt.Fprintf(&sb, "args = [\"mcp\"]\n")
	// deterministic env order
	keys := []string{"ZEKRA_API_URL", "ZEKRA_TOKEN", "ZEKRA_AGENT_ID", "ZEKRA_DEFAULT_NAMESPACE"}
	pairs := []string{}
	for _, k := range keys {
		if v, ok := env[k]; ok {
			pairs = append(pairs, fmt.Sprintf("%s = %q", k, v))
		}
	}
	fmt.Fprintf(&sb, "env = { %s }\n", strings.Join(pairs, ", "))
	return sb.String()
}

// removeTOMLBlock strips an existing [mcp_servers.<name>] table (up to the next
// top-level table header or EOF) so re-installs are idempotent.
func removeTOMLBlock(s, name string) string {
	header := fmt.Sprintf("[mcp_servers.%s]", name)
	lines := strings.Split(s, "\n")
	out := []string{}
	skip := false
	tableRe := regexp.MustCompile(`^\s*\[`)
	for _, ln := range lines {
		if strings.TrimSpace(ln) == header {
			skip = true
			continue
		}
		if skip {
			if tableRe.MatchString(ln) { // next table starts → stop skipping
				skip = false
			} else {
				continue
			}
		}
		out = append(out, ln)
	}
	return strings.Join(out, "\n")
}

// --- tiny helpers -------------------------------------------------------------

func orAll(s string) string {
	if s == "" {
		return "(all brains this token can read)"
	}
	return s
}

func nameOr(n string) string {
	if n == "" {
		return "zekra"
	}
	return n
}

func execOr(cmd string) string {
	if cmd != "" {
		return cmd
	}
	if e, _ := os.Executable(); e != "" {
		return e
	}
	return "zekra"
}
