// Package emit hook_scripts materializes hook script bodies stored
// under `.agnostic-ai/scripts/` into the target's `.<target>/hooks/`
// directory at emit time. Pairing this with `RewriteHookPath` lets a
// project keep `.<tool>/` gitignored: a fresh checkout reconstructs the
// per-tool hooks dir from the tracked scripts stash on every `sync`.
//
// Lookup precedence for a hook command of the form
// `.<source-tool>/hooks/<basename>`:
//
//  1. `.agnostic-ai/scripts/<target>/<basename>`       — explicit
//     target-specific variant (e.g. codex needs more privileges than
//     claude on the same script).
//  2. `.agnostic-ai/scripts/<source-tool>/<basename>`  — the body
//     captured at the spec's origin.
//  3. `.agnostic-ai/scripts/<basename>`                — unified
//     variant shared across every target.
//
// When no candidate is on disk the helper is a no-op: hand-authored
// hook commands (`gofmt`, `bash -c '…'`, absolute paths) carry no body
// to ship.
package emit

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// agnosticScriptsDir is the project-relative root for stashed hook
// script bodies. Lives under the managed `.agnostic-ai/` tree so
// gitignoring per-tool hook directories does not lose the bodies.
const agnosticScriptsDir = ".agnostic-ai/scripts"

// MaterializeHookScript copies the script body that backs cmd into the
// target's hooks directory when a stashed copy exists. Returns nil and
// writes nothing when cmd is not a sibling-tool hook path or no body is
// stashed; surfaces every other read/write error so callers can fail
// loudly on permission problems.
//
// `cmd` is the rewritten command path (post-RewriteHookPath), pointing
// at `.<target>/hooks/<basename>`. `sourceTool` carries the spec
// origin so the lookup falls back to that tool's stashed body when no
// target-specific variant exists.
func MaterializeHookScript(cmd, target, sourceTool string, dryRun bool) error {
	basename, ok := hookBasename(cmd, target)
	if !ok {
		return nil
	}
	body, mode, ok, err := findHookScriptBody(basename, target, sourceTool)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	dst := "." + target + "/hooks/" + basename
	if dryRun {
		return nil
	}
	if err := mkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(dst), err)
	}
	if err := os.WriteFile(dst, body, mode); err != nil {
		return fmt.Errorf("write %s: %w", dst, err)
	}
	return nil
}

// SourceToolFromHookCommand extracts the `.<tool>/hooks/` segment from
// cmd. Recognizes bare paths (`.codex/hooks/x.sh`) and shell-expansion
// wrappers (`"$(git rev-parse --show-toplevel)/.codex/hooks/x.sh"`) so
// the spec origin survives the round-trip through hooks.json. Returns
// ("", false) when cmd contains no recognized sibling-tool segment.
func SourceToolFromHookCommand(cmd string) (string, bool) {
	for _, prefix := range hookSiblingPrefixes {
		if hasHookSegment(cmd, prefix) {
			// strip leading "." and trailing "/hooks/" to recover tool name
			return prefix[1 : len(prefix)-len("/hooks/")], true
		}
	}
	return "", false
}

// hookBasename returns the script filename trailing a `.<target>/hooks/`
// segment in cmd. Scans the whole command so a shell-expansion wrapped
// path (the form codex hooks.json uses) still yields a basename. The
// basename ends at the first `/`, `"`, whitespace, or end of string so
// a trailing argument list does not bleed into the filename.
func hookBasename(cmd, target string) (string, bool) {
	segment := "." + target + "/hooks/"
	idx := indexHookSegment(cmd, segment)
	if idx < 0 {
		return "", false
	}
	rest := cmd[idx+len(segment):]
	end := len(rest)
	for i, r := range rest {
		if r == '/' || r == '"' || r == ' ' || r == '\t' || r == '\n' {
			end = i
			break
		}
	}
	basename := rest[:end]
	if basename == "" {
		return "", false
	}
	return basename, true
}

// hasHookSegment reports whether cmd contains the sibling-tool hooks
// prefix at a path boundary — either at the start of cmd or preceded by
// a character that cannot be part of a directory name (`/`, `"`, `'`).
func hasHookSegment(cmd, segment string) bool {
	return indexHookSegment(cmd, segment) >= 0
}

// indexHookSegment finds the byte offset where segment begins in cmd
// when it appears at a path boundary, or -1 if not present. A boundary
// is the start of cmd, a `/`, a `"`, or a `'`. The boundary check stops
// false positives when, say, a hook command embeds a literal string
// that happens to contain `.codex/hooks/` mid-token.
func indexHookSegment(cmd, segment string) int {
	from := 0
	for from <= len(cmd)-len(segment) {
		i := strings.Index(cmd[from:], segment)
		if i < 0 {
			return -1
		}
		abs := from + i
		if abs == 0 {
			return abs
		}
		switch cmd[abs-1] {
		case '/', '"', '\'':
			return abs
		}
		from = abs + 1
	}
	return -1
}

// findHookScriptBody walks the agnostic scripts stash in lookup order
// and returns the first non-empty body found. The boolean reports
// whether anything was loaded; the error channel surfaces real I/O
// failures while a plain "no script stashed" is communicated via the
// (nil, 0, false, nil) return.
func findHookScriptBody(basename, target, sourceTool string) ([]byte, os.FileMode, bool, error) {
	candidates := []string{
		filepath.Join(agnosticScriptsDir, target, basename),
	}
	if sourceTool != "" && sourceTool != target {
		candidates = append(candidates, filepath.Join(agnosticScriptsDir, sourceTool, basename))
	}
	candidates = append(candidates, filepath.Join(agnosticScriptsDir, basename))

	for _, path := range candidates {
		body, mode, ok, err := readHookCandidate(path)
		if err != nil {
			return nil, 0, false, err
		}
		if ok {
			return body, mode, true, nil
		}
	}
	return nil, 0, false, nil
}

// readHookCandidate reads path and returns (body, mode, true) when the
// file exists. Returns (nil, 0, false, nil) when path is absent so the
// caller can try the next candidate without special-casing fs.ErrNotExist
// in every loop.
func readHookCandidate(path string) ([]byte, os.FileMode, bool, error) {
	info, err := os.Stat(path)
	if IsAbsent(err) {
		return nil, 0, false, nil
	}
	if err != nil {
		return nil, 0, false, fmt.Errorf("stat %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return nil, 0, false, nil
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, false, fmt.Errorf("read %s: %w", path, err)
	}
	mode := info.Mode().Perm()
	if mode == 0 {
		mode = 0o755
	}
	return body, mode, true, nil
}
