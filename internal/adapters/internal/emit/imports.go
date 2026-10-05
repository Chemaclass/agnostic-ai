package emit

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/spec"
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
// for a target that cannot resolve them. A line inside a code fence is an
// example and stays. passthrough (and any unknown mode) returns body
// unchanged. strip drops the lines. inline replaces
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
	drop := map[int]bool{}
	for _, imp := range spec.IncludeLines(lines) {
		drop[imp.Line] = true
	}
	out := lines[:0]
	for i, ln := range lines {
		if !drop[i] {
			out = append(out, ln)
		}
	}
	return strings.Join(out, "\n")
}

// inlineImportLines replaces each lone `@path` import line with the
// referenced file's content, wrapped in sentinel markers. A missing or
// unreadable file is a hard error: the user opted into inlining, so a
// dangling reference must surface rather than ship silently.
func inlineImportLines(body string) (string, error) {
	lines := strings.Split(body, "\n")
	for _, imp := range spec.IncludeLines(lines) {
		data, err := os.ReadFile(imp.Ref)
		if err != nil {
			return "", fmt.Errorf("%s: %w", imp.Ref, err)
		}
		content := strings.TrimRight(string(data), "\n")
		lines[imp.Line] = fmt.Sprintf(importInlineStartFmt, imp.Ref) + "\n" + content + "\n" + importInlineEnd
	}
	return strings.Join(lines, "\n"), nil
}

// restoreImportInlines rewrites every sentinel-wrapped resolved import
// back to its lone `@path` line, so an entry-point emitted in inline mode
// round-trips to the canonical body on import. Returns body unchanged
// when it carries no inline blocks.
func restoreImportInlines(body string) string {
	return importInlineBlockRe.ReplaceAllString(body, "@$1")
}
