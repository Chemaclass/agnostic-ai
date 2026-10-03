package emit

import "strings"

// DotSlashHookScript prefixes ./ to a command whose first word is a
// plain path into target's synced hook scripts directory. Crush runs a
// command as a script, with its shebang or shell fallback, only when it
// starts with ./, ../, or /; otherwise a script without a shebang exits 1
// on Unix and every .sh script exits 1 on Windows, so a guard never
// blocks. A user's own path stays as written: on Windows ./bin/guard
// would skip the PATHEXT lookup that finds bin/guard.exe.
func DotSlashHookScript(command, target string) string {
	word, _, _ := strings.Cut(command, " ")
	if !strings.HasPrefix(word, HookScriptsDir(target)+"/") {
		return command
	}
	if strings.IndexFunc(word, func(r rune) bool { return !isPlainPathRune(r) }) >= 0 {
		return command
	}
	return "./" + command
}

// UndoDotSlashHookScript drops the ./ only when DotSlashHookScript would
// add it back, so a command the user wrote with ./ survives a round trip.
func UndoDotSlashHookScript(command, target string) string {
	stripped, ok := strings.CutPrefix(command, "./")
	if !ok || DotSlashHookScript(stripped, target) != command {
		return command
	}
	return stripped
}

func isPlainPathRune(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("_./@%+,-", r)
}
