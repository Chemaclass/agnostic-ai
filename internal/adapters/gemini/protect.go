package gemini

import (
	"fmt"
	"maps"
	"path/filepath"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/protecthook"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// ProtectedPaths reports that Gemini CLI enforces protected paths with
// a generated BeforeTool hook. The policy engine has a deny rule for
// write_file and replace, but its workspace tier "is currently
// non-functional" (geminicli.com/docs/reference/policy-engine).
func (Adapter) ProtectedPaths() (enforcement, reason string) { return "hook", "" }

// protectHook runs the generated script before write_file and replace,
// the two tools Gemini CLI counts as edits (EDIT_TOOL_NAMES in
// packages/core/src/tools/tool-names.ts). The matcher is a regular
// expression tested against the tool name, so it is anchored. Gemini CLI
// loads project settings from the directory it starts in and runs hooks
// from there, under bash or PowerShell. The command reads the same in
// both, but on Windows it needs an sh on PATH; without one PowerShell
// fails the command with a status Gemini CLI does not read as a block.
func protectHook() spec.Entry {
	return spec.Entry{
		Kind: spec.KindHook,
		Name: "agnostic-ai-protect",
		Path: "settings (protected)",
		Meta: map[string]any{
			"event":   "BeforeTool",
			"matcher": "^(write_file|replace)$",
			"command": protectCommand(),
		},
	}
}

func protectCommand() string {
	return "sh " + emit.HookScriptsDir(target) + "/" + protecthook.ScriptName
}

// IsProtectHookCommand reports whether command runs the generated
// protect script, which import skips: its source is the settings spec.
func IsProtectHookCommand(command string) bool {
	return command == protectCommand()
}

// withoutProtectHook drops the protect handler from a hooks object sync
// wrote before. The merge keeps a hooks key sync no longer writes, and
// Gemini CLI blocks every edit when that handler's script is gone,
// since sh then exits 127. hooks stays as it was, so the caller can
// fingerprint the value the cleanup started from.
func withoutProtectHook(hooks map[string]any) (map[string]any, bool) {
	stale := false
	kept := map[string]any{}
	for event, raw := range hooks {
		definitions, ok := raw.([]any)
		if !ok {
			kept[event] = raw
			continue
		}
		var keptDefinitions []any
		for _, rawDefinition := range definitions {
			definition, ok := rawDefinition.(map[string]any)
			handlers, isList := definition["hooks"].([]any)
			if !ok || !isList {
				keptDefinitions = append(keptDefinitions, rawDefinition)
				continue
			}
			var keptHandlers []any
			for _, rawHandler := range handlers {
				if handler, ok := rawHandler.(map[string]any); ok && IsProtectHookCommand(fmt.Sprint(handler["command"])) {
					stale = true
					continue
				}
				keptHandlers = append(keptHandlers, rawHandler)
			}
			if len(keptHandlers) > 0 {
				cleaned := maps.Clone(definition)
				cleaned["hooks"] = keptHandlers
				keptDefinitions = append(keptDefinitions, cleaned)
			}
		}
		if len(keptDefinitions) > 0 {
			kept[event] = keptDefinitions
		}
	}
	return kept, stale
}

func emitProtectScript(sess *emit.Session, groups []spec.ProtectGroup, dryRun bool) error {
	if len(groups) == 0 {
		return nil
	}
	path := filepath.Join(emit.HookScriptsDir(target), protecthook.ScriptName)
	return sess.WriteExecutableFile(path, renderProtectScript(groups), dryRun)
}

// renderProtectScript writes a hook that reads the BeforeTool JSON on
// stdin and exits 2 with the reasons on stderr when tool_input.file_path
// is protected. Gemini CLI reads stderr as the deny reason when stdout
// is empty and blocks the call (hookRunner.ts, convertPlainTextToHookOutput).
// Exit 1 and a timeout only warn, so every failure exits 2. An ask
// decision blocks too and tells the agent to ask the user, as on Codex.
func renderProtectScript(groups []spec.ProtectGroup) string {
	return protecthook.Render(protecthook.Reader{Edits: "Gemini CLI write_file and replace edits", Tool: "Gemini CLI", Awk: toolInputReaderAwk}, groups)
}

// toolInputReaderAwk scans the payload as JSON tokens and keeps the
// value of every file_path key, plus the first cwd and tool_name.
// Decoding keeps structural quotes literal, so a key spelled inside a
// string value never matches. Only those values are copied: other
// strings, such as write_file content, stream past.
//
// Gemini CLI strips NUL bytes from file_path, and drops a leading @ and
// the slashes after it when the literal path does not exist, so the
// hook checks every spelling, with and without those slashes.
// replace also searches the workspace for a relative path that does not
// exist from the project root and edits the one file whose path ends
// with it (correctPath in packages/core/src/utils/pathCorrector.ts). The
// hook cannot tell which file that is, so it blocks and asks for the
// full path.
const toolInputReaderAwk = `function start() {
  want["file_path"] = 1; want["cwd"] = 1; want["tool_name"] = 1
}
function text(s,    n, parts, i) {
  n = split(s, parts, "\"")
  for (i = 1; i <= n; i++) {
    if (i > 1) { if (instr) closed(); else opened() }
    if (instr) absorb(parts[i]); else between(parts[i])
  }
}
function between(t) {
  gsub(/[ \t\r\n]+/, "", t)
  if (t == "") return
  if (st == 1 && t == ":" && (key in want)) st = 2
  else st = 0
}
function opened() {
  instr = 1
  cap = ""
  toolong = 0
  value = (st == 2)
}
function absorb(t) {
  if (!value) { if (length(cap) <= 16) cap = cap substr(t, 1, 17); return }
  if (length(cap) + length(t) > 4096) toolong = 1
  else cap = cap t
}
function closed() {
  instr = 0
  if (!value) { key = cap; st = 1; return }
  st = 0
  store(key, cap)
}
function store(k, v,    s) {
  if (toolong) { block("the " k " is longer than the 4096 bytes the hook checks. Use a shorter path."); return }
  v = restore(restore(v, "\002", "\""), "\001", "/")
  if (k == "cwd") { if (!havecwd) { havecwd = 1; addAlias(v) } return }
  if (k == "tool_name") { if (!havetool) { havetool = 1; tool = v } return }
  v = restore(v, "\003", "")
  if (v == "") return
  named[++nnamed] = v
  paths[++npaths] = v
  s = unprefixed(v)
  if (s == "") return
  paths[++npaths] = s
  if (v ~ /^@\//) paths[++npaths] = substr(v, 2)
}
function unprefixed(p) {
  if (p !~ /^@./) return ""
  p = substr(p, 2)
  sub(/^\/+/, "", p)
  return p
}
function exists(p,    r, l) {
  r = (getline l < p)
  close(p)
  return r >= 0
}
function finish(    i, p, s) {
  if (tool != "replace") return
  for (i = 1; i <= nnamed; i++) {
    p = drive(named[i])
    if (substr(p, 1, 1) == "/" || exists(p)) continue
    s = unprefixed(p)
    if (s != "" && exists(s)) continue
    block(p " does not exist from the project root, so Gemini CLI would search the workspace for a file that ends with it, and the hook cannot check which one. Give the path from the project root.")
  }
}
`
