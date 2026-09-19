package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// remoteMCPURL is the hosted Zekra MCP endpoint (streamable HTTP, OAuth; no token).
const remoteMCPURL = "https://mcp.zekra.dev"

// remoteEntry returns the client-specific remote MCP entry, or nil when the
// client has no file-based remote MCP support (stdio stays the default there).
func remoteEntry(target string) map[string]any {
	switch target {
	case "claude-code", "cursor", "print":
		return map[string]any{"type": "http", "url": remoteMCPURL}
	case "gemini":
		return map[string]any{"httpUrl": remoteMCPURL} // Gemini CLI's streamable-HTTP key
	}
	return nil
}

// installRemote writes the hosted MCP entry; the client runs the OAuth sign-in
// on first use, so no token is written to disk.
func installRemote(target, name string, userScope bool) error {
	entry := remoteEntry(target)
	if entry == nil {
		if target == "claude-desktop" {
			return fmt.Errorf("Claude Desktop adds remote servers in Settings > Connectors > Add custom connector (URL %s); drop --remote to install the stdio server", remoteMCPURL)
		}
		return fmt.Errorf("--remote is not supported for %q (supported: claude-code, cursor, gemini, print); drop --remote to install the stdio server", target)
	}
	if target == "print" {
		fmt.Println(pretty(map[string]any{"mcpServers": map[string]any{name: entry}}))
		return nil
	}
	spec, ok := clients()[target]
	if !ok {
		return fmt.Errorf("unknown client %q", target)
	}
	p, err := spec.path(userScope)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	root := map[string]any{}
	if b, err := os.ReadFile(p); err == nil && len(strings.TrimSpace(string(b))) > 0 {
		if json.Unmarshal(b, &root) != nil {
			return fmt.Errorf("existing %s is not valid JSON — fix or move it, then retry", p)
		}
	}
	ms, ok := root["mcpServers"].(map[string]any)
	if !ok {
		ms = map[string]any{}
	}
	ms[name] = entry
	root["mcpServers"] = ms
	b, _ := json.MarshalIndent(root, "", "  ")
	if err := os.WriteFile(p, append(b, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Printf("✓ installed remote \"%s\" MCP (%s) into %s\n  file: %s\n", name, remoteMCPURL, spec.name, p)
	fmt.Println("  auth: OAuth — the client opens a Zekra sign-in on first use (no token stored)")
	if target == "claude-code" {
		fmt.Printf("  equivalent: claude mcp add --transport http %s %s\n", name, remoteMCPURL)
	}
	return nil
}
