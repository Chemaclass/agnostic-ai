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

type ImportSpan struct {
	Path  string
	Start int
	End   int
}

func ApplyImportModeWithSpans(body, mode string) (string, []ImportSpan, error) {
	if mode != ImportModeInline {
		text, err := ApplyImportMode(body, mode)
		return text, nil, err
	}
	return inlineImportLinesWithSpans(body)
}

func inlineImportLines(body string) (string, error) {
	text, _, err := inlineImportLinesWithSpans(body)
	return text, err
}

func inlineImportLinesWithSpans(body string) (string, []ImportSpan, error) {
	lines := strings.Split(body, "\n")
	imports := spec.IncludeLines(lines)
	byLine := make(map[int]string, len(imports))
	for _, imp := range imports {
		byLine[imp.Line] = imp.Ref
	}
	var out strings.Builder
	var spans []ImportSpan
	for i, line := range lines {
		if i > 0 {
			out.WriteByte('\n')
		}
		ref, ok := byLine[i]
		if !ok {
			out.WriteString(line)
			continue
		}
		data, err := os.ReadFile(ref)
		if err != nil {
			return "", nil, fmt.Errorf("%s: %w", ref, err)
		}
		start := out.Len()
		fmt.Fprintf(&out, importInlineStartFmt+"\n%s\n"+importInlineEnd, ref, strings.TrimRight(string(data), "\n"))
		spans = append(spans, ImportSpan{Path: ref, Start: start, End: out.Len()})
	}
	return out.String(), spans, nil
}

// restoreImportInlines rewrites every sentinel-wrapped resolved import
// back to its lone `@path` line, so an entry-point emitted in inline mode
// round-trips to the canonical body on import. Returns body unchanged
// when it carries no inline blocks.
func restoreImportInlines(body string) string {
	return importInlineBlockRe.ReplaceAllString(body, "@$1")
}
