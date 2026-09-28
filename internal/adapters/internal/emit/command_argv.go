package emit

import (
	"fmt"
	"strings"
)

// shellSyntax holds the characters that make a command need a shell:
// pipes, lists, redirects, subshells, expansion, globs, and comments.
const shellSyntax = "|&;<>()$`*?[]{}~#!"

// shellBuiltins are the first words only a shell can run: builtins that
// change the shell itself, and keywords. As an executable they fail.
var shellBuiltins = map[string]bool{
	"cd": true, "exec": true, "export": true, "source": true, ".": true, "set": true,
	"unset": true, "eval": true, "if": true, "for": true, "while": true, "until": true, "case": true,
}

// CommandArgv reads a command written as one string or as a list into
// the argv a tool starts it with. A list is taken as written, a number or
// bool in it as its text. A string of
// plain words splits on whitespace, honoring single quotes, double quotes,
// and backslash escapes; a string that needs a shell, such as a pipeline,
// several lines, a leading `VAR=value`, or a builtin such as `cd`, runs
// as `sh -c <command>`.
func CommandArgv(v any) []string {
	var argv []string
	if s, ok := v.(string); ok {
		argv = stringArgv(strings.TrimSpace(s))
	} else {
		argv = listArgv(v)
	}
	if len(argv) == 0 || strings.TrimSpace(argv[0]) == "" {
		return nil // no executable to start
	}
	return argv
}

// listArgv reads a command list word by word. YAML reads `7` or `true`
// as a number or bool, so a scalar is kept as its text; a list holding a
// nested list, a mapping, or null is not a command.
func listArgv(v any) []string {
	list, ok := v.([]any)
	if !ok {
		return StringSlice(v)
	}
	argv := make([]string, 0, len(list))
	for _, item := range list {
		switch item.(type) {
		case string, int, int64, float64, bool:
			argv = append(argv, fmt.Sprint(item))
		default:
			return nil
		}
	}
	if len(argv) == 0 {
		return nil
	}
	return argv
}

func stringArgv(command string) []string {
	if command == "" {
		return nil
	}
	shell := []string{"sh", "-c", command}
	var words []string
	var word strings.Builder
	inWord := false
	var quote rune
	escaped, pendingBackslash := false, false
	for _, r := range command {
		switch {
		case escaped:
			if r == '\n' {
				return shell // a line continuation
			}
			word.WriteRune(r)
			escaped = false
		case quote == '\'':
			if r == '\'' {
				quote = 0
			} else {
				word.WriteRune(r)
			}
		case quote == '"' && pendingBackslash:
			// Inside double quotes a backslash escapes only `"` and `\`
			// here; `$`, a backtick, and a newline already need a shell.
			if r != '"' && r != '\\' {
				word.WriteRune('\\')
			}
			word.WriteRune(r)
			pendingBackslash = false
		case quote == '"':
			switch r {
			case '"':
				quote = 0
			case '$', '`', '\n':
				return shell
			case '\\':
				pendingBackslash = true
			default:
				word.WriteRune(r)
			}
		case r == '\\':
			escaped, inWord = true, true
		case r == '\'' || r == '"':
			quote, inWord = r, true
		case r == '\n' || r == '\r':
			return shell // a newline separates commands
		case r == ' ' || r == '\t':
			if inWord {
				words = append(words, word.String())
				word.Reset()
				inWord = false
			}
		case strings.ContainsRune(shellSyntax, r):
			return shell
		case r == '=' && len(words) == 0 && !strings.Contains(word.String(), "/"):
			return shell
		default:
			word.WriteRune(r)
			inWord = true
		}
	}
	if quote != 0 || escaped {
		return shell
	}
	if inWord {
		words = append(words, word.String())
	}
	if len(words) == 0 || shellBuiltins[words[0]] {
		return shell
	}
	return words
}
