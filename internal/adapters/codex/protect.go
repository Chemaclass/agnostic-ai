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
// (learn.chatgpt.com/docs/hooks), so the command finds the script from
// the Git root and falls back to the cwd outside a repository. Windows
// runs it through the sh on PATH, such as Git for Windows provides.
func protectHook() spec.Entry {
	script := emit.HookScriptsDir(target) + "/" + protectScriptName
	return spec.Entry{
		Kind: spec.KindHook,
		Name: "agnostic-ai-protect",
		Path: "settings (protected)",
		Meta: map[string]any{
			"event":          "PreToolUse",
			"matcher":        "apply_patch",
			"command":        `"$(git rev-parse --show-toplevel 2>/dev/null || pwd)/` + script + `"`,
			"commandWindows": "sh " + script,
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
// agnostic-ai binary on PATH. The awk program sits in single quotes, so
// patterns and reasons are escaped for an awk string and then for sh.
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
				n, awkString(spec.ProtectPatternRegexp(path)), n, awkString(path), n, g+1)
		}
	}
	fmt.Fprintf(&rules, "  npat = %d\n", n)

	program := strings.Replace(protectAwkProgram, "  @RULES@\n", rules.String(), 1)
	var out strings.Builder
	out.WriteString("#!/bin/sh\n")
	out.WriteString(emit.HeaderBlock(emit.FormatShell))
	out.WriteString(protectScriptPreamble)
	out.WriteString("exec awk '")
	out.WriteString(strings.ReplaceAll(program, "'", `'\''`))
	out.WriteString("'\n")
	return out.String()
}

func awkString(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

const protectScriptPreamble = `# Blocks Codex apply_patch edits to the protected paths in settings specs.
# Needs only a POSIX shell and awk. Exit 2 blocks the edit; stderr says why.
hooks_dir=$(dirname -- "$0")
AGNOSTIC_AI_ROOT=$(cd -- "$hooks_dir/../.." && pwd -P) || exit 1
AGNOSTIC_AI_LOGICAL_ROOT=$(cd -- "$hooks_dir/../.." && pwd -L) || exit 1
AGNOSTIC_AI_CWD=$(pwd -P)
AGNOSTIC_AI_LOGICAL_CWD=$(pwd -L)
export AGNOSTIC_AI_ROOT AGNOSTIC_AI_LOGICAL_ROOT AGNOSTIC_AI_CWD AGNOSTIC_AI_LOGICAL_CWD
`

// protectAwkProgram decodes the JSON string escapes with gsub instead of
// a per-character loop, which is quadratic in awks that index UTF-8
// strings by character. A header line starts a decoded line or the
// patch string, after optional blanks, because Codex trims header lines;
// an added or removed body line starts with + or -, so it never reads
// as one. An absolute path under the payload's cwd, or under $PWD, is
// resolved from the physical cwd, so a symlinked checkout still matches.
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
function under(abs) {
  if (index(abs, root "/") == 1) return substr(abs, length(root) + 2)
  if (index(abs, lroot "/") == 1) return substr(abs, length(lroot) + 2)
  return ""
}
function relative(p,    abs, rel, i) {
  if (substr(p, 1, 1) == "/") abs = clean(p); else abs = clean(cwd "/" p)
  rel = under(abs)
  for (i = 1; rel == "" && i <= nalias; i++) {
    if (index(abs, alias[i] "/") == 1) rel = under(clean(cwd "/" substr(abs, length(alias[i]) + 2)))
  }
  return rel
}
function check(p,    rel, cand, i) {
  rel = relative(p)
  if (rel == "" || (rel in seen)) return
  seen[rel] = 1
  cand = rel
  while (1) {
    for (i = 1; i <= npat; i++) {
      if (cand ~ pat[i]) {
        blocked[++nblocked] = "agnostic-ai: " rel " is protected (" txt[i] "). " why[grp[i]]
        return
      }
    }
    if (index(cand, "/") == 0) return
    sub(/\/[^\/]*$/, "", cand)
  }
}
function header(line, marker,    at, before, p) {
  at = index(line, marker)
  if (at == 0) return
  before = substr(line, 1, at - 1)
  sub(/[ \t]+$/, "", before)
  if (before != "" && substr(before, length(before), 1) != "\"") return
  p = restore(stringAt(line, at + length(marker)), "\001", "\\")
  sub(/[ \t]+$/, "", p)
  if (p != "") check(p)
}
function stringAt(s, from,    p, end) {
  p = substr(s, from)
  end = index(p, "\"")
  if (end > 0) p = substr(p, 1, end - 1)
  return restore(p, "\002", "\"")
}
function addAlias(p) {
  if (p == "" || substr(p, 1, 1) != "/") return
  p = clean(restore(p, "\001", "\\"))
  if (p != "" && p != cwd) alias[++nalias] = p
}
function restore(p, code, text,    parts, n, i, out) {
  n = split(p, parts, code)
  out = parts[1]
  for (i = 2; i <= n; i++) out = out text parts[i]
  return out
}
BEGIN {
  root = ENVIRON["AGNOSTIC_AI_ROOT"]
  lroot = ENVIRON["AGNOSTIC_AI_LOGICAL_ROOT"]
  cwd = ENVIRON["AGNOSTIC_AI_CWD"]
  @RULES@
}
{ buf = buf $0 "\n" }
END {
  gsub(/\\\\/, "\001", buf)
  gsub(/\\"/, "\002", buf)
  gsub(/\\\//, "/", buf)
  gsub(/\\n/, "\n", buf)
  gsub(/\\r/, "", buf)
  gsub(/\\t/, "\t", buf)
  if (match(buf, /"cwd"[ \t]*:[ \t]*"/)) addAlias(stringAt(buf, RSTART + RLENGTH))
  addAlias(ENVIRON["AGNOSTIC_AI_LOGICAL_CWD"])
  n = split(buf, lines, "\n")
  for (i = 1; i <= n; i++) {
    header(lines[i], "*** Add File: ")
    header(lines[i], "*** Update File: ")
    header(lines[i], "*** Delete File: ")
    header(lines[i], "*** Move to: ")
  }
  if (nblocked == 0) exit 0
  for (i = 1; i <= nblocked; i++) print blocked[i] | "cat 1>&2"
  close("cat 1>&2")
  exit 2
}
`
