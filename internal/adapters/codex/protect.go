package codex

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const protectScriptName = "agnostic-ai-protect.sh"

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
	script := `"$(git rev-parse --show-toplevel 2>/dev/null || pwd)/` + emit.HookScriptsDir(target) + "/" + protectScriptName + `"`
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
	path := filepath.Join(emit.HookScriptsDir(target), protectScriptName)
	return sess.WriteExecutableFile(path, renderProtectScript(groups), dryRun)
}

// renderProtectScript writes a hook that reads the apply_patch JSON on
// stdin, collects every Add, Update, Delete, and Move to path, and exits
// 2 with the reasons on stderr when one is protected. Codex shows that
// stderr as the reason for the block. PreToolUse parses an ask decision
// but does not support it yet, so ask also blocks and tells the agent to
// ask the user.
//
// It needs only sh and awk: a Codex session may have neither jq nor the
// agnostic-ai binary on PATH. A hook that fails or times out lets the
// edit through, so every failure exits 2 and the decoding stays linear.
// The awk program sits in single quotes, so patterns and reasons are
// escaped for an awk string and then for sh.
func renderProtectScript(groups []spec.ProtectGroup) string {
	var rules strings.Builder
	n := 0
	for g, group := range groups {
		instruction := "Ask the user before editing it. This hook blocks the edit either way, so the user makes the change or removes the path from the protected list."
		if group.Decision == spec.ProtectDeny {
			instruction = "Do not edit it."
		}
		message := strings.TrimSpace(group.Reason + " " + instruction)
		fmt.Fprintf(&rules, "  why[%d] = %s\n", g+1, awkString(message))
		for _, path := range group.Paths {
			n++
			fmt.Fprintf(&rules, "  pat[%d] = %s; txt[%d] = %s; grp[%d] = %d\n",
				n, awkString(strings.ToLower(spec.ProtectPatternRegexp(path))), n, awkString(path), n, g+1)
		}
	}
	fmt.Fprintf(&rules, "  npat = %d\n", n)

	program := strings.Replace(protectAwkProgram, "  @RULES@\n", rules.String(), 1)
	var out strings.Builder
	out.WriteString("#!/bin/sh\n")
	out.WriteString(emit.HeaderBlock(emit.FormatShell))
	out.WriteString(protectScriptPreamble)
	out.WriteString("awk '")
	out.WriteString(strings.ReplaceAll(program, "'", `'\''`))
	out.WriteString("' 1>&2\n")
	out.WriteString(protectScriptStatus)
	return out.String()
}

func awkString(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

const protectScriptPreamble = `# Blocks Codex apply_patch edits to the protected paths in settings specs.
# Needs only a POSIX shell and awk. Exit 2 blocks the edit; stderr says why.
# Codex lets an edit through when a hook fails, so every failure exits 2.
LC_ALL=C
export LC_ALL
fail() {
  printf 'agnostic-ai: %s, so the protect hook blocked this edit.\n' "$1" >&2
  exit 2
}
command -v awk >/dev/null 2>&1 || fail "awk is not on PATH"
case $0 in
*/*) hooks_dir=${0%/*} ;;
*) hooks_dir=. ;;
esac
AGNOSTIC_AI_ROOT=$(cd -- "$hooks_dir/../.." && pwd -P) || fail "the project root is not readable"
AGNOSTIC_AI_LOGICAL_ROOT=$(cd -- "$hooks_dir/../.." && pwd -L) || fail "the project root is not readable"
AGNOSTIC_AI_CWD=$(pwd -P) || fail "the working directory is not readable"
AGNOSTIC_AI_LOGICAL_CWD=$(pwd -L) || fail "the working directory is not readable"
export AGNOSTIC_AI_ROOT AGNOSTIC_AI_LOGICAL_ROOT AGNOSTIC_AI_CWD AGNOSTIC_AI_LOGICAL_CWD
`

// protectScriptStatus maps awk's exit: 0 allows, 2 blocks with the
// reasons awk printed, and anything else is a failure that blocks.
const protectScriptStatus = `status=$?
case $status in
0) exit 0 ;;
2) exit 2 ;;
esac
fail "awk stopped with status $status"
`

// protectAwkProgram reads the payload with RS set to a backslash, so
// each record after the first starts with one JSON escape letter and
// decoding stays linear in awks where gsub or UTF-8 indexing is not.
// LC_ALL=C makes every awk read bytes.
//
// A patch line becomes a candidate header when, after the whitespace
// Codex trims (str::trim strips all Unicode White_Space), it starts with
// "*", a quote, or a control byte. Other lines are skipped as they
// stream. Non-ASCII bytes around a header are trimmed like whitespace
// and the rest is checked; other control bytes there block the edit.
//
// Paths compare without regard to case, since the checkout may sit on
// a case-insensitive file system, and a backslash reads as a separator.
// An absolute path under the payload's cwd, or under $PWD, is resolved
// from the physical cwd, so a symlinked checkout still matches.
const protectAwkProgram = `function clean(p,    n, parts, i, k, out, stack) {
  n = split(p, parts, "/")
  k = 0
  for (i = 1; i <= n; i++) {
    if (parts[i] == "" || parts[i] == ".") continue
    if (parts[i] == "..") { if (k > 0) k--; continue }
    stack[++k] = parts[i]
  }
  out = ""
  for (i = 1; i <= k; i++) out = out "/" stack[i]
  return out
}
function drive(p) {
  if (p ~ /^[A-Za-z]:\//) return "/" tolower(substr(p, 1, 1)) substr(p, 3)
  return p
}
function under(abs,    lower) {
  lower = tolower(abs)
  if (index(lower, tolower(root) "/") == 1) return substr(abs, length(root) + 2)
  if (index(lower, tolower(lroot) "/") == 1) return substr(abs, length(lroot) + 2)
  return ""
}
function relative(p,    abs, rel, i) {
  p = drive(p)
  if (substr(p, 1, 1) == "/") abs = clean(p); else abs = clean(cwd "/" p)
  rel = under(abs)
  for (i = 1; rel == "" && i <= nalias; i++) {
    if (index(tolower(abs), tolower(alias[i]) "/") == 1) rel = under(clean(cwd "/" substr(abs, length(alias[i]) + 2)))
  }
  return rel
}
function check(p,    rel, cand, i) {
  rel = relative(p)
  if (rel == "" || (rel in seen)) return
  seen[rel] = 1
  cand = tolower(rel)
  while (1) {
    for (i = 1; i <= npat; i++) {
      if (cand ~ pat[i]) {
        block(rel " is protected (" txt[i] "). " why[grp[i]])
        return
      }
    }
    if (index(cand, "/") == 0) return
    sub(/\/[^\/]*$/, "", cand)
  }
}
function block(message) {
  blocked[++nblocked] = "agnostic-ai: " message
}
function trimmed(c) {
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
function addAlias(p) {
  p = drive(p)
  if (substr(p, 1, 1) != "/") return
  p = clean(p)
  if (p != "" && p != cwd) alias[++nalias] = p
}
function restore(p, code, text,    parts, n, i, out) {
  if (index(p, code) == 0) return p
  n = split(p, parts, code)
  out = parts[1]
  for (i = 2; i <= n; i++) out = out text parts[i]
  return out
}
function unicode(h,    v, i, d) {
  v = 0
  for (i = 1; i <= 4; i++) {
    d = index("0123456789abcdef", tolower(substr(h, i, 1)))
    if (d == 0) return "\003"
    v = v * 16 + d - 1
  }
  if (v == 0) return "\003"
  if (v == 34) return "\002"
  if (v == 92) return "\001"
  if (v >= 128) return "\200"
  return sprintf("%c", v)
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
BEGIN {
  RS = "\\"
  first = 1
  fresh = 1
  esc["\""] = "\002"; esc["/"] = "/"; esc["t"] = "\t"; esc["r"] = "\r"; esc["f"] = "\f"; esc["b"] = "\b"
  root = drive(ENVIRON["AGNOSTIC_AI_ROOT"])
  lroot = drive(ENVIRON["AGNOSTIC_AI_LOGICAL_ROOT"])
  cwd = drive(ENVIRON["AGNOSTIC_AI_CWD"])
  addAlias(ENVIRON["AGNOSTIC_AI_LOGICAL_CWD"])
  @RULES@
}
NR == 1 { text($0); next }
plain { plain = 0; text($0); next }
$0 == "" { text("\001"); plain = 1; next }
{
  c = substr($0, 1, 1)
  r = substr($0, 2)
  if (c == "n") { newline(); text(r) }
  else if (c == "u") text(unicode(substr(r, 1, 4)) substr(r, 5))
  else if (c in esc) text(esc[c] r)
  else text(c r)
}
END {
  newline()
  for (i = 1; i <= npaths; i++) check(paths[i])
  for (i = 1; i <= nblocked; i++) print blocked[i]
  if (nblocked > 0) exit 2
}
`
