// Package emit hook_scripts materialises hook script bodies stored
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
	"errors"
	"fmt"
	"io/fs"
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
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(dst), err)
	}
	if err := os.WriteFile(dst, body, mode); err != nil {
		return fmt.Errorf("write %s: %w", dst, err)
	}
	return nil
}

// SourceToolFromHookCommand extracts the leading `.<tool>/hooks/`
// segment from cmd. Returns ("", false) when cmd is not a recognised
// sibling-tool hook path. Used by adapters to remember the spec origin
// before RewriteHookPath rewrites it to their own prefix.
func SourceToolFromHookCommand(cmd string) (string, bool) {
	if !strings.HasPrefix(cmd, ".") {
		return "", false
	}
	rest := cmd[1:]
	slash := strings.IndexByte(rest, '/')
	if slash <= 0 {
		return "", false
	}
	tool := rest[:slash]
	if !strings.HasPrefix(rest[slash:], "/hooks/") {
		return "", false
	}
	return tool, true
}

// hookBasename returns the script filename portion of cmd when cmd
// points at `.<target>/hooks/<basename>` (the post-rewrite form). The
// basename must contain no further slashes so an absolute path or a
// nested directory does not produce a misleading write target.
func hookBasename(cmd, target string) (string, bool) {
	prefix := "." + target + "/hooks/"
	if !strings.HasPrefix(cmd, prefix) {
		return "", false
	}
	rest := cmd[len(prefix):]
	if rest == "" || strings.ContainsRune(rest, '/') {
		return "", false
	}
	return rest, true
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
	if errors.Is(err, fs.ErrNotExist) {
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
