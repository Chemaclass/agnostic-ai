package emit

import "strings"

// HookJSONEscape defines aai_json, which writes its stdin as the body of
// a JSON string. It runs in the C locale, so bytes that are not UTF-8
// pass through instead of stopping awk; it drops control bytes other than
// tab and newline, and writes `{` and `}` as \u escapes, since Cline's VS
// Code extension's fallback parser counts braces without reading quotes
// (hook-factory.ts:366-466).
const HookJSONEscape = `aai_json() { LC_ALL=C tr -d '\000-\010\013-\037\177' | LC_ALL=C awk 'BEGIN { ORS = "" } { gsub(/\\/, "\\\\"); gsub(/"/, "\\\""); gsub(/\t/, "\\t"); gsub(/{/, "\\u007b"); gsub(/}/, "\\u007d"); if (NR > 1) printf "\\n"; print }'; }
`

// DecisionWrapperName is the file name of the script sync puts in front
// of a portable hook command on a target that reads a block another way
// than exit 2 and stderr.
const DecisionWrapperName = "agnostic-ai-portable-hook.sh"

// decisionWrapperShebang runs the wrapper with bash, as Claude Code runs a
// hook command, whatever shell the target starts the command line with.
// The provenance comment sits on the line below it, where header.Leads
// looks, so import leaves the wrapper behind.
const decisionWrapperShebang = "#!/usr/bin/env bash\n"

// DecisionWrapper renders that script. It runs its first argument with
// `bash -c`, passing stdin and the environment through, and replays the
// command's stderr. reply is a shell `case` over $aai_status, the
// command's exit code, that writes the target's reply: $aai_out names the
// file holding the command's stdout, and $aai_msg is its stderr as the
// body of a JSON string.
func DecisionWrapper(reply string) string {
	body := `aai_out=$(mktemp) || exit 1
aai_err=$(mktemp) || { rm -f "$aai_out"; exit 1; }
trap 'rm -f "$aai_out" "$aai_err"' EXIT
` + HookJSONEscape + `bash -c "$1" >"$aai_out" 2>"$aai_err"
aai_status=$?
cat "$aai_err" >&2
aai_msg=$(aai_json <"$aai_err") || aai_msg=
aai_msg=${aai_msg:-blocked by a hook that exited 2}
case $aai_status in
` + reply + "esac\n"
	return decisionWrapperShebang + WithHeader(body, FormatShell)
}

// DecisionWrapperCommand is the command line that runs command through
// the wrapper at path: the path, then command as one single-quoted word.
func DecisionWrapperCommand(path, command string) string {
	return path + " " + ShellQuote(command)
}

// UnwrapDecisionCommand returns the command DecisionWrapperCommand
// wrapped, and false when command does not run the wrapper at path.
func UnwrapDecisionCommand(path, command string) (string, bool) {
	quoted, ok := strings.CutPrefix(command, path+" ")
	if !ok || len(quoted) < 2 || quoted[0] != '\'' || quoted[len(quoted)-1] != '\'' {
		return "", false
	}
	inner := strings.ReplaceAll(quoted[1:len(quoted)-1], `'\''`, "'")
	if ShellQuote(inner) != quoted {
		return "", false
	}
	return inner, true
}
