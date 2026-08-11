package emit

import (
	"fmt"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// MCPSchema picks the JSON shape used by the target tool.
type MCPSchema int

const (
	// MCPSchemaServersMap wraps entries under {"mcpServers": {...}}.
	// Used by Claude Code (.mcp.json) and Cursor (.cursor/mcp.json).
	MCPSchemaServersMap MCPSchema = iota
	// MCPSchemaVSCodeServers wraps entries under {"servers": {...}} and
	// requires a `type` field per server. Used by VS Code / GitHub Copilot.
	MCPSchemaVSCodeServers
	// MCPSchemaServersMapNoType is MCPSchemaServersMap without the remote
	// transport discriminant, for a vendor whose schema has no `type`
	// field at all and no other divergence from the standard shape. Warp
	// was originally the case: its remote-server table documents only
	// `url` and `headers`, and one tab covers "Streamable HTTP or SSE
	// Server (URL)", so the transport is never named in config (#592).
	//
	// Targets whose vendor omits `type` but whose entry shape also
	// diverges (antigravity's `serverUrl`, trae's field set, windsurf's
	// `transport` key) keep their own builder instead. This variant is for
	// the case where only the discriminant differs. Warp itself moved
	// into that group once its stdio table needed a `working_directory`
	// field mapped from the spec's `cwd` (#606), so no adapter currently
	// selects this variant; it stays available for the next vendor whose
	// schema omits `type` and diverges no further.
	MCPSchemaServersMapNoType
)

// WriteMCPFile renders mcps with the target schema and writes to path.
// No file is written when mcps is empty or every entry produces an empty
// server block. Adapters call this in lieu of hand-rolling the
// guard / render / write triple at every call site.
func (s *Session) WriteMCPFile(mcps []spec.Entry, schema MCPSchema, path string, dryRun bool) error {
	if len(mcps) == 0 {
		return nil
	}
	doc, err := MCPDocument(mcps, schema)
	if err != nil {
		return err
	}
	if doc == "" {
		return nil
	}
	return s.WriteFile(path, doc, dryRun)
}

// MCPDocument renders an MCP server config file from bundle MCP entries.
//
// Each entry's frontmatter accepts: type (stdio|http|sse, default stdio),
// command, args, env, url, headers, description, disabled, roots.
// Empty entries are skipped.
func MCPDocument(mcps []spec.Entry, schema MCPSchema) (string, error) {
	servers := map[string]map[string]any{}
	for _, e := range mcps {
		if e.Name == "" {
			continue
		}
		entry := buildServer(e, schema)
		if len(entry) == 0 {
			continue
		}
		servers[e.Name] = entry
	}
	if len(servers) == 0 {
		return "", nil
	}
	root := map[string]any{}
	switch schema {
	case MCPSchemaVSCodeServers:
		root["servers"] = servers
	default:
		root["mcpServers"] = servers
	}
	raw, err := MarshalJSONIndent(root)
	if err != nil {
		return "", fmt.Errorf("marshal mcp: %w", err)
	}
	return string(raw) + "\n", nil
}

func buildServer(e spec.Entry, schema MCPSchema) map[string]any {
	transport := stringField(e.Meta, "type")
	if transport == "" {
		transport = "stdio"
	}
	out := map[string]any{}

	switch transport {
	case "stdio":
		if cmd := stringField(e.Meta, "command"); cmd != "" {
			out["command"] = cmd
		}
		if args := stringSlice(e.Meta, "args"); len(args) > 0 {
			out["args"] = args
		}
	case "http", "sse", "ws":
		if url := stringField(e.Meta, "url"); url != "" {
			out["url"] = url
		}
		if h := mapField(e.Meta, "headers"); len(h) > 0 {
			out["headers"] = h
		}
		// Remote servers carry an explicit `type` in every schema. A
		// `.mcp.json` entry with only `url` is ambiguous (http vs sse
		// vs ws); stdio stays type-less since it is the inferred
		// default. `ws` takes the same url/headers shape as http per
		// https://code.claude.com/docs/en/mcp, and Qoder documents the
		// same transport. Before it shared this branch, a `ws` entry
		// matched no case at all and was written with no command, no
		// url, and no type: a malformed server object, emitted with no
		// warning.
		//
		// MCPSchemaServersMapNoType opts out: see its doc comment.
		if schema != MCPSchemaServersMapNoType {
			out["type"] = transport
		}
	}

	if env := mapField(e.Meta, "env"); len(env) > 0 {
		out["env"] = env
	}
	if desc := stringField(e.Meta, "description"); desc != "" {
		out["description"] = desc
	}
	if disabled, _ := e.Meta["disabled"].(bool); disabled {
		out["disabled"] = true
	}
	if roots := BuildRoots(e.Meta); len(roots) > 0 {
		out["roots"] = roots
	}
	if schema == MCPSchemaVSCodeServers {
		out["type"] = transport
	}
	return out
}

// StripMCPDisabled returns mcps with any `disabled: true` meta flag
// removed, and buffers one NoteFieldNoOp report counting how many
// entries carried it. Neither the input slice nor its Meta maps are
// mutated; entries without the flag pass through unchanged (and
// unaliased-but-identical) so callers can share one bundle across
// targets.
//
// Call this before WriteMCPFile for a target whose project-scoped MCP
// file has no working per-server disable key. Writing a dead `disabled:
// true` would let a user believe the server stopped connecting when the
// target ignores the key entirely and keeps using it — worse than
// omitting the field, since the spec now silently lies about the
// server's state. reason is the short, user-facing phrase used in the
// flushed note, e.g. "no file-based way to pre-disable a project-scoped
// MCP server".
func StripMCPDisabled(target string, mcps []spec.Entry, reason string) []spec.Entry {
	out := make([]spec.Entry, len(mcps))
	count := 0
	for i, e := range mcps {
		disabled, _ := e.Meta["disabled"].(bool)
		if !disabled {
			out[i] = e
			continue
		}
		count++
		meta := make(map[string]any, len(e.Meta))
		for k, v := range e.Meta {
			if k != "disabled" {
				meta[k] = v
			}
		}
		e.Meta = meta
		out[i] = e
	}
	NoteFieldNoOp(target, spec.KindMCP, "disabled", count, reason)
	return out
}

// BuildRoots constructs the `roots` array from spec meta. Each element is
// a map with at least a `uri` key; `name` is optional. Exported so an
// adapter with its own bespoke MCP builder (warp's mcp.go) can render
// the same `roots` shape as this shared builder without duplicating the
// field's parsing.
func BuildRoots(meta map[string]any) []map[string]any {
	raw, _ := meta["roots"].([]any)
	if len(raw) == 0 {
		return nil
	}
	roots := make([]map[string]any, 0, len(raw))
	for _, r := range raw {
		m, _ := r.(map[string]any)
		if m == nil {
			continue
		}
		entry := map[string]any{}
		if uri, _ := m["uri"].(string); uri != "" {
			entry["uri"] = uri
		}
		if name, _ := m["name"].(string); name != "" {
			entry["name"] = name
		}
		if len(entry) > 0 {
			roots = append(roots, entry)
		}
	}
	return roots
}

func stringField(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return v
}

func stringSlice(m map[string]any, key string) []string {
	raw, ok := m[key].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func mapField(m map[string]any, key string) map[string]string {
	raw, ok := m[key].(map[string]any)
	if !ok {
		return nil
	}
	out := make(map[string]string, len(raw))
	for k, v := range raw {
		if s, ok := v.(string); ok {
			out[k] = s
		}
	}
	return out
}
