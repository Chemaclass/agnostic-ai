package openhands

import (
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
// lands in. `sse_servers` and `shttp_servers` hold bare URL strings,
// the simplest of the two documented forms; `stdio_servers` holds
// name/command/args/env tables, since a stdio server needs more than a
// URL to identify or launch it. The direct-key arrays (`sse_servers`,
// `shttp_servers`) must render before the `[[mcp.stdio_servers]]`
// array-of-tables blocks: TOML forbids reopening the `[mcp]` table for
// a plain key once a nested array-of-tables under it has been opened.
func renderConfigTOML(stdio, sse, shttp []spec.Entry) string {
	if len(stdio) == 0 && len(sse) == 0 && len(shttp) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(emit.HeaderBlock(emit.FormatTOML))
	sb.WriteString("[mcp]\n")

	wroteDirectKey := false
	if len(sse) > 0 {
		emit.WriteTOMLStringArray(&sb, "sse_servers", urlsOf(sse))
		wroteDirectKey = true
	}
	if len(shttp) > 0 {
		emit.WriteTOMLStringArray(&sb, "shttp_servers", urlsOf(shttp))
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

// urlsOf returns the `url` meta field of each entry, in the same
// order. Callers only pass entries mcpTransportBuckets already
// confirmed carry a non-empty url.
func urlsOf(entries []spec.Entry) []string {
	out := make([]string, 0, len(entries))
	for _, m := range entries {
		url, _ := m.Meta["url"].(string)
		out = append(out, url)
	}
	return out
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
func mcpTransportBuckets(mcps []spec.Entry) (stdio, sse, shttp []spec.Entry, unmapped int) {
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
			}
		case "http":
			if url, _ := m.Meta["url"].(string); url != "" {
				shttp = append(shttp, m)
			}
		default:
			unmapped++
		}
	}
	byName := func(a, b spec.Entry) int { return strings.Compare(a.Name, b.Name) }
	slices.SortFunc(stdio, byName)
	slices.SortFunc(sse, byName)
	slices.SortFunc(shttp, byName)
	return stdio, sse, shttp, unmapped
}
