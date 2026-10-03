package emit

import (
	"strings"
	"unicode"
)

// DotSlashSyncedHookScript prefixes ./ to rewritten when source starts
// with a plain path into the shared scripts directory, the one script
// sync copies into target's hooks directory. Crush runs a command as a
// script, with its shebang or shell fallback, only when it starts with
// ./, ../, or /; otherwise a script without a shebang exits 1 on Unix and
// every .sh script exits 1 on Windows, so a guard never blocks. Any other
// command stays as written: on Windows ./.crush/hooks/guard would skip
// the PATHEXT lookup that finds guard.exe.
func DotSlashSyncedHookScript(source, rewritten, target string) string {
	sourceWord, _, _ := strings.Cut(source, " ")
	word, _, _ := strings.Cut(rewritten, " ")
	if !strings.HasPrefix(sourceWord, agnosticScriptsDir+"/") || !strings.HasPrefix(word, HookScriptsDir(target)+"/") {
		return rewritten
	}
	if strings.IndexFunc(word, func(r rune) bool { return !isPlainPathRune(r) }) >= 0 {
		return rewritten
	}
	return "./" + rewritten
}

func isPlainPathRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("_./@%+,-", r)
}
