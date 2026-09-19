package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
)

const protocolVersion = "2024-11-05"

// runMCP serves the Model Context Protocol over stdio (JSON-RPC 2.0, newline
// framed) — a thin adapter over the Zekra REST API, so every scoping/validation
// decision stays server-side. This is what `zekra mcp` runs and what MCP hosts
// (Claude Desktop, Claude Code, Codex, Gemini, Cursor) invoke.
func runMCP(cfg Config) {
	m := &mcp{cl: newClient(cfg), defaultNS: cfg.Namespace, out: json.NewEncoder(os.Stdout)}
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
		m.reply(req.ID, map[string]any{"tools": toolDefs})
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
	if json.Unmarshal(req.Params, &p) != nil {
		m.fail(req.ID, -32602, "invalid params")
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

	var (
		body map[string]any
		code int
		err  error
	)
	switch p.Name {
	case "memory_retain":
		body, code, err = m.cl.do("POST", "/api/brain/retain", nil, compact(map[string]any{
			"namespace": a["namespace"], "content": a["content"], "sourceKind": a["source_kind"],
			"sourceRef": a["source_ref"], "visibility": a["visibility"], "importanceHint": a["importance_hint"]}))
	case "memory_recall":
		body, code, err = m.cl.do("POST", "/api/brain/recall", nil, compact(map[string]any{
			"namespace": a["namespace"], "query": a["query"], "limit": a["limit"],
			"expandEntity": a["expand_entities"], "minImportance": a["min_importance"]}))
	case "memory_get":
		body, code, err = m.cl.do("GET", "/api/brain/memory", url.Values{
			"namespace": {str(a["namespace"])}, "id": {str(a["id"])}}, nil)
	case "memory_forget":
		body, code, err = m.cl.do("POST", "/api/brain/forget", nil, compact(map[string]any{
			"namespace": a["namespace"], "id": a["id"], "reason": a["reason"]}))
	case "memory_edit":
		body, code, err = m.cl.do("POST", "/api/brain/memory/edit", nil, compact(map[string]any{
			"namespace": a["namespace"], "id": a["id"], "content": a["content"],
			"importance": a["importance"], "metadata": a["metadata"]}))
	case "memory_share":
		body, code, err = m.cl.do("POST", "/api/brain/share", nil, compact(map[string]any{
			"namespace": a["namespace"], "granteeAgentId": a["grantee_agent_id"],
			"canRead": a["can_read"], "canWrite": a["can_write"]}))
	case "memory_gaps":
		q := url.Values{}
		for _, k := range []string{"namespace", "status", "limit"} {
			if v := str(a[k]); v != "" {
				q.Set(k, v)
			}
		}
		body, code, err = m.cl.do("GET", "/api/brain/gaps", q, nil)
	case "memory_resolve_gap":
		body, code, err = m.cl.do("POST", "/api/brain/gaps/resolve", nil, compact(map[string]any{
			"id": a["id"], "status": a["status"], "resolution": a["resolution"]}))
	case "brain_list":
		body, code, err = m.cl.do("GET", "/api/brain/namespaces", nil, nil)
	case "brain_details":
		body, code, err = m.cl.do("GET", "/api/brain/brain", url.Values{"namespace": {str(a["namespace"])}}, nil)
	case "brain_create":
		// Convenience: no server-side create endpoint exists (brains are
		// enumerated from stored memories), so seed a genesis marker.
		ns := str(a["name"])
		if ns == "" {
			ns = str(a["namespace"])
		}
		desc := str(a["description"])
		body, code, err = m.cl.do("POST", "/api/brain/retain", nil, compact(map[string]any{
			"namespace":      ns,
			"content":        strings.TrimSpace(fmt.Sprintf("Brain %q created via MCP. %s", ns, desc)),
			"sourceKind":     "system",
			"sourceRef":      "zekra-cli/brain-create",
			"importanceHint": 0.9}))
	case "brain_delete":
		// The API guard is confirm == namespace; accept a boolean true from the
		// agent and translate it to the namespace so the tool is ergonomic.
		confirm := a["confirm"]
		if b, ok := confirm.(bool); ok && b {
			confirm = a["namespace"]
		}
		body, code, err = m.cl.do("POST", "/api/brain/brain/delete", nil, compact(map[string]any{
			"namespace": a["namespace"], "confirm": confirm}))
	case "brain_grant":
		body, code, err = m.cl.do("POST", "/api/brain/grant", nil, compact(map[string]any{
			"agentId": a["agentId"], "namespace": a["namespace"], "canRead": a["canRead"], "canWrite": a["canWrite"]}))
	case "brain_revoke_grant":
		body, code, err = m.cl.do("POST", "/api/brain/grant/revoke", nil, compact(map[string]any{
			"agentId": a["agentId"], "namespace": a["namespace"]}))
	case "brain_create_token":
		body, code, err = m.cl.do("POST", "/api/brain/tokens", nil, compact(map[string]any{
			"agentId": a["agentId"], "label": a["label"], "isAdmin": a["isAdmin"]}))
	case "brain_tokens":
		q := url.Values{}
		if b, _ := a["includeRevoked"].(bool); b {
			q.Set("includeRevoked", "1")
		}
		body, code, err = m.cl.do("GET", "/api/brain/tokens", q, nil)
	case "brain_chat":
		body, code, err = m.cl.do("POST", "/api/brain/chat", nil, compact(map[string]any{
			"namespace": a["namespace"], "message": a["message"], "topK": a["topK"]}))
	case "secret_list":
		body, code, err = m.cl.do("GET", "/api/brain/secrets", url.Values{"namespace": {str(a["namespace"])}}, nil)
	case "secret_store":
		body, code, err = m.cl.do("POST", "/api/brain/secrets", nil, compact(map[string]any{
			"namespace": a["namespace"], "name": a["name"], "value": a["value"], "kind": a["kind"]}))
	case "secret_reveal":
		body, code, err = m.cl.do("POST", "/api/brain/secrets/reveal", nil, compact(map[string]any{
			"namespace": a["namespace"], "name": a["name"]}))
	case "secret_delete":
		body, code, err = m.cl.do("POST", "/api/brain/secrets/delete", nil, compact(map[string]any{
			"namespace": a["namespace"], "name": a["name"]}))
	case "datasource_list":
		body, code, err = m.cl.do("GET", "/api/brain/datasources", url.Values{"namespace": {str(a["namespace"])}}, nil)
	case "datasource_create":
		body, code, err = m.cl.do("POST", "/api/brain/datasources", nil, compact(map[string]any{
			"namespace": a["namespace"], "kind": a["kind"], "name": a["name"], "config": a["config"]}))
	case "datasource_sync":
		body, code, err = m.cl.do("POST", "/api/brain/datasources/sync", nil, map[string]any{"id": a["id"]})
	case "datasource_delete":
		body, code, err = m.cl.do("POST", "/api/brain/datasources/delete", nil, map[string]any{"id": a["id"]})
	default:
		m.fail(req.ID, -32602, "unknown tool: "+p.Name)
		return
	}

	if err != nil {
		m.toolResult(req.ID, map[string]any{"error": map[string]string{"code": "unavailable", "message": err.Error()}}, true)
		return
	}
	m.toolResult(req.ID, body, code >= 400)
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

func str(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

var _ = io.EOF // keep io imported for the framed reader contract
