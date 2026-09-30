package codex

import (
	"path/filepath"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/protecthook"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// ProtectedPaths reports that Codex enforces protected paths with a
// generated PreToolUse hook: Codex has no per-tool permission key.
func (Adapter) ProtectedPaths() (enforcement, reason string) { return "hook", "" }

// protectHook is the PreToolUse hook that runs the generated script on
// every apply_patch call. Codex reports edits as apply_patch whichever
// alias the matcher names, and runs hooks from the session cwd
// (learn.chatgpt.com/docs/hooks), so both commands find the script from
// the Git root and fall back to the cwd outside a repository. Windows
// runs it through the sh on PATH, such as Git for Windows provides.
func protectHook() spec.Entry {
	script := `"$(git rev-parse --show-toplevel 2>/dev/null || pwd)/` + emit.HookScriptsDir(target) + "/" + protecthook.ScriptName + `"`
	return spec.Entry{
		Kind: spec.KindHook,
		Name: "agnostic-ai-protect",
		Path: "settings (protected)",
		Meta: map[string]any{
			"event":          "PreToolUse",
			"matcher":        "apply_patch",
			"command":        script,
			"commandWindows": "sh -c 'exec sh " + script + "'",
		},
	}
}

// IsProtectHookCommand reports whether command runs the generated
// protect script, which import skips: its source is the settings spec.
func IsProtectHookCommand(command string) bool {
	return command == protectHook().Meta["command"]
}

func emitProtectScript(sess *emit.Session, groups []spec.ProtectGroup, dryRun bool) error {
	if len(groups) == 0 {
		return nil
	}
	path := filepath.Join(emit.HookScriptsDir(target), protecthook.ScriptName)
	return sess.WriteExecutableFile(path, renderProtectScript(groups), dryRun)
}

// renderProtectScript writes a hook that reads the apply_patch JSON on
// stdin, collects every Add, Update, Delete, and Move to path, and exits
// 2 with the reasons on stderr when one is protected. Codex shows that
// stderr as the reason for the block. PreToolUse parses an ask decision
// but does not support it yet, so ask also blocks and tells the agent to
// ask the user.
func renderProtectScript(groups []spec.ProtectGroup) string {
	return protecthook.Render(protecthook.Reader{Edits: "Codex apply_patch edits", Tool: "Codex", Awk: patchReaderAwk}, groups)
}

// patchReaderAwk finds the paths in an apply_patch body. A patch line
// becomes a candidate header when, after the whitespace Codex trims
// (str::trim strips all Unicode White_Space), it starts with "*", a
// quote, or a control byte. Other lines are skipped as they stream.
// Non-ASCII bytes around a header are trimmed like whitespace and the
// rest is checked; other control bytes there block the edit.
const patchReaderAwk = `function trimmed(c) {
  return c != "" && (index(" \t\v\f\r", c) > 0 || c >= "\200")
}
function control(c) {
  return c != "" && (c < " " || c == "\177") && !trimmed(c)
}
function header(l, marker,    at, before, p, c, odd) {
  at = index(l, marker)
  if (at == 0) return
  before = substr(l, 1, at - 1)
  odd = 0
  sub(/[ \t]+$/, "", before)
  while (before != "") {
    c = substr(before, length(before), 1)
    if (control(c)) odd = 1
    else if (!trimmed(c)) break
    before = substr(before, 1, length(before) - 1)
  }
  if (before != "" && c != "\"") return
  p = restore(stringAt(l, at + length(marker)), "\001", "/")
  sub(/[ \t]+$/, "", p)
  while (p != "") {
    c = substr(p, length(p), 1)
    if (control(c)) odd = 1
    else if (!trimmed(c)) break
    p = substr(p, 1, length(p) - 1)
  }
  if (control(substr(p, 1, 1))) odd = 1
  if (odd) {
    block("the header for " p " has control characters the hook cannot check. Write the header without them.")
    return
  }
  if (p != "") paths[++npaths] = p
}
function stringAt(s, from,    p, end) {
  p = substr(s, from)
  end = index(p, "\"")
  if (end > 0) p = substr(p, 1, end - 1)
  return restore(p, "\002", "\"")
}
function start() {
  first = 1
  fresh = 1
}
function text(s,    i) {
  while ((i = index(s, "\n")) > 0) {
    piece(substr(s, 1, i - 1))
    newline()
    s = substr(s, i + 1)
  }
  piece(s)
}
function piece(s,    t, c) {
  if (skip) return
  if (fresh) {
    t = s
    sub(/^[ \t]+/, "", t)
    while (t != "" && trimmed(substr(t, 1, 1))) t = substr(t, 2)
    if (t != "") {
      fresh = 0
      c = substr(t, 1, 1)
      if (!first && c != "*" && c != "\"" && !control(c)) {
        skip = 1
        cur = ""
        return
      }
    }
  }
  cur = cur s
}
function newline() {
  if (!skip) line(cur)
  cur = ""
  skip = 0
  fresh = 1
  first = 0
}
function finish() {
  newline()
}
function line(l) {
  if (!havecwd && match(l, /"cwd"[ \t]*:[ \t]*"/)) {
    havecwd = 1
    addAlias(restore(stringAt(l, RSTART + RLENGTH), "\001", "/"))
  }
  header(l, "*** Add File: ")
  header(l, "*** Update File: ")
  header(l, "*** Delete File: ")
  header(l, "*** Move to: ")
}
`
