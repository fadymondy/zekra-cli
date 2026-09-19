package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// The `zekra hook <event>` commands back the bundled Claude Code plugin hooks.
// They read the hook JSON on stdin and emit hookSpecificOutput.additionalContext
// on stdout — so the memory-first discipline and auto-recall inject themselves into
// a session with NO jq/python dependency (the Go binary does the parsing). Every
// hook FAILS OPEN: any error → exit 0 with no output, so a hook never blocks the user.

// memoryFirstRules is injected at SessionStart (unless disabled). Keep it tight —
// it goes into every session's context.
const memoryFirstRules = `Zekra memory-first (this session has a Zekra brain via MCP tools: memory_recall, memory_retain, brain_list, brain_details, …). Operate recall → answer/act → retain:
1. RECALL FIRST. Before answering or acting on anything touching durable knowledge (a person, project, venture, decision, issue, learning, or a who/what/why), call memory_recall with a concise, keyword-forward query — even if you think you already know. The brain is the source of truth.
2. ANSWER FROM MEMORY, and cite it. If recall returns nothing relevant, say so plainly — never invent facts to fill the gap.
3. RETAIN WHAT'S NEW. After producing something durable (a decision + its rationale, a correction, a learned constraint/gotcha, a new fact, an interface detail), call memory_retain with a distilled sentence or two. The write-decision de-dupes.
Prefer the brain over asking the user to repeat facts it likely already holds.`

// firstEnv returns the first non-empty value among the named env vars.
func firstEnv(names ...string) string {
	for _, n := range names {
		if v := strings.TrimSpace(os.Getenv(n)); v != "" {
			return v
		}
	}
	return ""
}

// envOnAny is true if any named env var is truthy; def when all are unset.
func envOnAny(def bool, names ...string) bool {
	v := strings.ToLower(firstEnv(names...))
	if v == "" {
		return def
	}
	return v == "1" || v == "true" || v == "on" || v == "yes"
}

// emitContext prints the hookSpecificOutput JSON that injects `ctx` into the
// conversation for the given hook event.
func emitContext(event, ctx string) {
	out := map[string]any{
		"hookSpecificOutput": map[string]any{
			"hookEventName":     event,
			"additionalContext": ctx,
		},
	}
	b, _ := json.Marshal(out)
	fmt.Println(string(b))
}

// cmdHook dispatches `zekra hook <event>`. It always exits 0 (fail-open).
func cmdHook(args []string) error {
	if len(args) == 0 {
		return nil
	}
	switch args[0] {
	case "rules", "session-start":
		hookRules()
	case "recall", "user-prompt":
		hookRecall()
	default:
		// unknown event → no-op, never error out a hook
	}
	return nil
}

// hookRules injects the memory-first discipline at SessionStart.
// Disable with ZEKRA_HOOK_RULES=0.
func hookRules() {
	if !envOnAny(true, "ZEKRA_HOOK_RULES", "CABRAIN_HOOK_RULES", "CLAUDE_PLUGIN_OPTION_INJECT_RULES") {
		return
	}
	emitContext("SessionStart", memoryFirstRules)
}

// hookRecall auto-recalls relevant memories for the submitted prompt and injects
// them at UserPromptSubmit. OPT-IN: set ZEKRA_HOOK_AUTORECALL=1 and a brain via
// ZEKRA_AUTORECALL_BRAIN (or ZEKRA_DEFAULT_NAMESPACE / saved config).
func hookRecall() {
	if !envOnAny(false, "ZEKRA_HOOK_AUTORECALL", "CABRAIN_HOOK_AUTORECALL", "CLAUDE_PLUGIN_OPTION_AUTO_RECALL") {
		return
	}
	cfg := loadConfig()
	// Fall back to the plugin's userConfig (exported as CLAUDE_PLUGIN_OPTION_*) when
	// the CLI wasn't separately logged in via `zekra auth login`.
	if cfg.Token == "" {
		cfg.Token = firstEnv("CLAUDE_PLUGIN_OPTION_API_TOKEN")
	}
	if u := firstEnv("CLAUDE_PLUGIN_OPTION_API_URL"); u != "" && (cfg.URL == "" || cfg.URL == defaultURL) {
		cfg.URL = strings.TrimRight(u, "/")
	}
	ns := firstEnv("ZEKRA_AUTORECALL_BRAIN", "CABRAIN_AUTORECALL_BRAIN", "CLAUDE_PLUGIN_OPTION_DEFAULT_NAMESPACE")
	if ns == "" {
		ns = cfg.Namespace
	}
	if ns == "" || cfg.Token == "" {
		return // nothing to recall against, or not authenticated → stay silent
	}

	// Read the hook payload and pull out the prompt.
	raw, err := io.ReadAll(io.LimitReader(os.Stdin, 1<<20))
	if err != nil || len(raw) == 0 {
		return
	}
	var in struct {
		Prompt string `json:"prompt"`
	}
	if json.Unmarshal(raw, &in) != nil || strings.TrimSpace(in.Prompt) == "" {
		return
	}

	// Short-timeout client so the hook never delays the prompt noticeably.
	cl := newClient(cfg)
	cl.hc.Timeout = 8 * time.Second
	body, code, err := cl.do("POST", "/api/brain/recall", nil, map[string]any{
		"namespace": ns, "query": in.Prompt, "limit": 5})
	if err != nil || code >= 400 || body == nil {
		return
	}
	results, _ := body["results"].([]any)
	if len(results) == 0 {
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Relevant Zekra memories (brain=%q, auto-recalled for this prompt). Ground your answer in these and cite them; if they don't cover it, recall again or say so:\n", ns)
	for _, r := range results {
		rm, _ := r.(map[string]any)
		c := strings.ReplaceAll(fmt.Sprint(rm["content"]), "\n", " ")
		if len(c) > 300 {
			c = c[:300] + "…"
		}
		fmt.Fprintf(&b, "- %s\n", c)
	}
	emitContext("UserPromptSubmit", b.String())
}
