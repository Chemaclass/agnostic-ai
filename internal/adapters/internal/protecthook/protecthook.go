// Package protecthook renders the sh and awk script that blocks an
// agent's edit to a protected path. A target whose hook reads the tool
// call as JSON on stdin and blocks on exit 2 supplies only the awk that
// finds the edited paths in its payload; the decoding, path resolution,
// matching, and failure handling are shared.
package protecthook

import (
	"fmt"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// ScriptName is the generated script's file name in a target's hooks
// directory.
const ScriptName = "agnostic-ai-protect.sh"

// Reader is a target's half of the awk program. Awk defines start(),
// called once the rules are set; text(s), called with each decoded
// piece of the payload in order; and finish(), called at the end of
// input. It records each edited path with paths[++npaths] and can call
// block(message) and addAlias(absolute cwd).
//
// Decoded text keeps JSON's structural quotes as `"`. A quote inside a
// string arrives as "\002", a backslash as "\001", a NUL or bad escape
// as "\003", and any other escaped non-ASCII character as "\200".
type Reader struct {
	// Edits names what the script blocks, as in "Codex apply_patch edits".
	Edits string
	// Tool names the tool that lets an edit through when its hook fails.
	Tool string
	Awk  string
}

// Render writes the hook for groups. It needs only sh and awk: a
// session may have neither jq nor the agnostic-ai binary on PATH. A
// hook that fails or times out lets the edit through, so every failure
// exits 2 and the decoding stays linear. The awk program sits in single
// quotes, so patterns and reasons are escaped for an awk string and
// then for sh.
func Render(r Reader, groups []spec.ProtectGroup) string {
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

	program := sharedAwk + r.Awk + strings.Replace(mainAwk, "  @RULES@\n", rules.String(), 1)
	var out strings.Builder
	out.WriteString("#!/bin/sh\n")
	out.WriteString(emit.HeaderBlock(emit.FormatShell))
	fmt.Fprintf(&out, "# Blocks %s to the protected paths in settings specs.\n", r.Edits)
	out.WriteString("# Needs only a POSIX shell and awk. Exit 2 blocks the edit; stderr says why.\n")
	fmt.Fprintf(&out, "# %s lets an edit through when a hook fails, so every failure exits 2.\n", r.Tool)
	out.WriteString(preamble)
	out.WriteString("awk '")
	out.WriteString(strings.ReplaceAll(program, "'", `'\''`))
	out.WriteString("' 1>&2\n")
	out.WriteString(status)
	return out.String()
}

func awkString(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

const preamble = `LC_ALL=C
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

// status maps awk's exit: 0 allows, 2 blocks with the reasons awk
// printed, and anything else is a failure that blocks.
const status = `status=$?
case $status in
0) exit 0 ;;
2) exit 2 ;;
esac
fail "awk stopped with status $status"
`

// sharedAwk resolves and matches paths. Paths compare without regard
// to case, since the checkout may sit on a case-insensitive file
// system, and a backslash reads as a separator. An absolute path under
// the payload's cwd, or under $PWD, is resolved from the physical cwd,
// so a symlinked checkout still matches.
const sharedAwk = `function clean(p,    n, parts, i, k, out, stack) {
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
`

// mainAwk reads the payload with RS set to a backslash, so each record
// after the first starts with one JSON escape letter and decoding stays
// linear in awks where gsub or UTF-8 indexing is not. LC_ALL=C makes
// every awk read bytes.
const mainAwk = `BEGIN {
  RS = "\\"
  esc["\""] = "\002"; esc["/"] = "/"; esc["t"] = "\t"; esc["r"] = "\r"; esc["f"] = "\f"; esc["b"] = "\b"; esc["n"] = "\n"
  root = drive(ENVIRON["AGNOSTIC_AI_ROOT"])
  lroot = drive(ENVIRON["AGNOSTIC_AI_LOGICAL_ROOT"])
  cwd = drive(ENVIRON["AGNOSTIC_AI_CWD"])
  addAlias(ENVIRON["AGNOSTIC_AI_LOGICAL_CWD"])
  @RULES@
  start()
}
NR == 1 { text($0); next }
plain { plain = 0; text($0); next }
$0 == "" { text("\001"); plain = 1; next }
{
  c = substr($0, 1, 1)
  r = substr($0, 2)
  if (c == "u") text(unicode(substr(r, 1, 4)) substr(r, 5))
  else if (c in esc) text(esc[c] r)
  else text(c r)
}
END {
  finish()
  for (i = 1; i <= npaths; i++) check(paths[i])
  for (i = 1; i <= nblocked; i++) print blocked[i]
  if (nblocked > 0) exit 2
}
`
