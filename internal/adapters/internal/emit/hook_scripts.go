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
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// agnosticScriptsDir is the project-relative root for stashed hook
// script bodies. Lives under the managed `.agnostic-ai/` tree so
// gitignoring per-tool hook directories does not lose the bodies.
const agnosticScriptsDir = ".agnostic-ai/scripts"

type HookScript struct {
	Path string
	Body []byte
	Mode fs.FileMode
}

func NeutralHookScripts(command, target, sourceDir, outputDir string, literal ...bool) ([]HookScript, error) {
	var scripts []HookScript
	for _, ref := range neutralHookReferences(command, len(literal) > 0 && literal[0]) {
		name := ref.name
		if !fs.ValidPath(name) || strings.Contains(name, "\\") {
			return nil, fmt.Errorf("%s/%s: shared hook script path must stay inside the scripts directory", sourceDir, name)
		}
		body, mode, ok, err := findHookScriptBodyIn(sourceDir, name, target, "", true)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("%s/%s: shared hook script not found", sourceDir, name)
		}
		scripts = append(scripts, HookScript{Path: filepath.Join(outputDir, filepath.FromSlash(name)), Body: body, Mode: mode})
	}
	return scripts, nil
}

func (s *Session) MaterializeNeutralHookScripts(hooks []spec.Entry, target, outputDir string, dryRun bool) error {
	if wrapper, ok := PortableHookWrapperScript(hooks, target, outputDir); ok {
		if err := s.writeFileWithMode(wrapper.Path, string(wrapper.Body), wrapper.Mode, true, dryRun); err != nil {
			return err
		}
	}
	for _, hook := range hooks {
		if event, _ := hook.Meta["event"].(string); event == "" {
			continue
		}
		kind, _ := hook.Meta["type"].(string)
		if (target == "claude" || target == "codex" || target == "qoder" || target == "copilot") && kind != "" && kind != "command" {
			continue
		}
		if (target == "cursor" || target == "windsurf") && kind == "prompt" {
			continue
		}
		commands, literal := hookSourceCommands(hook, target)
		type sourceCommand struct {
			value   string
			literal bool
		}
		var references []sourceCommand
		for _, command := range commands {
			references = append(references, sourceCommand{command, literal})
		}
		if target == "codex" {
			if windows, _ := hook.Meta["commandWindows"].(string); windows != "" {
				references = append(references, sourceCommand{windows, false})
			}
		}
		for _, command := range references {
			scripts, err := NeutralHookScripts(command.value, target, agnosticScriptsDir, outputDir, command.literal)
			if err != nil {
				path := hook.Path
				if path == "" {
					path = hook.Name
				}
				return fmt.Errorf("%s: %w", path, err)
			}
			for _, script := range scripts {
				if err := s.writeFileWithMode(script.Path, string(script.Body), script.Mode, true, dryRun); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

type neutralHookReference struct {
	start, end int
	rootStart  int
	quote      byte
	name       string
}

func neutralHookReferences(command string, literal bool) []neutralHookReference {
	const prefix = agnosticScriptsDir + "/"
	var refs []neutralHookReference
	for from := 0; from < len(command); {
		index := indexHookSegment(command[from:], prefix)
		if index < 0 {
			break
		}
		start := from + index
		rootStart, valid := neutralHookRoot(command, start)
		if !valid {
			from = start + len(prefix)
			continue
		}
		quote := hookShellQuote(command[:start])
		end, name := neutralHookFilename(command[start+len(prefix):], quote, literal)
		end += start + len(prefix)
		refs = append(refs, neutralHookReference{start: start, end: end, rootStart: rootStart, quote: quote, name: name})
		from = end
	}
	return refs
}

func neutralHookRoot(command string, start int) (int, bool) {
	if start == 0 || command[start-1] != '/' {
		return start, true
	}
	roots := []string{".", gitHookRoot}
	for _, variable := range nativeHookRoots {
		roots = append(roots, "$"+variable, "${"+variable+"}")
	}
	for _, root := range roots {
		prefix := root + "/"
		if !strings.HasSuffix(command[:start], prefix) {
			continue
		}
		before := start - len(prefix)
		if before == 0 || strings.ContainsRune(" \t\r\n\"';|&()", rune(command[before-1])) {
			return before, true
		}
	}
	return start, false
}

func hookShellQuote(command string) byte {
	var quote byte
	for i := 0; i < len(command); i++ {
		if command[i] == '\\' && quote != '\'' {
			i++
			continue
		}
		if command[i] == quote {
			quote = 0
		} else if quote == 0 && (command[i] == '\'' || command[i] == '"') {
			quote = command[i]
		}
	}
	return quote
}

func neutralHookFilename(path string, quote byte, literal bool) (int, string) {
	if literal {
		return len(path), path
	}
	var name strings.Builder
	for i := 0; i < len(path); i++ {
		ch := path[i]
		if quote == 0 && strings.ContainsRune(" \t\r\n;|&()<>", rune(ch)) {
			return i, name.String()
		}
		if ch == '\\' && quote != '\'' && i+1 < len(path) {
			next := path[i+1]
			if quote == 0 || strings.ContainsRune("$`\"\\\n", rune(next)) {
				i++
				if next != '\n' {
					name.WriteByte(next)
				}
				continue
			}
		}
		if ch == quote {
			quote = 0
		} else if quote == 0 && (ch == '\'' || ch == '"') {
			quote = ch
		} else {
			name.WriteByte(ch)
		}
	}
	return len(path), name.String()
}

func hookSourceCommands(hook spec.Entry, target string) ([]string, bool) {
	meta := ResolveMeta(hook.Meta, target)
	literal := len(StringSlice(meta["args"])) > 0 || target == "augment"
	if target == "gemini" {
		if native, ok := hook.Meta["x-gemini"].(map[string]any); ok {
			if handlers, exists := native["hooks"]; exists {
				var commands []string
				for _, handler := range HookCommandEntries(handlers) {
					commands = append(commands, handler["command"].(string))
				}
				return commands, false
			}
		}
		return HookCommands(meta["command"]), literal
	}
	if target == "kiro" {
		if native, ok := hook.Meta["x-kiro"].(map[string]any); ok {
			if action, exists := native["action"]; exists {
				if fields, ok := action.(map[string]any); ok && fields["type"] == "command" {
					return HookCommands(fields["command"]), false
				}
				return nil, false
			}
		}
	}
	return HookCommands(hook.Meta["command"]), literal
}

// MaterializeHookScript copies the script body that backs cmd into the
// target's hooks directory when a stashed copy exists. Returns nil and
// writes nothing when cmd is not a sibling-tool hook path or no body is
// stashed; surfaces every other read/write error so callers can fail
// loudly on permission problems.
//
// The write goes through the session like every other output, so a
// script listed under sync.unmanaged is skipped and reported, and the
// write honors capture, dry-run, backup, transaction rollback, and
// detailed recording (which puts the script in the sync ledger) (#789).
// The body is copied byte for byte with the stash's permission bits.
//
// `cmd` is the rewritten command path (post-RewriteHookPath), pointing
// at `.<target>/hooks/<basename>`. `sourceTool` carries the spec
// origin so the lookup falls back to that tool's stashed body when no
// target-specific variant exists.
func (s *Session) MaterializeHookScript(cmd, target, sourceTool string, dryRun bool) error {
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
	return s.writeFileWithMode("."+target+"/hooks/"+basename, string(body), mode, true, dryRun)
}

func (s *Session) MaterializedHookScriptBody(cmd, target, sourceTool string) ([]byte, bool, error) {
	basename, ok := hookBasename(cmd, target)
	if !ok || s.IsUnmanaged("."+target+"/hooks/"+basename) {
		return nil, false, nil
	}
	body, _, found, err := findHookScriptBody(basename, target, sourceTool)
	return body, found, err
}

func (s *Session) MaterializedHookScriptBodies(cmd, target, sourceTool string, metadata ...map[string]any) ([][]byte, error) {
	meta := hookRootMeta(target, metadata)
	literal := len(StringSlice(meta["args"])) > 0 || target == "augment"
	scripts, err := NeutralHookScripts(cmd, target, agnosticScriptsDir, HookScriptsDir(target), literal)
	if err != nil {
		return nil, err
	}
	var bodies [][]byte
	for _, script := range scripts {
		if !s.IsUnmanaged(script.Path) {
			bodies = append(bodies, script.Body)
		}
	}
	cmd = RewriteNeutralHookPath(cmd, ".", literal)
	body, found, err := s.MaterializedHookScriptBody(RewriteHookPath(cmd, target, metadata...), target, sourceTool)
	if err != nil {
		return nil, err
	}
	if found {
		bodies = append(bodies, body)
	}
	return bodies, nil
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

// indexHookSegment finds a segment at the start or after whitespace, a slash, or a quote.
// The boundary check stops
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
		case '/', '"', '\'', ' ', '\t', '\n', ';', '|', '&', '(', ')':
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
	return findHookScriptBodyIn(agnosticScriptsDir, basename, target, sourceTool)
}

func findHookScriptBodyIn(dir, basename, target, sourceTool string, confined ...bool) ([]byte, os.FileMode, bool, error) {
	candidates := []string{
		filepath.Join(dir, target, basename),
	}
	if sourceTool != "" && sourceTool != target {
		candidates = append(candidates, filepath.Join(dir, sourceTool, basename))
	}
	candidates = append(candidates, filepath.Join(dir, basename))

	for _, path := range candidates {
		if len(confined) > 0 && confined[0] {
			resolved, err := canonicalHookPath(path)
			if IsAbsent(err) {
				continue
			}
			if err != nil {
				return nil, 0, false, fmt.Errorf("%s: %w", path, err)
			}
			root, err := canonicalHookPath(dir)
			if err != nil {
				return nil, 0, false, fmt.Errorf("%s: %w", dir, err)
			}
			relative, err := filepath.Rel(root, resolved)
			if err != nil || !fs.ValidPath(filepath.ToSlash(relative)) {
				return nil, 0, false, fmt.Errorf("%s: shared hook script resolves outside the scripts directory", path)
			}
			path = resolved
		}
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

func canonicalHookPath(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(absolute)
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
