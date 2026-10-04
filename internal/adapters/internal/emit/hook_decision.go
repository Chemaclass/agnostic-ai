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
// decision protocol: at exit 0, a JSON object on stdout with "decision"
// "allow", "deny", or "ask" and an optional "reason". deny and ask become
// exit 2 with the reason on stderr; ask blocks too, since not every tool
// can ask the user. allow drops the object and goes on as a plain exit 0,
// so it never grants more than the tool's own rules do. An object that
// names a decision the wrapper cannot read blocks, so a typo never lets a
// call through. The reason is decoded for \", \\, \/, \n, and \t; a \u
// escape stays as written. Other stdout passes through unchanged.
const hookStdoutDecision = `aai_unescape() { LC_ALL=C awk 'BEGIN { ORS = "" } { s = $0; out = ""; while ((i = index(s, "\\")) > 0) { c = substr(s, i + 1, 1); if (c == "n") c = "\n"; else if (c == "t") c = "\t"; else if (c == "r" || c == "b" || c == "f") c = ""; else if (c == "u") c = "\\u"; out = out substr(s, 1, i - 1) c; s = substr(s, i + 2) } print out s }'; }
aai_decide() {
  [ "$aai_status" -eq 0 ] || return 0
  aai_flat=$(tr '\n\r\t' '   ' <"$aai_out")
  printf '%s\n' "$aai_flat" | grep -Eq '"decision"[[:space:]]*:' || return 0
  aai_verdict=$(printf '%s\n' "$aai_flat" | sed -n -E 's/.*"decision"[[:space:]]*:[[:space:]]*"(allow|deny|ask)"[[:space:]]*[,}].*/\1/p')
  aai_reason=$(printf '%s\n' "$aai_flat" | sed -n -E 's/.*"reason"[[:space:]]*:[[:space:]]*"(([^"\\]|\\.)*)".*/\1/p' | aai_unescape)
  : >"$aai_out"
  case $aai_verdict in
  allow) ;;
  deny | ask)
    printf '%s\n' "${aai_reason:-blocked by the hook decision}" >>"$aai_err"
    aai_status=2 ;;
  *)
    printf '%s\n' 'blocked: the hook printed a decision that is not "allow", "deny", or "ask"' >>"$aai_err"
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
aai_out=$(mktemp) || exit 1
aai_err=$(mktemp) || { rm -f "$aai_out"; exit 1; }
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

// PortableHookWrapper renders the wrapper script for target.
func PortableHookWrapper(target string) string {
	switch target {
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
// command through the wrapper on target.
func writesPortableHookWrapper(hooks []spec.Entry, target string) bool {
	for _, h := range hooks {
		kind, _ := h.Meta["type"].(string)
		if h.WrapsCommand(target) && (kind == "" || kind == "command") && len(HookCommands(h.Meta["command"])) > 0 {
			return true
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
