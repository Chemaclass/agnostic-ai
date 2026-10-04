package emit

import (
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// HookJSONEscape defines aai_json, which writes its stdin as the body of
// a JSON string. It runs in the C locale, so bytes that are not UTF-8
// pass through instead of stopping awk; it drops control bytes other than
// tab and newline, and writes `{` and `}` as \u escapes, since Cline's VS
// Code extension's fallback parser counts braces without reading quotes
// (hook-factory.ts:366-466).
const HookJSONEscape = `aai_json() { LC_ALL=C tr -d '\000-\010\013-\037\177' | LC_ALL=C awk 'BEGIN { ORS = "" } { gsub(/\\/, "\\\\"); gsub(/"/, "\\\""); gsub(/\t/, "\\t"); gsub(/{/, "\\u007b"); gsub(/}/, "\\u007d"); if (NR > 1) printf "\\n"; print }'; }
`

// hookStdoutDecision defines aai_decide, which reads the portable
// decision protocol at exit 0. stdout carries only the decision: empty
// stdout goes on as a plain exit 0, and anything else must be one JSON
// object with a single top-level "decision" of "allow", "deny", or "ask"
// and an optional top-level "reason". aai_parse walks the JSON in awk, so
// a "decision" in a nested object or a string never counts, and escaped
// keys read as the key they spell. deny and ask become exit 2 with the
// reason on stderr; ask blocks too, since not every tool can ask the
// user. allow drops the object and goes on as a plain exit 0, so it never
// grants more than the tool's own rules do. Output that is not one JSON
// object, a duplicate or missing top-level decision, or another value
// blocks, so a broken guard never lets a call through, and so does
// stdout over 1,000,000 bytes, which keeps the parse inside any hook
// timeout. Parsing also blocks beyond 10,000 values or 64 container
// levels. A \u escape outside ASCII reads as "?".
const hookStdoutDecision = `aai_parse() {
  LC_ALL=C tr -d '\000' <"$1" | cmp -s "$1" - || { printf 'error\n'; return; }
  LC_ALL=C awk '
function fail() { print "error"; exit 0 }
function ws() { while (match(substr(s, p, 4096), /^[ \t\n\r]+/)) p += RLENGTH }
function hex(h, v, i) {
  v = 0
  for (i = 1; i <= 4; i++) v = v * 16 + index("0123456789abcdef", tolower(substr(h, i, 1))) - 1
  return v
}
function str(out, c, h) {
  if (substr(s, p, 1) != "\"") fail()
  p++
  out = ""
  while (p <= n) {
    if (match(substr(s, p, 4096), /^[^"\\]+/)) {
      c = substr(s, p, RLENGTH)
      if (c ~ /[\001-\037]/) fail()
      out = out c
      p += RLENGTH
      continue
    }
    c = substr(s, p, 1)
    if (c == "\"") { p++; return out }
    if (c != "\\") fail()
    c = substr(s, p + 1, 1)
    p += 2
    if (c == "n") out = out "\n"
    else if (c == "t") out = out "\t"
    else if (c == "r") out = out sprintf("%c", 13)
    else if (c == "b") out = out sprintf("%c", 8)
    else if (c == "f") out = out sprintf("%c", 12)
    else if (c == "\"" || c == "\\" || c == "/") out = out c
    else if (c == "u") {
      h = substr(s, p, 4)
      if (h !~ /^[0-9A-Fa-f][0-9A-Fa-f][0-9A-Fa-f][0-9A-Fa-f]$/) fail()
      h = hex(h)
      out = out ((h >= 32 && h < 127) ? sprintf("%c", h) : "?")
      p += 4
    } else fail()
  }
  fail()
}
function value(depth, c, start, number) {
  if (++tokens > 10000) fail()
  ws()
  c = substr(s, p, 1)
  if (c == "{") return obj(0, depth + 1)
  if (c == "[") return arr(depth + 1)
  if (c == "\"") { last = str(); return "s" }
  if (substr(s, p, 4) == "true" || substr(s, p, 4) == "null") { p += 4; return "x" }
  if (substr(s, p, 5) == "false") { p += 5; return "x" }
  start = p
  while (match(substr(s, p, 4096), /^[-+0-9.eE]+/)) p += RLENGTH
  number = substr(s, start, p - start)
  if (number ~ /^-?(0|[1-9][0-9]*)(\.[0-9]+)?([eE][-+]?[0-9]+)?$/) return "x"
  fail()
}
function arr(depth) {
  if (depth > 64) fail()
  p++
  ws()
  if (substr(s, p, 1) == "]") { p++; return "x" }
  while (1) {
    value(depth)
    ws()
    if (substr(s, p, 1) == ",") { p++; continue }
    if (substr(s, p, 1) == "]") { p++; return "x" }
    fail()
  }
}
function obj(top, depth, k, t) {
  if (depth > 64) fail()
  p++
  ws()
  if (substr(s, p, 1) == "}") { p++; return "x" }
  while (1) {
    ws()
    k = str()
    ws()
    if (substr(s, p, 1) != ":") fail()
    p++
    t = value(depth)
    if (top && k == "decision") { count++; verdict = (t == "s") ? last : "" }
    if (top && k == "reason" && t == "s") reason = last
    ws()
    if (substr(s, p, 1) == ",") { p++; continue }
    if (substr(s, p, 1) == "}") { p++; return "x" }
    fail()
  }
}
BEGIN { RS = "\001" }
{ s = (NR > 1 ? s "\001" : "") $0 }
END {
  n = length(s)
  if (n > 1000000) fail()
  p = 1
  ws()
  if (p > n) { print "empty"; exit 0 }
  if (substr(s, p, 1) != "{") fail()
  obj(1, 1)
  ws()
  if (p <= n || count != 1) fail()
  print verdict
  printf "%s", reason
}' "$1"
}
aai_decide() {
  [ "$aai_status" -eq 0 ] || return 0
  aai_parsed=$(aai_parse "$aai_out") || aai_parsed=error
  aai_verdict=${aai_parsed%%$'\n'*}
  aai_reason=
  case $aai_parsed in *$'\n'*) aai_reason=${aai_parsed#*$'\n'} ;; esac
  : >"$aai_out"
  case $aai_verdict in
  allow | empty) ;;
  deny | ask)
    printf '%s\n' "${aai_reason:-blocked by the hook decision}" >>"$aai_err"
    aai_status=2 ;;
  *)
    printf '%s\n' 'blocked: stdout is not one JSON object with a single "decision" of "allow", "deny", or "ask"' >>"$aai_err"
    aai_status=2 ;;
  esac
}
`

// DecisionWrapperName is the file name of the script sync puts in front
// of a portable hook command: on a target that reads a block another way
// than exit 2 and stderr, and for a hook that sets `decision: stdout`.
const DecisionWrapperName = "agnostic-ai-portable-hook.sh"

// portableHookWrapperSource is where a portable hook command names the
// wrapper before a target's path rewrite, the same as a shared script.
const portableHookWrapperSource = agnosticScriptsDir + "/" + DecisionWrapperName

// decisionWrapperShebang runs the wrapper with bash, as Claude Code runs a
// hook command, whatever shell the target starts the command line with.
// The provenance comment sits on the line below it, where header.Leads
// looks, so import leaves the wrapper behind.
const decisionWrapperShebang = "#!/usr/bin/env bash\n"

// DecisionWrapper renders that script. Options come first: --decision
// reads the stdout decision protocol, and --prompt picks a target's
// prompt reply. It then runs its next argument with `bash -c`, passing
// stdin and the environment through, and replays the command's stderr.
// reply is a shell `case` over $aai_status, the command's exit code, that
// writes the target's reply: $aai_out names the file holding the
// command's stdout, and $aai_msg is its stderr as the body of a JSON
// string.
func DecisionWrapper(reply string) string {
	body := `aai_decision=
aai_prompt=
while :; do
  case $1 in
  --decision) aai_decision=1 ;;
  --prompt) aai_prompt=1 ;;
  *) break ;;
  esac
  shift
done
aai_out=$(mktemp) || { echo "agnostic-ai hook wrapper: mktemp failed" >&2; exit 2; }
aai_err=$(mktemp) || { rm -f "$aai_out"; echo "agnostic-ai hook wrapper: mktemp failed" >&2; exit 2; }
trap 'rm -f "$aai_out" "$aai_err"' EXIT
` + HookJSONEscape + hookStdoutDecision + `bash -c "$1" >"$aai_out" 2>"$aai_err"
aai_status=$?
[ -z "$aai_decision" ] || aai_decide
cat "$aai_err" >&2
aai_msg=$(aai_json <"$aai_err") || aai_msg=
aai_msg=${aai_msg:-blocked by a hook that exited 2}
case $aai_status in
` + reply + "esac\n"
	return decisionWrapperShebang + WithHeader(body, FormatShell)
}

// exitCodeReply keeps stdout and the exit code, for a target that reads
// exit 2 and stderr as Claude Code does. Only `decision: stdout` wraps a
// command there, and the decision has become exit 2 by now.
const exitCodeReply = `*)
  cat "$aai_out"
  exit "$aai_status" ;;
`

// cursorReply turns the exit code into the reply a Cursor hook gives
// (cursor.com/docs/hooks), quoted in cursor's decisionReply notes before
// this moved here. On preToolUse, exit 2 becomes `permission: "deny"`
// with stderr as user_message ("shown in client when denied") and
// agent_message ("sent to agent when denied"); the docs read exit 2 as
// that deny but name no message for it. With --prompt, for
// beforeSubmitPrompt, it becomes `continue: false` with stderr as
// user_message. Exit 0 passes a JSON reply the command prints, or allows,
// since a permission hook blocks on output that is not JSON. Another exit
// stays, and Cursor fails open on it ("Hook failed, action proceeds"),
// as Claude Code reports a non-blocking error.
const cursorReply = `2)
  if [ -n "$aai_prompt" ]; then
    printf '{"continue":false,"user_message":"%s"}\n' "$aai_msg"
  else
    printf '{"permission":"deny","user_message":"%s","agent_message":"%s"}\n' "$aai_msg" "$aai_msg"
  fi ;;
0)
  aai_reply=$(cat "$aai_out")
  case ${aai_reply#"${aai_reply%%[![:space:]]*}"} in
  "{"*) printf '%s\n' "$aai_reply" ;;
  *)
    if [ -n "$aai_prompt" ]; then
      printf '{"continue":true}\n'
    else
      printf '{"permission":"allow"}\n'
    fi ;;
  esac ;;
*)
  exit "$aai_status" ;;
`

// copilotReply turns the exit code into what a Copilot preToolUse hook
// gives (docs.github.com/en/copilot/reference/hooks-reference). Exit 2
// stays, since it "denies the tool call", and its "stdout JSON is merged
// with the deny decision", so the reply adds stderr as
// permissionDecisionReason, the "Reason fed to the LLM when denying".
// Exit 0 passes stdout. Another exit stays, so Copilot denies the call
// where Claude Code reports an error and goes on: a guard that breaks
// keeps blocking instead of letting every call through.
const copilotReply = `2)
  printf '{"permissionDecision":"deny","permissionDecisionReason":"%s"}\n' "$aai_msg"
  exit 2 ;;
0)
  cat "$aai_out" ;;
*)
  exit "$aai_status" ;;
`

// geminiReply keeps the exit code, and at exit 2 prints the deny Gemini
// CLI reads: it takes stdout, or stderr when stdout is empty, as JSON
// whatever the exit code, so a reason that looks like JSON would
// otherwise read as no decision (hookrun's decideGemini, from gemini-cli
// hookRunner.ts).
const geminiReply = `2)
  printf '{"decision":"deny","reason":"%s"}\n' "$aai_msg"
  exit 2 ;;
*)
  cat "$aai_out"
  exit "$aai_status" ;;
`

// PortableHookWrapper renders the wrapper script for target.
func PortableHookWrapper(target string) string {
	switch target {
	case "gemini":
		return DecisionWrapper(geminiReply)
	case "cursor":
		return DecisionWrapper(cursorReply)
	case "copilot":
		return DecisionWrapper(copilotReply)
	}
	return DecisionWrapper(exitCodeReply)
}

// PortableHookCommand returns the command line for one command of hook h
// on target: inner, the command as the target writes it with any args
// folded in, or that run through the wrapper when h.WrapsCommand. rewrite
// maps a shared script path to the target's, the way the adapter maps
// the hook's own scripts, so the wrapper path resolves alike.
func PortableHookCommand(h spec.Entry, target, inner string, rewrite func(string) string) string {
	if !h.WrapsCommand(target) {
		return inner
	}
	return DecisionWrapperCommand(rewrite(portableHookWrapperSource), h.WrapperOptions(target), inner)
}

// WrapPortableHook is PortableHookCommand for a target whose hook
// scripts land in HookScriptsDir, as RewriteHookPath maps them.
func WrapPortableHook(h spec.Entry, target, inner string) string {
	return PortableHookCommand(h, target, inner, func(path string) string { return RewriteHookPath(path, target) })
}

// DecisionWrapperCommand is the command line that runs command through
// the wrapper at path: the path, the options, then command as one
// single-quoted word.
func DecisionWrapperCommand(path string, options []string, command string) string {
	return strings.Join(append(append([]string{path}, options...), ShellQuote(command)), " ")
}

// UnwrapDecisionCommand returns the options and the command a wrapper
// command line runs, and false when command does not run the wrapper:
// its first word ends in DecisionWrapperName, its next words are
// options, and its last is one single-quoted word.
func UnwrapDecisionCommand(command string) (path string, options []string, inner string, ok bool) {
	end := strings.Index(command, DecisionWrapperName+" ")
	if end < 0 {
		return "", nil, "", false
	}
	path = command[:end+len(DecisionWrapperName)]
	if base := filepath.Base(strings.Trim(path, `'"`)); base != DecisionWrapperName {
		return "", nil, "", false
	}
	rest := command[end+len(DecisionWrapperName)+1:]
	for strings.HasPrefix(rest, "--") {
		option, after, found := strings.Cut(rest, " ")
		if !found {
			return "", nil, "", false
		}
		options = append(options, option)
		rest = after
	}
	if len(rest) < 2 || rest[0] != '\'' || rest[len(rest)-1] != '\'' {
		return "", nil, "", false
	}
	inner = strings.ReplaceAll(rest[1:len(rest)-1], `'\''`, "'")
	if ShellQuote(inner) != rest {
		return "", nil, "", false
	}
	return path, options, inner, true
}

// writesPortableHookWrapper reports whether a hook in hooks runs its
// command through the wrapper on target: a portable hook sync wraps, or
// one whose command already calls the wrapper, as a hook imported from a
// synced file does. The wrapper is sync output that import leaves behind,
// so sync writes it again.
func writesPortableHookWrapper(hooks []spec.Entry, target string) bool {
	for _, h := range hooks {
		kind, _ := h.Meta["type"].(string)
		if kind != "" && kind != "command" {
			continue
		}
		commands, _ := hookSourceCommands(h, target)
		if h.WrapsCommand(target) && len(commands) > 0 {
			return true
		}
		for _, command := range commands {
			if _, _, _, ok := UnwrapDecisionCommand(StripCursorGuard(StripHookTargetExport(command, target))); ok {
				return true
			}
		}
	}
	return false
}

// PortableHookWrapperScript is the wrapper file for target in dir, or
// false when no hook in hooks needs it.
func PortableHookWrapperScript(hooks []spec.Entry, target, dir string) (HookScript, bool) {
	if !writesPortableHookWrapper(hooks, target) {
		return HookScript{}, false
	}
	return HookScript{Path: filepath.Join(dir, DecisionWrapperName), Body: []byte(PortableHookWrapper(target)), Mode: executablePerm}, true
}
