package openhands

import (
	"fmt"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// renderConfigTOML builds the `./config.toml` `[mcp]` section from
// already-bucketed MCP entries. Returns "" when every bucket is empty.
//
// OpenHands has no `type` discriminator field the way Claude Code or
// Cursor do: transport is implied entirely by which array a server
// lands in. Each `sse_servers` / `shttp_servers` element is a bare URL
// string, the simplest of the two documented forms, or the vendor's
// `{ url = "...", api_key = "..." }` object when the entry sets
// `api_key` (see serverValue). `stdio_servers` holds name/command/
// args/env tables, since a stdio server needs more than a URL to
// identify or launch it. TOML 1.0 allows mixing scalar and inline-
// table elements in one array, exactly as the vendor's own example
// does, so a bucket with both forms still renders one valid array.
// The direct-key arrays (`sse_servers`, `shttp_servers`) must render
// before the `[[mcp.stdio_servers]]` array-of-tables blocks: TOML
// forbids reopening the `[mcp]` table for a plain key once a nested
// array-of-tables under it has been opened.
func renderConfigTOML(stdio, sse, shttp []spec.Entry) string {
	if len(stdio) == 0 && len(sse) == 0 && len(shttp) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(emit.HeaderBlock(emit.FormatTOML))
	sb.WriteString("[mcp]\n")

	wroteDirectKey := false
	if len(sse) > 0 {
		writeServerArray(&sb, "sse_servers", sse, false)
		wroteDirectKey = true
	}
	if len(shttp) > 0 {
		writeServerArray(&sb, "shttp_servers", shttp, true)
		wroteDirectKey = true
	}
	if wroteDirectKey && len(stdio) > 0 {
		sb.WriteString("\n")
	}
	for _, m := range stdio {
		sb.WriteString("[[mcp.stdio_servers]]\n")
		emit.WriteTOMLString(&sb, "name", m.Name)
		emit.WriteTOMLMCPStdioFields(&sb, m.Meta)
		sb.WriteString("\n")
	}
	return sb.String()
}

// writeServerArray writes `key = [...]`, one element per entry in the
// order given (mcpTransportBuckets already sorted it by name). shttp
// selects whether serverValue also reads `timeout`, since the vendor
// documents that field for shttp_servers only.
func writeServerArray(sb *strings.Builder, key string, entries []spec.Entry, shttp bool) {
	sb.WriteString(key + " = [")
	for i, m := range entries {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(serverValue(m, shttp))
	}
	sb.WriteString("]\n")
}

// serverValue renders one sse_servers/shttp_servers array element.
// Callers only pass entries mcpTransportBuckets already confirmed
// carry a non-empty url.
//
// `api_key` is the only credential field OpenHands documents for a
// remote server (https://docs.openhands.dev/openhands/usage/settings/mcp-settings,
// "either a string URL or an object with the following properties":
// `url` required, `api_key` optional). When set, the entry upgrades
// from the bare URL string to that object form. The cross-tool spec's
// own auth field, `headers`, has no vendor-documented equivalent here
// (no header-map key exists for sse_servers/shttp_servers, and the doc
// never ties `api_key` to a specific header or scheme), so it is never
// read for this; emitMCPConfig surfaces that gap as a coverage note
// instead of guessing a translation.
//
// `timeout` (int, 1-3600s, default 60, worked example `timeout = 1800`)
// is documented under the SHTTP tab only, not for sse_servers, so shttp
// is the only caller that passes shttp=true and lets it upgrade the
// entry the same way api_key does; an sse entry that sets it is a
// no-op the caller (mcpTransportBuckets' timeoutNoOp) turns into a
// coverage note instead (#588).
func serverValue(m spec.Entry, shttp bool) string {
	url, _ := m.Meta["url"].(string)
	apiKey, _ := m.Meta["api_key"].(string)
	timeout := 0
	if shttp {
		timeout = intMeta(m.Meta, "timeout")
	}
	if apiKey == "" && timeout == 0 {
		return `"` + emit.EscapeTOMLBasic(url) + `"`
	}
	out := `{ url = "` + emit.EscapeTOMLBasic(url) + `"`
	if apiKey != "" {
		out += `, api_key = "` + emit.EscapeTOMLBasic(apiKey) + `"`
	}
	if timeout != 0 {
		out += fmt.Sprintf(", timeout = %d", timeout)
	}
	return out + " }"
}

// intMeta reads an int-typed meta key, accepting int / int64 / float64
// (yaml.v3 decodes numerics as one of these depending on the source).
// Returns 0 when missing or the wrong type, which doubles as "unset"
// here since OpenHands' own documented range starts at 1.
func intMeta(meta map[string]any, key string) int {
	switch v := meta[key].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	}
	return 0
}

// mcpTransportBuckets sorts mcps into OpenHands' three `[mcp]` arrays,
// each sorted by name for deterministic output.
//
// `stdio` (the default when `type` is unset) requires `command`;
// `sse` and `http` (OpenHands' streamable-HTTP transport, `shttp_servers`)
// require `url`. An entry missing its transport's required field is
// dropped silently: there is nothing for OpenHands to run or connect
// to (mirrors the codex and kilo adapters).
//
// unmapped counts entries whose `type` is set to something other than
// the stdio default, `sse`, or `http` (e.g. `ws`, confirmed elsewhere
// for Claude Code and Qoder but not documented anywhere for OpenHands).
// These have no home in any `[mcp]` array; the caller turns unmapped
// into a coverage note rather than silently guessing a bucket for a
// transport OpenHands does not document.
//
// headersNoOp counts sse/http entries that do land in their bucket
// (carry a non-empty url) and also declare a non-empty `headers` map.
// OpenHands' object form has no header-map key for either array, only
// the single `api_key` string serverValue reads (config_toml.go), so
// headers is inert on every entry that reaches this adapter; the
// caller turns the count into a coverage note so that never goes
// unmarked. An entry that is itself dropped for missing url is not
// counted here: NoteFieldNoOp is for a field going inert on an entry
// that otherwise reaches the target in full, not one that never does.
//
// timeoutNoOp is the same idea for `timeout`, but scoped to the sse
// bucket only: the vendor documents that field under the SHTTP tab,
// never for sse_servers, so an sse entry that sets it never reaches
// serverValue's object form (see serverValue's shttp parameter).
func mcpTransportBuckets(mcps []spec.Entry) (stdio, sse, shttp []spec.Entry, unmapped, headersNoOp, timeoutNoOp int) {
	hasHeaders := func(m spec.Entry) bool { return len(emit.StringMap(m.Meta["headers"])) > 0 }
	hasTimeout := func(m spec.Entry) bool { return intMeta(m.Meta, "timeout") != 0 }
	for _, m := range mcps {
		if m.Name == "" {
			continue
		}
		transport, _ := m.Meta["type"].(string)
		if transport == "" {
			transport = "stdio"
		}
		switch transport {
		case "stdio":
			if cmd, _ := m.Meta["command"].(string); cmd != "" {
				stdio = append(stdio, m)
			}
		case "sse":
			if url, _ := m.Meta["url"].(string); url != "" {
				sse = append(sse, m)
				if hasHeaders(m) {
					headersNoOp++
				}
				if hasTimeout(m) {
					timeoutNoOp++
				}
			}
		case "http":
			if url, _ := m.Meta["url"].(string); url != "" {
				shttp = append(shttp, m)
				if hasHeaders(m) {
					headersNoOp++
				}
			}
		default:
			unmapped++
		}
	}
	byName := func(a, b spec.Entry) int { return strings.Compare(a.Name, b.Name) }
	slices.SortFunc(stdio, byName)
	slices.SortFunc(sse, byName)
	slices.SortFunc(shttp, byName)
	return stdio, sse, shttp, unmapped, headersNoOp, timeoutNoOp
}
