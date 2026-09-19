package main

// toolDefs is the tools/list payload — the memory organ surface exposed to any
// MCP host. Schemas mirror the Zekra REST contract (contracts/tools.md).
type prop map[string]any

func obj(required []string, props map[string]any) map[string]any {
	// JSON Schema requires `required` to be an array — never null. Tools with no
	// required fields must emit [], or strict MCP clients (Claude Code's Zod
	// validator) reject the whole tools/list with "expected array, received null".
	if required == nil {
		required = []string{}
	}
	return map[string]any{"type": "object", "properties": props, "required": required}
}

var toolDefs = []map[string]any{
	{
		"name":        "memory_recall",
		"description": "Hybrid recall (vector + BM25 fused with RRF, reranked, 1-hop entity expansion). Read the brain BEFORE answering.",
		"inputSchema": obj([]string{"namespace", "query"}, map[string]any{
			"namespace":       prop{"type": "string", "description": "brain to read"},
			"query":           prop{"type": "string", "description": "keyword-forward query"},
			"limit":           prop{"type": "number", "description": "max results (default 8)"},
			"expand_entities": prop{"type": "boolean"},
			"min_importance":  prop{"type": "number"},
		}),
	},
	{
		"name":        "memory_retain",
		"description": "Store a memory (runs the ADD/UPDATE/INVALIDATE/NOOP write-decision, embeds + BM25 + entity graph). Retain what's new.",
		"inputSchema": obj([]string{"namespace", "content"}, map[string]any{
			"namespace":       prop{"type": "string"},
			"content":         prop{"type": "string"},
			"source_kind":     prop{"type": "string"},
			"source_ref":      prop{"type": "string"},
			"visibility":      prop{"type": "string"},
			"importance_hint": prop{"type": "number", "description": "[0,1] salience flag, blended not authoritative"},
		}),
	},
	{
		"name":        "memory_get",
		"description": "Fetch one memory by id with full provenance.",
		"inputSchema": obj([]string{"namespace", "id"}, map[string]any{
			"namespace": prop{"type": "string"}, "id": prop{"type": "string"}}),
	},
	{
		"name":        "memory_forget",
		"description": "Invalidate a memory by id (soft-delete; stays for archive recall).",
		"inputSchema": obj([]string{"namespace", "id"}, map[string]any{
			"namespace": prop{"type": "string"}, "id": prop{"type": "string"}, "reason": prop{"type": "string"}}),
	},
	{
		"name":        "memory_edit",
		"description": "Edit a memory by id — change content (re-embeds), importance, or metadata.",
		"inputSchema": obj([]string{"namespace", "id"}, map[string]any{
			"namespace": prop{"type": "string"}, "id": prop{"type": "string"}, "content": prop{"type": "string"},
			"importance": prop{"type": "number"}, "metadata": prop{"type": "object"}}),
	},
	{
		"name":        "memory_gaps",
		"description": "List knowledge gaps (queries that recalled nothing) for a brain.",
		"inputSchema": obj(nil, map[string]any{
			"namespace": prop{"type": "string"},
			"status":    prop{"type": "string", "enum": []string{"open", "indexed", "dismissed", "all"}},
			"limit":     prop{"type": "number"}}),
	},
	{
		"name":        "memory_resolve_gap",
		"description": "Resolve a knowledge gap after indexing the missing knowledge, or dismiss it.",
		"inputSchema": obj([]string{"id", "status"}, map[string]any{
			"id": prop{"type": "number"}, "status": prop{"type": "string", "enum": []string{"indexed", "dismissed", "open"}},
			"resolution": prop{"type": "string"}}),
	},
	{
		"name":        "brain_list",
		"description": "List brains (namespaces) this token can read, with memory counts.",
		"inputSchema": obj(nil, map[string]any{}),
	},
	{
		"name":        "brain_details",
		"description": "Detail for one brain: memory count, breakdown by type/source, open gaps, recall activity.",
		"inputSchema": obj([]string{"namespace"}, map[string]any{"namespace": prop{"type": "string"}}),
	},
	{
		"name":        "brain_create",
		"description": "Create a new empty named brain (seeds a genesis marker so the namespace exists and is connectable).",
		"inputSchema": obj([]string{"name"}, map[string]any{
			"name":        prop{"type": "string", "description": "the new brain's namespace"},
			"description": prop{"type": "string"}}),
	},
	{
		"name":        "brain_delete",
		"description": "Delete a brain and ALL its memories. Requires confirm=true.",
		"inputSchema": obj([]string{"namespace", "confirm"}, map[string]any{
			"namespace": prop{"type": "string"}, "confirm": prop{"type": "boolean"}}),
	},
	{
		"name":        "brain_grant",
		"description": "ACL (admin): grant an agent/token read and/or write on a brain.",
		"inputSchema": obj([]string{"agentId", "namespace"}, map[string]any{
			"agentId": prop{"type": "string"}, "namespace": prop{"type": "string"},
			"canRead": prop{"type": "boolean"}, "canWrite": prop{"type": "boolean"}}),
	},
	{
		"name":        "brain_revoke_grant",
		"description": "ACL (admin): revoke an agent's grant on a brain.",
		"inputSchema": obj([]string{"agentId", "namespace"}, map[string]any{
			"agentId": prop{"type": "string"}, "namespace": prop{"type": "string"}}),
	},
	{
		"name":        "brain_create_token",
		"description": "ACL (admin): mint an access token for an agent identity (optionally admin). The holder sets ZEKRA_TOKEN.",
		"inputSchema": obj([]string{"agentId"}, map[string]any{
			"agentId": prop{"type": "string"}, "label": prop{"type": "string"}, "isAdmin": prop{"type": "boolean"}}),
	},
	{
		"name":        "brain_tokens",
		"description": "ACL (admin): list access tokens with their per-brain grants.",
		"inputSchema": obj(nil, map[string]any{"includeRevoked": prop{"type": "boolean"}}),
	},
	{
		"name":        "brain_chat",
		"description": "Ask the brain a question in natural language (RAG over one namespace).",
		"inputSchema": obj([]string{"namespace", "message"}, map[string]any{
			"namespace": prop{"type": "string"}, "message": prop{"type": "string"}, "topK": prop{"type": "number"}}),
	},
	{
		"name":        "secret_list",
		"description": "List a brain's secret names (values stay hidden).",
		"inputSchema": obj([]string{"namespace"}, map[string]any{"namespace": prop{"type": "string"}}),
	},
	{
		"name":        "secret_store",
		"description": "Store/replace a secret in a brain's vault (AES-256, write access required).",
		"inputSchema": obj([]string{"namespace", "name", "value"}, map[string]any{
			"namespace": prop{"type": "string"}, "name": prop{"type": "string"}, "value": prop{"type": "string"}, "kind": prop{"type": "string"}}),
	},
	{
		"name":        "secret_reveal",
		"description": "Decrypt and return a secret value (write/admin on the brain required).",
		"inputSchema": obj([]string{"namespace", "name"}, map[string]any{
			"namespace": prop{"type": "string"}, "name": prop{"type": "string"}}),
	},
	{
		"name":        "secret_delete",
		"description": "Delete a secret from a brain's vault.",
		"inputSchema": obj([]string{"namespace", "name"}, map[string]any{
			"namespace": prop{"type": "string"}, "name": prop{"type": "string"}}),
	},
	{
		"name":        "datasource_list",
		"description": "List a brain's configured connectors (github/sql/crawler/markdown/webhook) with status + doc counts.",
		"inputSchema": obj([]string{"namespace"}, map[string]any{"namespace": prop{"type": "string"}}),
	},
	{
		"name":        "datasource_create",
		"description": "Create a connector bound to a brain. On sync it pulls docs, chunks, and retains them.",
		"inputSchema": obj([]string{"namespace", "kind", "name"}, map[string]any{
			"namespace": prop{"type": "string"},
			"kind":      prop{"type": "string", "enum": []string{"text", "markdown", "crawler", "github", "sql", "webhook"}},
			"name":      prop{"type": "string"}, "config": prop{"type": "object"}}),
	},
	{
		"name":        "datasource_sync",
		"description": "Run a connector now (pull + retain). Returns {ingested}.",
		"inputSchema": obj([]string{"id"}, map[string]any{"id": prop{"type": "string"}}),
	},
	{
		"name":        "datasource_delete",
		"description": "Delete a connector.",
		"inputSchema": obj([]string{"id"}, map[string]any{"id": prop{"type": "string"}}),
	},
}
