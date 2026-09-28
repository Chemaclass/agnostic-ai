package emit

import "strings"

// shellSyntax holds the characters that make a command need a shell:
// pipes, lists, redirects, subshells, expansion, globs, and comments.
const shellSyntax = "|&;<>()$`*?[]{}~#"

// CommandArgv reads a command written as one string or as a list into
// the argv a tool starts it with. A list is taken as written. A string of
// plain words splits on whitespace, honoring single quotes, double quotes,
// and backslash escapes; a string that needs a shell, such as a pipeline
// or a leading `VAR=value`, runs as `sh -c <command>`.
func CommandArgv(v any) []string {
	if s, ok := v.(string); ok {
		return stringArgv(strings.TrimSpace(s))
	}
	if argv := StringSlice(v); len(argv) > 0 {
		return argv
	}
	return nil
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
	escaped := false
	for _, r := range command {
		switch {
		case escaped:
			word.WriteRune(r)
			escaped = false
		case quote == '\'':
			if r == '\'' {
				quote = 0
			} else {
				word.WriteRune(r)
			}
		case quote == '"':
			switch r {
			case '"':
				quote = 0
			case '$', '`':
				return shell
			case '\\':
				escaped = true
			default:
				word.WriteRune(r)
			}
		case r == '\\':
			escaped, inWord = true, true
		case r == '\'' || r == '"':
			quote, inWord = r, true
		case r == ' ' || r == '\t' || r == '\n':
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
	return words
}
