package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const protocolVersion = "2024-11-05"

// runMCP serves the Model Context Protocol over stdio (JSON-RPC 2.0, newline
// framed) — a thin adapter over the Zekra REST API, so every scoping/validation
// decision stays server-side. This is what `zekra mcp` runs and what MCP hosts
// (Claude Desktop, Claude Code, Codex, Gemini, Cursor) invoke.
//
// The tool set and the argument → REST mapping mirror the server's shared
// package (github.com/togo-framework/brain/mcptools).
func runMCP(cfg Config) {
	cl := newClient(cfg)
	m := &mcp{cl: cl, defaultNS: cfg.Namespace, out: json.NewEncoder(os.Stdout), tools: advertisedTools(cl)}
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var req rpcReq
		if json.Unmarshal(line, &req) != nil {
			continue
		}
		m.dispatch(&req)
	}
}

type rpcReq struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}
type rpcResp struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcErr         `json:"error,omitempty"`
}
type rpcErr struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type mcp struct {
	cl        *client
	defaultNS string
	out       *json.Encoder
	tools     []map[string]any
}

func (m *mcp) reply(id json.RawMessage, res any) {
	_ = m.out.Encode(rpcResp{JSONRPC: "2.0", ID: id, Result: res})
}
func (m *mcp) fail(id json.RawMessage, code int, msg string) {
	_ = m.out.Encode(rpcResp{JSONRPC: "2.0", ID: id, Error: &rpcErr{Code: code, Message: msg}})
}

func (m *mcp) dispatch(req *rpcReq) {
	switch req.Method {
	case "initialize":
		m.reply(req.ID, map[string]any{
			"protocolVersion": protocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "zekra", "version": version},
		})
	case "notifications/initialized", "notifications/cancelled":
		// notifications get no response
	case "ping":
		m.reply(req.ID, map[string]any{})
	case "tools/list":
		m.reply(req.ID, map[string]any{"tools": m.tools})
	case "tools/call":
		m.callTool(req)
	default:
		if len(req.ID) > 0 {
			m.fail(req.ID, -32601, "method not found: "+req.Method)
		}
	}
}

func (m *mcp) callTool(req *rpcReq) {
	var p struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if json.Unmarshal(req.Params, &p) != nil || p.Name == "" {
		m.fail(req.ID, -32602, "invalid params")
		return
	}
	if !knownTool(p.Name) {
		m.fail(req.ID, -32602, "unknown tool: "+p.Name)
		return
	}
	a := map[string]any{}
	if len(p.Arguments) > 0 {
		_ = json.Unmarshal(p.Arguments, &a)
	}
	// Default the namespace when the session is bound to one brain.
	if m.defaultNS != "" {
		if v, ok := a["namespace"]; !ok || v == nil || v == "" {
			a["namespace"] = m.defaultNS
		}
	}
	payload, isErr := m.call(p.Name, a)
	m.toolResult(req.ID, payload, isErr)
}

// call translates one tool call into its REST request — the same mapping as the
// server's mcptools.Call (plugins/brain/mcptools/dispatch.go).
func (m *mcp) call(name string, a map[string]any) (any, bool) {
	var (
		body any
		code int
		err  error
	)
	post := func(path string, payload map[string]any) {
		body, code, err = m.cl.doAny("POST", path, nil, compact(payload), nil)
	}
	get := func(path string, q url.Values) { body, code, err = m.cl.doAny("GET", path, q, nil, nil) }
	note := func(suffix string) string { return "/api/notes/" + url.PathEscape(str(a["id"])) + suffix }

	switch name {
	case "memory_retain":
		post("/api/brain/retain", map[string]any{
			"namespace": a["namespace"], "content": a["content"], "sourceKind": a["source_kind"],
			"sourceRef": a["source_ref"], "visibility": a["visibility"], "importanceHint": a["importance_hint"]})
	case "memory_recall":
		post("/api/brain/recall", map[string]any{
			"namespace": a["namespace"], "query": a["query"], "limit": a["limit"],
			"expandEntity": a["expand_entities"], "types": a["types"],
			"excludeSourceKinds": a["exclude_source_kinds"], "minImportance": a["min_importance"]})
	case "memory_recall_archive":
		// Phase 2 on the server too: cold-tier deep recall is stubbed.
		return map[string]any{"error": map[string]string{"code": "unavailable",
			"message": "memory_recall_archive: cold tier not yet provisioned (Phase 2)"}}, false
	case "memory_get":
		get("/api/brain/memory", url.Values{"namespace": {str(a["namespace"])}, "id": {str(a["id"])}})
	case "memory_dedup":
		post("/api/brain/dedup", map[string]any{"namespace": a["namespace"], "sourceKind": a["sourceKind"]})
	case "memory_forget":
		post("/api/brain/forget", map[string]any{"namespace": a["namespace"], "id": a["id"], "reason": a["reason"]})
	case "memory_share":
		post("/api/brain/share", map[string]any{
			"namespace": a["namespace"], "granteeAgentId": a["grantee_agent_id"],
			"canRead": a["can_read"], "canWrite": a["can_write"]})
	case "graph_traverse":
		post("/api/brain/graph/traverse", map[string]any{
			"namespace": a["namespace"], "entity": a["entity"], "depth": a["depth"],
			"relations": a["relations"], "types": a["types"],
			"direction": a["direction"], "asOf": a["asOf"], "limit": a["limit"]})
	case "graph_spine":
		post("/api/brain/graph/spine", map[string]any{
			"namespace": a["namespace"], "entity": a["entity"], "depth": a["depth"],
			"hubs": a["hubs"], "roles": a["roles"], "perGroup": a["perGroup"],
			"window": a["window"], "since": a["since"], "until": a["until"], "timeRoles": a["timeRoles"]})
	case "graph_neighbors":
		post("/api/brain/graph/neighbors", map[string]any{
			"namespace": a["namespace"], "entity": a["entity"], "asOf": a["asOf"]})
	case "graph_path":
		post("/api/brain/graph/path", map[string]any{
			"namespace": a["namespace"], "from": a["from"], "to": a["to"], "maxDepth": a["maxDepth"]})
	case "graph_ontology":
		get("/api/brain/graph/ontology", url.Values{"namespace": {str(a["namespace"])}})
	case "memory_gaps":
		get("/api/brain/gaps", nonEmpty(url.Values{
			"namespace": {str(a["namespace"])}, "status": {str(a["status"])}, "limit": {str(a["limit"])}}))
	case "memory_resolve_gap":
		post("/api/brain/gaps/resolve", map[string]any{"id": a["id"], "status": a["status"], "resolution": a["resolution"]})
	case "brain_list":
		get("/api/brain/namespaces", nil)
	case "brain_details":
		get("/api/brain/brain", url.Values{"namespace": {str(a["namespace"])}})
	case "memory_edit":
		post("/api/brain/memory/edit", map[string]any{
			"namespace": a["namespace"], "id": a["id"], "content": a["content"],
			"importance": a["importance"], "metadata": a["metadata"]})
	case "brain_create":
		ns := a["namespace"]
		if str(ns) == "" {
			ns = a["name"] // accept the older CLI argument name
		}
		post("/api/brain/brains", map[string]any{"namespace": ns, "description": a["description"]})
	case "brain_delete":
		// Server guard: confirm must be the namespace string itself.
		post("/api/brain/brain/delete", map[string]any{"namespace": a["namespace"], "confirm": a["confirm"]})
	case "brain_grant":
		post("/api/brain/grant", map[string]any{
			"agentId": a["agentId"], "namespace": a["namespace"], "canRead": a["canRead"], "canWrite": a["canWrite"]})
	case "brain_revoke_grant":
		post("/api/brain/grant/revoke", map[string]any{"agentId": a["agentId"], "namespace": a["namespace"]})
	case "brain_create_token":
		post("/api/brain/tokens", map[string]any{"agentId": a["agentId"], "label": a["label"], "isAdmin": a["isAdmin"]})
	case "brain_tokens":
		q := url.Values{}
		if b, _ := a["includeRevoked"].(bool); b {
			q.Set("includeRevoked", "1")
		}
		get("/api/brain/tokens", q)
	case "brain_chat":
		post("/api/brain/chat", map[string]any{"namespace": a["namespace"], "message": a["message"], "topK": a["topK"]})
	case "secret_list":
		get("/api/brain/secrets", url.Values{"namespace": {str(a["namespace"])}})
	case "secret_store":
		post("/api/brain/secrets", map[string]any{
			"namespace": a["namespace"], "name": a["name"], "value": a["value"], "kind": a["kind"]})
	case "secret_reveal":
		post("/api/brain/secrets/reveal", map[string]any{"namespace": a["namespace"], "name": a["name"]})
	case "secret_delete":
		post("/api/brain/secrets/delete", map[string]any{"namespace": a["namespace"], "name": a["name"]})
	case "datasource_list":
		get("/api/brain/datasources", url.Values{"namespace": {str(a["namespace"])}})
	case "datasource_create":
		post("/api/brain/datasources", map[string]any{
			"namespace": a["namespace"], "kind": a["kind"], "name": a["name"], "config": a["config"]})
	case "datasource_sync":
		post("/api/brain/datasources/sync", map[string]any{"id": a["id"]})
	case "datasource_delete":
		post("/api/brain/datasources/delete", map[string]any{"id": a["id"]})

	// --- notes (/api/notes) ---------------------------------------------------
	case "note_create":
		post("/api/notes", map[string]any{
			"namespace": a["namespace"], "title": a["title"], "body": a["body"],
			"tags": a["tags"], "pinned": a["pinned"], "source": "agent"})
	case "note_update":
		var hdr map[string]string
		if v := str(a["version"]); v != "" {
			hdr = map[string]string{"If-Match": v}
		}
		body, code, err = m.cl.doAny("PUT", note(""), nil, compact(map[string]any{
			"title": a["title"], "body": a["body"], "tags": a["tags"], "pinned": a["pinned"],
			"archived": a["archived"], "version": a["version"], "source": "agent"}), hdr)
	case "note_append":
		post(note("/append"), map[string]any{"text": a["text"], "source": "agent"})
		if err == nil && (code == http.StatusNotFound || code == http.StatusMethodNotAllowed) {
			body, code, err = m.appendByPut(note(""), str(a["text"]))
		}
	case "note_get":
		get(note(""), nil)
	case "note_search":
		limit := str(a["limit"])
		if limit == "" {
			limit = "20"
		}
		get("/api/notes", nonEmpty(url.Values{
			"q": {str(a["query"])}, "namespace": {str(a["namespace"])},
			"tag": {str(a["tag"])}, "since": {str(a["since"])}, "limit": {limit}}))
	case "note_list":
		q := nonEmpty(url.Values{
			"namespace": {str(a["namespace"])}, "tag": {str(a["tag"])}, "since": {str(a["since"])},
			"limit": {str(a["limit"])}, "cursor": {str(a["cursor"])}})
		if v, _ := a["archived"].(bool); v {
			q.Set("archived", "1")
		}
		get("/api/notes", q)
	case "note_delete":
		body, code, err = m.cl.doAny("DELETE", note(""), nil, nil, nil)
	default:
		return map[string]any{"error": map[string]string{"code": "invalid_argument", "message": "unknown tool: " + name}}, true
	}
	if err != nil {
		return map[string]any{"error": map[string]string{"code": "unavailable", "message": err.Error()}}, true
	}
	return body, code >= 400
}

// appendByPut is note_append for a server without POST /api/notes/{id}/append:
// read the note, then PUT the extended body guarded by If-Match: <version>, so a
// concurrent edit returns 409 instead of being overwritten.
func (m *mcp) appendByPut(path, text string) (any, int, error) {
	cur, code, err := m.cl.doAny("GET", path, nil, nil, nil)
	if err != nil || code >= 400 {
		return cur, code, err
	}
	n, _ := cur.(map[string]any)
	if inner, ok := n["note"].(map[string]any); ok {
		n = inner
	}
	b := strings.TrimRight(str(n["body"]), "\n")
	if b != "" {
		b += "\n\n"
	}
	b += text
	payload := map[string]any{"body": b, "source": "agent"}
	var hdr map[string]string
	if v := str(n["version"]); v != "" {
		payload["version"] = n["version"]
		hdr = map[string]string{"If-Match": v}
	}
	return m.cl.doAny("PUT", path, nil, payload, hdr)
}

// advertisedTools picks the tools/list payload: the server's live list from
// GET /api/mcp/tools when reachable (so new server schemas appear without a CLI
// release), restricted to tools this CLI can dispatch; otherwise the built-in list.
func advertisedTools(cl *client) []map[string]any {
	builtin := builtinTools()
	c := *cl
	c.hc = &http.Client{Timeout: 3 * time.Second}
	body, code, err := c.doAny("GET", "/api/mcp/tools", nil, nil, nil)
	if err != nil || code != http.StatusOK {
		return builtin
	}
	var list []any
	switch v := body.(type) {
	case []any:
		list = v
	case map[string]any:
		list, _ = v["tools"].([]any)
	}
	var out []map[string]any
	for _, t := range list {
		tm, ok := t.(map[string]any)
		if !ok {
			continue
		}
		name, _ := tm["name"].(string)
		if _, schema := tm["inputSchema"].(map[string]any); name == "" || !schema || !knownTool(name) {
			continue
		}
		out = append(out, tm)
	}
	if len(out) == 0 {
		return builtin
	}
	return out
}

// doAny is client.do for MCP: any JSON body shape (arrays included) and optional
// extra headers (If-Match). Sends X-Zekra-Token plus legacy X-Cabrain-Token.
func (c *client) doAny(method, path string, q url.Values, payload map[string]any, hdr map[string]string) (any, int, error) {
	var rd io.Reader
	if payload != nil {
		b, _ := json.Marshal(payload)
		rd = bytes.NewReader(b)
	}
	u := strings.TrimRight(c.base, "/") + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequest(method, u, rd)
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
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	var body any
	if len(raw) > 0 && json.Unmarshal(raw, &body) != nil {
		body = map[string]any{"raw": string(raw)}
	}
	return body, resp.StatusCode, nil
}

func (m *mcp) toolResult(id json.RawMessage, payload any, isErr bool) {
	b, _ := json.MarshalIndent(payload, "", "  ")
	m.reply(id, map[string]any{
		"content": []map[string]any{{"type": "text", "text": string(b)}},
		"isError": isErr,
	})
}

// compact drops nil values so absent optional args don't override server defaults.
func compact(mp map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range mp {
		if v != nil {
			out[k] = v
		}
	}
	return out
}

func nonEmpty(q url.Values) url.Values {
	for k, v := range q {
		if len(v) == 0 || strings.TrimSpace(v[0]) == "" {
			delete(q, k)
		}
	}
	return q
}

func str(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case float64:
		if t == float64(int64(t)) {
			return fmt.Sprintf("%d", int64(t))
		}
	}
	return fmt.Sprint(v)
}
