package preview

import (
	"path/filepath"
	"regexp"
	"strings"
)

// passwordEnvNames are the environment variables a client reads its
// password from.
var passwordEnvNames = map[string]bool{"PGPASSWORD": true, "MYSQL_PWD": true, "SSHPASS": true}

var (
	commandSeparators = map[string]bool{";": true, "|": true, "||": true, "&&": true, "&": true}
	commandWrappers   = map[string]bool{"sudo": true, "env": true, "exec": true, "command": true, "time": true, "nohup": true, "nice": true}
	shellAssignment   = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)
)

// MaskCommandPasswords masks the passwords a one-line command passes as
// plain arguments: -p and --password to a MySQL client, -p to sshpass,
// the password in a curl user, and PGPASSWORD, MYSQL_PWD or SSHPASS set
// before the command. shown keeps each flag with Redacted for its value;
// rest drops the flag and value, so a credential check reads the rest of
// the command without finding the flag's name. A reference such as
// $DB_PASSWORD stays. words are separated by single spaces.
func MaskCommandPasswords(line string) (shown, rest string) {
	words := strings.Split(line, " ")
	var out, kept []string
	program := ""
	for i := 0; i < len(words); i++ {
		word := words[i]
		at, offset, found := -1, 0, false
		switch {
		case commandSeparators[word]:
			program = ""
		case program == "":
			if name, _, ok := strings.Cut(word, "="); ok && passwordEnvNames[name] {
				at, offset, found = i, len(name)+1, true
			} else if !shellAssignment.MatchString(word) && !commandWrappers[word] && !strings.HasPrefix(word, "-") {
				program = filepath.Base(word)
			}
		case mysqlClient(program):
			at, offset, found = mysqlPasswordAt(words, i)
		case program == "sshpass":
			switch {
			case word == "-p":
				at, found = i+1, true
			case strings.HasPrefix(word, "-p"):
				at, offset, found = i, 2, true
			case word == "-f" || word == "-d" || word == "-P":
				out, kept = append(out, word), append(kept, word)
				i++
				if i < len(words) {
					out, kept = append(out, words[i]), append(kept, words[i])
				}
				continue
			case !strings.HasPrefix(word, "-"):
				program = filepath.Base(word)
			}
		case program == "curl" || program == "curl.exe":
			at, offset, found = curlPasswordAt(words, i)
		}
		if end, offset, after, ok := passwordSpan(words, at, offset, found); ok {
			out = append(out, words[i:at]...)
			out = append(out, words[at][:offset]+Redacted+after)
			if after = strings.TrimLeft(after, `'"`); after != "" {
				kept = append(kept, after)
			}
			i = end
		} else {
			out, kept = append(out, word), append(kept, word)
		}
		if last := out[len(out)-1]; strings.HasSuffix(last, ";") || strings.HasSuffix(last, "&") || strings.HasSuffix(last, "|") {
			program = ""
		}
	}
	return strings.Join(out, " "), strings.Join(kept, " ")
}

// mysqlPasswordAt finds a MySQL client's password at words[i]: glued to
// -p or --password=, or the next word after a bare -p or --password.
func mysqlPasswordAt(words []string, i int) (at, offset int, found bool) {
	word := words[i]
	switch {
	case word == "-p" || word == "--password":
		return i + 1, 0, true
	case strings.HasPrefix(word, "--password="):
		return i, len("--password="), true
	case strings.HasPrefix(word, "-p"):
		return i, 2, true
	}
	return 0, 0, false
}

// curlPasswordAt finds the password after the first `:` of a curl user
// option's value. A user with no `:` makes curl ask for the password.
func curlPasswordAt(words []string, i int) (at, offset int, found bool) {
	offset, glued, ok := curlUserAt(words[i])
	if !ok {
		return 0, 0, false
	}
	at = i
	if !glued {
		at, offset = i+1, 0
	}
	if at >= len(words) {
		return 0, 0, false
	}
	colon := strings.Index(words[at][offset:], ":")
	if colon < 0 {
		return 0, 0, false
	}
	return at, offset + colon + 1, true
}

// passwordSpan reads the password starting at words[at][offset:]: to its
// closing quote, across words, when it or its option value opens with
// one, else to the end
// of the word or a shell separator. end is the last word it covers,
// offset moves past an opening quote, and after is the text that follows
// the password in the last word. A missing value, an option, a separator
// or a reference is no password.
func passwordSpan(words []string, at, offset int, found bool) (end, start int, after string, ok bool) {
	if !found || at >= len(words) {
		return 0, 0, "", false
	}
	value := words[at][offset:]
	if value == "" || commandSeparators[value] || offset == 0 && strings.HasPrefix(value, "-") {
		return 0, 0, "", false
	}
	quote := byte(0)
	switch {
	case value[0] == '\'' || value[0] == '"':
		quote, offset, value = value[0], offset+1, value[1:]
	case strings.Count(words[at][:offset], "'")%2 == 1:
		quote = '\''
	case strings.Count(words[at][:offset], `"`)%2 == 1:
		quote = '"'
	}
	if quote == 0 {
		cut := len(value)
		if i := strings.IndexAny(value, `;&|)'"`); i >= 0 {
			cut = i
		}
		if cut == 0 || PureReference(value[:cut]) {
			return 0, 0, "", false
		}
		return at, offset, value[cut:], true
	}
	text := strings.Join(append([]string{value}, words[at+1:]...), " ")
	closing := strings.IndexByte(text, quote)
	if closing < 0 {
		return len(words) - 1, offset, "", !PureReference(text)
	}
	if closing == 0 || quote == '"' && PureReference(text[:closing]) {
		return 0, 0, "", false
	}
	after = text[closing:]
	if space := strings.IndexByte(after, ' '); space >= 0 {
		after = after[:space]
	}
	return at + strings.Count(text[:closing], " "), offset, after, true
}
