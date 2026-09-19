package main

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// zekra login [--url URL] [--token TOKEN] [--agent ID] [--brain NS]
func cmdLogin(args []string) error {
	_, f := parseFlags(args)
	c := loadConfig()
	if v := f["url"]; v != "" {
		c.URL = strings.TrimRight(v, "/")
	}
	if v := f["token"]; v != "" {
		c.Token = v
	}
	if v := f["agent"]; v != "" {
		c.AgentID = v
	}
	if v := f["brain"]; v != "" {
		c.Namespace = v
	}
	if err := saveConfig(c); err != nil {
		return err
	}
	fmt.Printf("saved %s\n  url:   %s\n  token: %s\n", configPath(), c.URL, mask(c.Token))
	if c.AgentID != "" {
		fmt.Printf("  agent: %s\n", c.AgentID)
	}
	// Verify the credentials right away.
	cl := newClient(c)
	m, code, err := cl.do("GET", "/api/brain/ping", nil, nil)
	if err != nil {
		fmt.Printf("  (warning: could not reach %s: %v)\n", c.URL, err)
		return nil
	}
	if code == 200 {
		fmt.Printf("  reachable: %v\n", m["status"])
	}
	return nil
}

// zekra auth logout — forget saved credentials.
func cmdLogout(args []string) error {
	c := loadConfig()
	c.Token = ""
	if err := saveConfig(c); err != nil {
		return err
	}
	fmt.Printf("cleared token from %s (endpoint %s kept)\n", configPath(), c.URL)
	return nil
}

// zekra status
func cmdStatus(args []string) error {
	c := loadConfig()
	cl := newClient(c)
	fmt.Printf("endpoint: %s\n", c.URL)
	fmt.Printf("token:    %s\n", mask(c.Token))
	m, code, err := cl.do("GET", "/api/brain/ping", nil, nil)
	if err != nil {
		return fmt.Errorf("unreachable: %w", err)
	}
	fmt.Printf("ping:     HTTP %d  %v  (authRequired=%v)\n", code, m["status"], m["authRequired"])
	// Which brains can this token see?
	nm, code, _ := cl.do("GET", "/api/brain/namespaces", nil, nil)
	if code == 200 {
		if brains, ok := nm["brains"].([]any); ok {
			fmt.Printf("brains:   %d reachable\n", len(brains))
			for _, b := range brains {
				if bm, ok := b.(map[string]any); ok {
					fmt.Printf("  • %-14v %v memories\n", bm["namespace"], bm["memories"])
				}
			}
		}
	} else if code == 401 || code == 403 {
		fmt.Println("brains:   (token missing or lacks grants — run `zekra login --token …`)")
	}
	return nil
}

// zekra brain <sub>
func cmdBrain(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: zekra brain <list|create|delete> …")
	}
	sub, rest := args[0], args[1:]
	c := loadConfig()
	cl := newClient(c)
	switch sub {
	case "list", "ls":
		m, code, err := cl.do("GET", "/api/brain/namespaces", nil, nil)
		if err != nil {
			return err
		}
		if code >= 400 {
			return apiErr(m, code)
		}
		brains, _ := m["brains"].([]any)
		if len(brains) == 0 {
			fmt.Println("(no brains reachable with this token)")
			return nil
		}
		for _, b := range brains {
			bm, _ := b.(map[string]any)
			fmt.Printf("%-16v %v memories   last: %v\n", bm["namespace"], bm["memories"], bm["lastAt"])
		}
		return nil

	case "create", "new":
		pos, f := parseFlags(rest)
		if len(pos) == 0 {
			return fmt.Errorf("usage: zekra brain create <name> [--description D] [--token]")
		}
		return brainCreate(cl, c, pos[0], f)

	case "delete", "rm":
		pos, f := parseFlags(rest)
		if len(pos) == 0 {
			return fmt.Errorf("usage: zekra brain delete <name> --confirm")
		}
		if f["confirm"] != "true" && f["confirm"] != "yes" {
			return fmt.Errorf("refusing to delete brain %q without --confirm", pos[0])
		}
		// The API requires confirm == namespace as a guard.
		m, code, err := cl.do("POST", "/api/brain/brain/delete", nil, map[string]any{
			"namespace": pos[0], "confirm": pos[0]})
		if err != nil {
			return err
		}
		if code >= 400 {
			return apiErr(m, code)
		}
		fmt.Printf("deleted brain %q: %s\n", pos[0], pretty(m))
		return nil
	}
	return fmt.Errorf("unknown: zekra brain %s", sub)
}

// brainCreate materialises a new named brain. Brains are enumerated from the
// memories table, so an empty namespace is invisible; we seed a single genesis
// marker so the brain shows up and is immediately connectable. With --token we
// also mint a non-admin token scoped to just this brain and print a ready-to-paste
// MCP config for it.
func brainCreate(cl *client, c Config, name string, f map[string]string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("brain name is required")
	}
	desc := f["description"]
	if desc == "" {
		desc = fmt.Sprintf("Genesis marker for the %q brain.", name)
	}
	// 1. Seed the genesis memory (creates the namespace).
	m, code, err := cl.do("POST", "/api/brain/retain", nil, map[string]any{
		"namespace":      name,
		"content":        fmt.Sprintf("Brain %q created via zekra-cli. %s", name, desc),
		"sourceKind":     "system",
		"sourceRef":      "zekra-cli/brain-create",
		"importanceHint": 0.9,
	})
	if err != nil {
		return err
	}
	if code >= 400 {
		return apiErr(m, code)
	}
	fmt.Printf("✓ created brain %q (genesis memory %v)\n", name, m["id"])

	// 2. Optionally mint a scoped token + grant, and print connect instructions.
	if f["token"] == "true" || f["connect"] == "true" {
		agent := f["agent"]
		if agent == "" {
			agent = "brain-" + name
		}
		tm, code, err := cl.do("POST", "/api/brain/tokens", nil, map[string]any{
			"agentId": agent, "label": "zekra-cli scoped: " + name, "isAdmin": false})
		if err != nil {
			return err
		}
		if code >= 400 {
			return fmt.Errorf("brain created, but minting a scoped token needs an admin token: %w", apiErr(tm, code))
		}
		tok, _ := tm["token"].(string)
		// Grant that token read+write on the new brain.
		if _, code, _ := cl.do("POST", "/api/brain/grant", nil, map[string]any{
			"agentId": agent, "namespace": name, "canRead": true, "canWrite": true}); code >= 400 {
			fmt.Println("  (warning: token minted but grant failed — grant it manually)")
		}
		fmt.Printf("\n  scoped token (agent %q): %s\n", agent, tok)
		fmt.Println("\n  paste into any MCP client (Claude Desktop / Claude Code / Cursor / Gemini):")
		fmt.Println(indent(jsonSnippet("zekra-"+name, c.URL, tok, name)))
		fmt.Printf("\n  or run:  zekra install claude-desktop --brain %s --name zekra-%s\n", name, name)
	} else {
		fmt.Printf("  connect it:  zekra install <client> --brain %s\n", name)
		fmt.Printf("  (add --token to mint a scoped token you can hand to a teammate)\n")
	}
	return nil
}

// zekra token <sub>
func cmdToken(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: zekra token <list|new> …")
	}
	sub, rest := args[0], args[1:]
	cl := newClient(loadConfig())
	switch sub {
	case "list", "ls":
		m, code, err := cl.do("GET", "/api/brain/tokens", nil, nil)
		if err != nil {
			return err
		}
		if code >= 400 {
			return apiErr(m, code)
		}
		toks, _ := m["tokens"].([]any)
		for _, t := range toks {
			tm, _ := t.(map[string]any)
			grants := []string{}
			if gs, ok := tm["grants"].([]any); ok {
				for _, g := range gs {
					if gm, ok := g.(map[string]any); ok {
						grants = append(grants, fmt.Sprint(gm["namespace"]))
					}
				}
			}
			fmt.Printf("%-24v admin=%-5v revoked=%-5v grants=%v\n",
				tm["agentId"], tm["isAdmin"], tm["revoked"], grants)
		}
		return nil

	case "new", "create":
		pos, f := parseFlags(rest)
		if len(pos) == 0 {
			return fmt.Errorf("usage: zekra token new <agentId> [--admin] [--brain NAME]")
		}
		agent := pos[0]
		m, code, err := cl.do("POST", "/api/brain/tokens", nil, map[string]any{
			"agentId": agent, "label": f["label"], "isAdmin": f["admin"] == "true"})
		if err != nil {
			return err
		}
		if code >= 400 {
			return apiErr(m, code)
		}
		tok, _ := m["token"].(string)
		fmt.Printf("token for %q: %s\n", agent, tok)
		if ns := f["brain"]; ns != "" {
			if _, code, _ := cl.do("POST", "/api/brain/grant", nil, map[string]any{
				"agentId": agent, "namespace": ns, "canRead": true, "canWrite": true}); code < 400 {
				fmt.Printf("granted read+write on brain %q\n", ns)
			} else {
				fmt.Printf("(warning: grant on %q failed)\n", ns)
			}
		}
		return nil
	}
	return fmt.Errorf("unknown: zekra token %s", sub)
}

// zekra recall <brain> <query...>
func cmdRecall(args []string) error {
	pos, f := parseFlags(args)
	if len(pos) < 2 {
		return fmt.Errorf("usage: zekra recall <brain> <query...>")
	}
	cl := newClient(loadConfig())
	payload := map[string]any{"namespace": pos[0], "query": strings.Join(pos[1:], " ")}
	if f["limit"] != "" {
		if n, err := strconv.Atoi(f["limit"]); err == nil {
			payload["limit"] = n
		}
	}
	m, code, err := cl.do("POST", "/api/brain/recall", nil, payload)
	if err != nil {
		return err
	}
	if code >= 400 {
		return apiErr(m, code)
	}
	res, _ := m["results"].([]any)
	if len(res) == 0 {
		fmt.Println("(no memories — the brain has no recall for that query)")
		return nil
	}
	for _, r := range res {
		rm, _ := r.(map[string]any)
		content := fmt.Sprint(rm["content"])
		content = strings.ReplaceAll(content, "\n", " ")
		if len(content) > 280 {
			content = content[:280] + "…"
		}
		fmt.Printf("• [%.2f] %s\n", asFloat(rm["score"]), content)
	}
	return nil
}

// zekra retain <brain> <content...>
func cmdRetain(args []string) error {
	pos, f := parseFlags(args)
	if len(pos) < 2 {
		return fmt.Errorf("usage: zekra retain <brain> <content...>")
	}
	cl := newClient(loadConfig())
	payload := map[string]any{"namespace": pos[0], "content": strings.Join(pos[1:], " ")}
	if f["source"] != "" {
		payload["sourceKind"] = f["source"]
	}
	m, code, err := cl.do("POST", "/api/brain/retain", nil, payload)
	if err != nil {
		return err
	}
	if code >= 400 {
		return apiErr(m, code)
	}
	fmt.Printf("retained: %v (%v)\n", m["id"], m["decision"])
	return nil
}

// --- small helpers ------------------------------------------------------------

func mask(t string) string {
	if t == "" {
		return "(none)"
	}
	if len(t) <= 12 {
		return t[:4] + "…"
	}
	return t[:12] + "…"
}

func asFloat(v any) float64 {
	if f, ok := v.(float64); ok {
		return f
	}
	return 0
}

func indent(s string) string {
	return "    " + strings.ReplaceAll(s, "\n", "\n    ")
}

// jsonSnippet renders the mcpServers entry a JSON-based client expects.
func jsonSnippet(name, apiURL, token, ns string) string {
	env := map[string]string{"ZEKRA_API_URL": apiURL, "ZEKRA_TOKEN": token}
	if ns != "" {
		env["ZEKRA_DEFAULT_NAMESPACE"] = ns
	}
	entry := map[string]any{
		"mcpServers": map[string]any{
			name: map[string]any{"command": "zekra", "args": []string{"mcp"}, "env": env},
		},
	}
	return pretty(entry)
}

var _ = url.Values{} // keep net/url imported for future query use
