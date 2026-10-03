package copilot

import (
	"path"
	"strings"
)

// A Copilot command hook runs from `cwd`, so a repository-relative
// script path in `command` or `args` has to be written relative to
// that directory. The hook file is OS-independent JSON, hence the
// slash semantics of package path.

// cleanHookCwd returns cwd as a clean repository-relative directory,
// or false when it is unset, absolute, variable-based, or leaves the
// repository, in which case the script path stays as written.
func cleanHookCwd(cwd string) (string, bool) {
	if cwd == "" || strings.ContainsAny(cwd, `\$~`) || path.IsAbs(cwd) || (len(cwd) > 1 && cwd[1] == ':') {
		return "", false
	}
	clean := path.Clean(cwd)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", false
	}
	return clean, true
}

// syncedScriptDirs are the hook script locations sync writes or leaves
// in place; any other path is the user's own argument and stays as is.
var syncedScriptDirs = []string{".github/hooks/scripts/", ".agnostic-ai/hooks/"}

var hookInterpreters = map[string]bool{
	"sh": true, "bash": true, "zsh": true, "node": true,
	"python": true, "python3": true, "pwsh": true, "powershell": true,
}

func isSyncedScript(repoPath string) bool {
	for _, dir := range syncedScriptDirs {
		if strings.HasPrefix(repoPath, dir) {
			return true
		}
	}
	return false
}

// scriptWord reports whether word is a plain relative path: it has a
// slash and is not absolute, a Windows drive or UNC path, `$`-prefixed,
// a flag, or quoted.
func scriptWord(word string) bool {
	if !strings.Contains(word, "/") || strings.ContainsAny(word, "\"'`\\$;|&<>()*?") {
		return false
	}
	if len(word) > 1 && word[1] == ':' {
		return false
	}
	return !strings.HasPrefix(word, "/") && !strings.HasPrefix(word, "-") && !strings.HasPrefix(word, "~")
}

func relativeToCwd(script, cwd string) (string, bool) {
	clean := path.Clean(script)
	if !isSyncedScript(clean) {
		return "", false
	}
	if rest, ok := strings.CutPrefix(clean, cwd+"/"); ok {
		return "./" + rest, true
	}
	return strings.Repeat("../", strings.Count(cwd, "/")+1) + clean, true
}

func relativeToRepository(script, cwd string) (string, bool) {
	clean := path.Join(cwd, script)
	if !isSyncedScript(clean) {
		return "", false
	}
	return clean, true
}

type pathMap func(script, cwd string) (string, bool)

// mapCommand maps the script word of a shell command line: its first
// word, or the second when the first is a recognized interpreter. A
// line with no such word comes back unchanged.
func mapCommand(command, cwd string, convert pathMap) string {
	fields := strings.Fields(command)
	i := 0
	if len(fields) > 1 && !scriptWord(fields[0]) && hookInterpreters[fields[0]] {
		i = 1
	}
	if len(fields) == 0 || !scriptWord(fields[i]) {
		return command
	}
	converted, ok := convert(fields[i], cwd)
	if !ok {
		return command
	}
	start := 0
	for n := 0; n < i; n++ {
		start += strings.Index(command[start:], fields[n]) + len(fields[n])
	}
	start += strings.Index(command[start:], fields[i])
	return command[:start] + converted + command[start+len(fields[i]):]
}

func mapExec(exec string, args []string, cwd string, convert pathMap) (string, []string) {
	if scriptWord(exec) {
		if converted, ok := convert(exec, cwd); ok {
			exec = converted
		}
		return exec, args
	}
	if len(args) > 0 && hookInterpreters[exec] && scriptWord(args[0]) {
		if converted, ok := convert(args[0], cwd); ok {
			args = append([]string{converted}, args[1:]...)
		}
	}
	return exec, args
}

// ScriptForCwd rewrites the script path of a shell command line to be
// relative to a hook's cwd.
func ScriptForCwd(command, cwd string) string {
	clean, ok := cleanHookCwd(cwd)
	if !ok {
		return command
	}
	return mapCommand(command, clean, relativeToCwd)
}

// ExecForCwd is ScriptForCwd for the exec form.
func ExecForCwd(exec string, args []string, cwd string) (string, []string) {
	clean, ok := cleanHookCwd(cwd)
	if !ok {
		return exec, args
	}
	return mapExec(exec, args, clean, relativeToCwd)
}

// ScriptFromCwd undoes ScriptForCwd, used when importing a native file.
func ScriptFromCwd(command, cwd string) string {
	clean, ok := cleanHookCwd(cwd)
	if !ok {
		return command
	}
	return mapCommand(command, clean, relativeToRepository)
}

// ExecFromCwd undoes ExecForCwd.
func ExecFromCwd(exec string, args []string, cwd string) (string, []string) {
	clean, ok := cleanHookCwd(cwd)
	if !ok {
		return exec, args
	}
	return mapExec(exec, args, clean, relativeToRepository)
}
