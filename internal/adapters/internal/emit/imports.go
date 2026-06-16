package emit

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// File-import resolution modes for the shared entry-point body. They
// govern how `@path` lines reach targets that cannot follow them. See
// config.SyncConfig.ResolveImports.
const (
	ImportModePassthrough = "passthrough"
	ImportModeStrip       = "strip"
	ImportModeInline      = "inline"
)

// fileImportTargets names targets whose CLI resolves `@path` file-import
// lines in its entry-point file. They always receive the body verbatim;
// the resolve-imports modes apply only to the rest.
var fileImportTargets = map[string]bool{
	"claude": true,
}

// SupportsFileImports reports whether target's CLI resolves `@path`
// file-import lines in its entry-point file.
func SupportsFileImports(target string) bool {
	return fileImportTargets[target]
}

// importLineRe matches a line that is a lone `@path` file-import token:
// optional surrounding whitespace, a leading `@`, then a single non-space
// path. Prose `@mentions` embedded in a sentence never match because the
// whole line must be the token.
var importLineRe = regexp.MustCompile(`^[ \t]*@(\S+)[ \t]*$`)

// Sentinel markers wrapping a resolved import in inline mode. The start
// marker carries the original `@path` so import can rebuild the lone
// `@path` line, keeping the AGNOSTIC_AI.md round-trip lossless.
const (
	importInlineStartFmt = "<!-- agnostic-ai:import:start %s -->"
	importInlineEnd      = "<!-- agnostic-ai:import:end -->"
)

// importInlineBlockRe matches a sentinel-wrapped resolved import, capturing
// the original path from the start marker.
var importInlineBlockRe = regexp.MustCompile(`(?s)<!-- agnostic-ai:import:start (\S+) -->\n.*?\n<!-- agnostic-ai:import:end -->`)

// ApplyImportMode rewrites lone `@path` file-import lines in body per mode
// for a target that cannot resolve them. passthrough (and any unknown
// mode) returns body unchanged. strip drops the lines. inline replaces
// each with the referenced file's content wrapped in a sentinel block
// that import restores to the original `@path` line.
func ApplyImportMode(body, mode string) (string, error) {
	switch mode {
	case ImportModeStrip:
		return stripImportLines(body), nil
	case ImportModeInline:
		return inlineImportLines(body)
	default:
		return body, nil
	}
}

// stripImportLines drops every lone `@path` import line from body.
func stripImportLines(body string) string {
	lines := strings.Split(body, "\n")
	out := lines[:0]
	for _, ln := range lines {
		if importLineRe.MatchString(ln) {
			continue
		}
		out = append(out, ln)
	}
	return strings.Join(out, "\n")
}

// inlineImportLines replaces each lone `@path` import line with the
// referenced file's content, wrapped in sentinel markers. A missing or
// unreadable file is a hard error: the user opted into inlining, so a
// dangling reference must surface rather than ship silently.
func inlineImportLines(body string) (string, error) {
	lines := strings.Split(body, "\n")
	for i, ln := range lines {
		m := importLineRe.FindStringSubmatch(ln)
		if m == nil {
			continue
		}
		path := m[1]
		data, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("%s: %w", path, err)
		}
		content := strings.TrimRight(string(data), "\n")
		lines[i] = fmt.Sprintf(importInlineStartFmt, path) + "\n" + content + "\n" + importInlineEnd
	}
	return strings.Join(lines, "\n"), nil
}

// RestoreImportInlines rewrites every sentinel-wrapped resolved import
// back to its lone `@path` line, so an entry-point emitted in inline mode
// round-trips to the canonical body on import. Returns body unchanged
// when it carries no inline blocks.
func RestoreImportInlines(body string) string {
	return importInlineBlockRe.ReplaceAllString(body, "@$1")
}
