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

// MCPOption gates an optional, target-scoped field in the shared
// MCPSchemaServersMap builder. Kept separate from MCPSchema because the
// same schema (and therefore the same JSON wrapper shape) is shared by
// several targets whose vendor docs disagree on which extra per-server
// fields exist.
type MCPOption func(*mcpOptions)

type mcpOptions struct {
	cursorExtras bool
	claudeExtras bool
	kiroExtras   bool
}

// WithCursorMCPExtras turns on Cursor-only MCP fields (`envFile` on
// stdio servers, an `auth` object on remote servers) in the shared
// MCPSchemaServersMap builder. This builder also serves Claude Code,
// Kiro, Junie, Qoder, Factory, and Copilot's root-mcp-file mirror; none
// of their vendor docs mention either field, so the option keeps them
// opt-in per caller rather than always-on for every MCPSchemaServersMap
// target. cursor.com/docs/mcp.md: "envFile ... Path to an environment
// file to load more variables" (stdio only) and "Add an `auth` object
// to remote server entries that use `url`" (target-audit 2026-09-03,
// #661).
func WithCursorMCPExtras() MCPOption {
	return func(o *mcpOptions) { o.cursorExtras = true }
}

// WithClaudeMCPExtras turns on the four Claude Code MCP fields that
// reached `.mcp.json` by no route before, top-level or namespaced
// (target-audit 2026-08-27, #634). code.claude.com/docs/en/mcp:
//
//   - headersHelper (remote only): "If your MCP server uses an
//     authentication scheme other than OAuth, such as Kerberos,
//     short-lived tokens, or an internal SSO, use headersHelper to
//     generate request headers at connection time."
//   - timeout: "Set a per-server tool execution timeout by adding a
//     timeout field in milliseconds to that server's .mcp.json entry".
//   - alwaysLoad: "If a server's tools should always be visible to
//     Claude without a search step, set alwaysLoad to true in that
//     server's configuration." The page adds that it "is available on
//     all server types", so it is not gated on transport.
//   - oauth (remote only): "Set authServerMetadataUrl in the oauth
//     object of your server's config in .mcp.json".
//
// Kept opt-in rather than always-on because this builder also serves
// Cursor, Kiro, Junie, Qoder, Factory, and Copilot's root-mcp-file
// mirror, and none of their vendor docs names these four.
func WithClaudeMCPExtras() MCPOption {
	return func(o *mcpOptions) { o.claudeExtras = true }
}

// WithKiroMCPExtras turns on the four Kiro MCP fields that reached
// `.kiro/settings/mcp.json` by no route before (target-audit
// 2026-08-27, #634). kiro.dev/docs/mcp/configuration/ documents
// `autoApprove` ("Tool names to auto-approve without prompting") and
// `disabledTools` ("Tool names to omit when calling the Agent") on both
// local and remote servers, plus `oauth` ("OAuth configuration for
// servers that require authentication") and `oauthScopes` ("OAuth
// scopes to request (fallback; overridden by oauth.oauthScopes if both
// are set)") on remote servers only.
//
// Kiro's `oauth` object is not Claude Code's: it takes clientId,
// clientSecret, redirectUri, clientMetadataUrl, and oauthScopes, where
// Claude Code takes clientId, callbackPort, authServerMetadataUrl, and
// a space-separated scopes string. Each target maps its own documented
// sub-keys, so a spec written for one never leaks an unrecognized key
// into the other's file.
func WithKiroMCPExtras() MCPOption {
	return func(o *mcpOptions) { o.kiroExtras = true }
}

// WriteMCPFile renders mcps with the target schema and writes to path.
// No file is written when mcps is empty or every entry produces an empty
// server block. Adapters call this in lieu of hand-rolling the
// guard / render / write triple at every call site.
func (s *Session) WriteMCPFile(mcps []spec.Entry, schema MCPSchema, path string, dryRun bool, opts ...MCPOption) error {
	if len(mcps) == 0 {
		return nil
	}
	doc, err := MCPDocument(mcps, schema, opts...)
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
func MCPDocument(mcps []spec.Entry, schema MCPSchema, opts ...MCPOption) (string, error) {
	var o mcpOptions
	for _, opt := range opts {
		opt(&o)
	}
	servers := map[string]map[string]any{}
	for _, e := range mcps {
		if e.Name == "" {
			continue
		}
		entry := buildServer(e, schema, o)
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

func buildServer(e spec.Entry, schema MCPSchema, o mcpOptions) map[string]any {
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
		// envFile is documented stdio-only: "The envFile option is only
		// available for STDIO servers. Remote servers (HTTP/SSE) do not
		// support envFile." cursor.com/docs/mcp.md.
		if o.cursorExtras {
			if envFile := stringField(e.Meta, "envFile"); envFile != "" {
				out["envFile"] = envFile
			}
		}
	case "http", "sse", "ws":
		if url := stringField(e.Meta, "url"); url != "" {
			out["url"] = url
		}
		if h := mapField(e.Meta, "headers"); len(h) > 0 {
			out["headers"] = h
		}
		// "Add an auth object to remote server entries that use url":
		// CLIENT_ID / CLIENT_SECRET / scopes, explicit field mapping
		// rather than a generic passthrough so a typo in the spec cannot
		// leak an unrelated key into Cursor's static-OAuth block.
		// cursor.com/docs/mcp.md.
		if o.cursorExtras {
			if auth := buildCursorMCPAuth(e.Meta); len(auth) > 0 {
				out["auth"] = auth
			}
		}
		// headersHelper and the oauth object are remote-only on Claude
		// Code: the ws section says a `ws` entry "accepts the same url,
		// headers, headersHelper, timeout, and alwaysLoad fields as
		// http", and the OAuth flags "only apply to HTTP and SSE
		// transports. They have no effect on stdio servers". See #634.
		if o.claudeExtras {
			if helper := stringField(e.Meta, "headersHelper"); helper != "" {
				out["headersHelper"] = helper
			}
			if oauth := buildClaudeMCPOAuth(e.Meta); len(oauth) > 0 {
				out["oauth"] = oauth
			}
		}
		// Kiro documents oauth and oauthScopes on its remote-server
		// table only; the local-server table has neither. See #634.
		if o.kiroExtras {
			if oauth := buildKiroMCPOAuth(e.Meta); len(oauth) > 0 {
				out["oauth"] = oauth
			}
			if scopes, ok := scopeList(e.Meta, "oauthScopes"); ok {
				out["oauthScopes"] = scopes
			}
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
	// timeout and alwaysLoad are transport-independent on Claude Code
	// ("The alwaysLoad field is available on all server types"), so they
	// land here rather than in a transport branch. See #634.
	if o.claudeExtras {
		if timeout, ok := IntField(e.Meta, "timeout"); ok {
			out["timeout"] = timeout
		}
		if alwaysLoad, _ := e.Meta["alwaysLoad"].(bool); alwaysLoad {
			out["alwaysLoad"] = true
		}
	}
	// Kiro lists autoApprove and disabledTools in both its local-server
	// and remote-server tables, so both are transport-independent too.
	if o.kiroExtras {
		if approve := stringSlice(e.Meta, "autoApprove"); len(approve) > 0 {
			out["autoApprove"] = approve
		}
		if disabledTools := stringSlice(e.Meta, "disabledTools"); len(disabledTools) > 0 {
			out["disabledTools"] = disabledTools
		}
	}
	if schema == MCPSchemaVSCodeServers {
		out["type"] = transport
	}
	return out
}

// buildClaudeMCPOAuth reads the spec's `oauth` object into the shape
// code.claude.com/docs/en/mcp documents for a `.mcp.json` entry:
// `clientId`, `callbackPort`, `authServerMetadataUrl`, and a
// space-separated `scopes` string. Explicit field mapping, not a
// generic passthrough, so a key meant for another target's
// differently-shaped `oauth` (Kiro's `redirectUri`, Crush's plain bool)
// cannot reach the file.
//
// `clientSecret` is deliberately absent: the vendor says "The client
// secret is stored securely in your system keychain (macOS) or a
// credentials file, not in your config", so writing one here would put
// a secret in a committed file that Claude Code never reads.
func buildClaudeMCPOAuth(meta map[string]any) map[string]any {
	raw, ok := meta["oauth"].(map[string]any)
	if !ok {
		return nil
	}
	out := map[string]any{}
	if id, _ := raw["clientId"].(string); id != "" {
		out["clientId"] = id
	}
	if port, ok := IntField(raw, "callbackPort"); ok {
		out["callbackPort"] = port
	}
	if url, _ := raw["authServerMetadataUrl"].(string); url != "" {
		out["authServerMetadataUrl"] = url
	}
	if scopes, _ := raw["scopes"].(string); scopes != "" {
		out["scopes"] = scopes
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// buildKiroMCPOAuth reads the spec's `oauth` object into Kiro's own
// documented shape (kiro.dev/docs/mcp/configuration/ "OAuth
// properties"): `clientId`, `clientSecret`, `redirectUri`,
// `clientMetadataUrl`, and an `oauthScopes` array. Explicit field
// mapping for the same reason as the Claude Code mapper above.
//
// Unlike Claude Code, Kiro does document `clientSecret` inside the
// config object, with its own warning that "The CLI supports
// confidential clients (clientId + clientSecret)", so it maps here.
func buildKiroMCPOAuth(meta map[string]any) map[string]any {
	raw, ok := meta["oauth"].(map[string]any)
	if !ok {
		return nil
	}
	out := map[string]any{}
	if id, _ := raw["clientId"].(string); id != "" {
		out["clientId"] = id
	}
	if secret, _ := raw["clientSecret"].(string); secret != "" {
		out["clientSecret"] = secret
	}
	if uri, _ := raw["redirectUri"].(string); uri != "" {
		out["redirectUri"] = uri
	}
	if url, _ := raw["clientMetadataUrl"].(string); url != "" {
		out["clientMetadataUrl"] = url
	}
	if scopes, ok := scopeList(raw, "oauthScopes"); ok {
		out["oauthScopes"] = scopes
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// scopeList reads a list-valued meta key, reporting presence separately
// from length so an explicitly empty list still emits. Kiro's own doc
// makes `[]` meaningful rather than equivalent to absent: "If you
// encounter OAuth scope errors, use an empty array: `oauthScopes: []`".
func scopeList(m map[string]any, key string) ([]string, bool) {
	raw, ok := m[key].([]any)
	if !ok {
		return nil, false
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out, true
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

// buildCursorMCPAuth reads the spec's `auth` object into Cursor's static
// OAuth shape: `{CLIENT_ID, CLIENT_SECRET, scopes}`. Explicit field
// mapping, not a generic passthrough, so an unrelated key under `auth`
// (or a value meant for another target's differently-shaped `auth`
// field, e.g. Codex's `auth: "oauth"` string) cannot reach the emitted
// file. Returns nil when the meta value is missing, the wrong type, or
// carries none of the three known keys.
func buildCursorMCPAuth(meta map[string]any) map[string]any {
	raw, ok := meta["auth"].(map[string]any)
	if !ok {
		return nil
	}
	out := map[string]any{}
	if id, _ := raw["CLIENT_ID"].(string); id != "" {
		out["CLIENT_ID"] = id
	}
	if secret, _ := raw["CLIENT_SECRET"].(string); secret != "" {
		out["CLIENT_SECRET"] = secret
	}
	if scopes := stringSlice(raw, "scopes"); len(scopes) > 0 {
		out["scopes"] = scopes
	}
	if len(out) == 0 {
		return nil
	}
	return out
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
