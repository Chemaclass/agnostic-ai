package codex

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// defaultEnvironmentFile is where the Codex app keeps a project's local
// environment: setup and cleanup scripts for worktrees and the actions in
// its top bar (developers.openai.com/codex/app/local-environments). The
// page does not show the file, so the layout follows what the app writes
// (`[setup]`, `[setup.win32]`, `[cleanup]`, `[[actions]]`).
const defaultEnvironmentFile = ".codex/environments/environment.toml"

// defaultActionIcon is the icon the app gives an action that names none.
const defaultActionIcon = "run"

// environmentSpecKeys are the environment spec keys the Codex emit reads
// or notes itself, plus the spec's identity fields.
var environmentSpecKeys = map[string]bool{
	"name": true, "description": true, "scope": true, "setup": true, "setup-windows": true,
	"cleanup": true, "dev-commands": true, "install": true, "terminals": true,
}

// actionFieldsWithoutEffect are the dev-command fields a Codex action has
// no key for, with the reason each note gives.
var actionFieldsWithoutEffect = []struct{ field, reason string }{
	{"port", "a Codex action has no port field"},
	{"auto-port", "a Codex action has no port field"},
	{"env", "a Codex action has no env field; set variables in the command"},
	{"url", "a Codex action has no url field"},
}

// environmentDoc is the merged environment: as with every environment
// field, the last spec that sets one wins.
type environmentDoc struct {
	name                string
	setup, setupWindows string
	cleanup             string
	devCommands         []any
}

// emitEnvironment writes the environment specs to
// .codex/environments/environment.toml: `setup` and `cleanup` become the
// `[setup]` and `[cleanup]` scripts, `setup-windows` the `[setup.win32]`
// script, and `dev-commands` the `[[actions]]` in the app's top bar.
func emitEnvironment(sess *emit.Session, envs []spec.Entry, cfg *config.Config, dryRun bool) error {
	if len(envs) == 0 {
		return nil
	}
	noteEnvironmentNoOps(envs)
	doc := mergeEnvironments(envs)
	var sb strings.Builder
	sb.WriteString("version = 1\n")
	emit.WriteTOMLString(&sb, "name", doc.name)
	writeScript(&sb, "setup", doc.setup)
	writeScript(&sb, "setup.win32", doc.setupWindows)
	writeScript(&sb, "cleanup", doc.cleanup)
	actions := 0
	for _, c := range doc.devCommands {
		m, _ := c.(map[string]any)
		name, _ := m["name"].(string)
		command := actionCommand(m["command"])
		if name == "" || command == "" {
			continue
		}
		cwd, _ := m["cwd"].(string)
		command = commandInDir(cwd, command)
		icon, _ := m["icon"].(string)
		if icon == "" {
			icon = defaultActionIcon
		}
		sb.WriteString("\n[[actions]]\n")
		emit.WriteTOMLString(&sb, "name", name)
		emit.WriteTOMLString(&sb, "icon", icon)
		writeValue(&sb, "command", command)
		actions++
	}
	if doc.setup == "" && doc.setupWindows == "" && doc.cleanup == "" && actions == 0 {
		return nil
	}
	path := emit.OutputEnvironmentFile(cfg, target, defaultEnvironmentFile)
	return sess.WriteFile(path, emit.WithHeader(sb.String(), emit.FormatTOML), dryRun)
}

func mergeEnvironments(envs []spec.Entry) environmentDoc {
	var doc environmentDoc
	for _, e := range envs {
		m := emit.ResolveMeta(e.Meta, target)
		name := e.Name
		if s, ok := m["name"].(string); ok && s != "" {
			name = s
		}
		if name != "" {
			doc.name = name
		}
		if v, ok := m["setup"]; ok {
			doc.setup = scriptText(v)
		}
		if v, ok := m["setup-windows"]; ok {
			doc.setupWindows = scriptText(v)
		}
		if v, ok := m["cleanup"]; ok {
			doc.cleanup = scriptText(v)
		}
		if v, ok := m["dev-commands"]; ok {
			doc.devCommands, _ = v.([]any)
		}
	}
	return doc
}

// scriptText reads a setup or cleanup field written as one command or a
// list, as the script that runs each command on its own line.
func scriptText(v any) string {
	if s, ok := v.(string); ok {
		return strings.TrimRight(s, "\n")
	}
	return strings.Join(emit.StringSlice(v), "\n")
}

// writeScript writes `[table]` with its script. An empty script writes no
// table, so an action-only spec leaves `[setup]` out.
func writeScript(sb *strings.Builder, table, script string) {
	if script == "" {
		return
	}
	fmt.Fprintf(sb, "\n[%s]\n", table)
	writeValue(sb, "script", script)
}

// writeValue writes key as a basic string, or a multi-line one when the
// value spans lines.
func writeValue(sb *strings.Builder, key, value string) {
	if strings.Contains(value, "\n") {
		emit.WriteTOMLMultiline(sb, key, value)
		return
	}
	emit.WriteTOMLString(sb, key, value)
}

var plainShellWord = regexp.MustCompile(`^[A-Za-z0-9_@%+=:,./-]+$`)

// actionCommand reads a dev command as the shell text an action runs: a
// string as written, a list of words quoted where a shell needs it.
func actionCommand(v any) string {
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	words := emit.CommandArgv(v)
	for i, w := range words {
		if !plainShellWord.MatchString(w) {
			words[i] = emit.ShellQuote(w)
		}
	}
	return strings.Join(words, " ")
}

// commandInDir runs command from cwd, since a Codex action has no cwd
// field and runs from the project root. A multi-line script stops when
// the directory is missing rather than running from the root.
func commandInDir(cwd, command string) string {
	cwd = strings.TrimSpace(cwd)
	if cwd == "" || cwd == "." {
		return command
	}
	if !plainShellWord.MatchString(cwd) {
		cwd = emit.ShellQuote(cwd)
	}
	if strings.Contains(command, "\n") {
		return "cd " + cwd + " || exit 1\n" + command
	}
	return "cd " + cwd + " && " + command
}

// noteEnvironmentNoOps notes each environment field Codex has no place for.
func noteEnvironmentNoOps(envs []spec.Entry) {
	emit.NoteFieldNoOp(target, spec.KindEnvironment, "install", emit.EnvironmentsWithField(target, envs, "install"),
		"Codex has no install step; put the commands in setup")
	emit.NoteFieldNoOp(target, spec.KindEnvironment, "terminals", emit.EnvironmentsWithField(target, envs, "terminals"),
		"Codex has no terminal list; use dev-commands for actions")

	counts := map[string]int{}
	for _, e := range envs {
		for k := range emit.ResolveMeta(e.Meta, target) {
			if !environmentSpecKeys[k] {
				counts[k]++
			}
		}
	}
	for _, k := range slices.Sorted(maps.Keys(counts)) {
		emit.NoteFieldNoOp(target, spec.KindEnvironment, k, counts[k], "Codex has no file for it")
	}

	doc := mergeEnvironments(envs)
	for _, f := range actionFieldsWithoutEffect {
		n := 0
		for _, c := range doc.devCommands {
			if m, ok := c.(map[string]any); ok && m[f.field] != nil {
				n++
			}
		}
		emit.NoteFieldNoOp(target, spec.KindEnvironment, "dev-commands."+f.field, n, f.reason)
	}
}
