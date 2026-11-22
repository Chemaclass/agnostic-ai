package preview

import (
	"path/filepath"
	"regexp"
	"slices"
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
// $DB_PASSWORD stays. ok is false when a password is passed in a way it
// could not mask, such as a value with a backslash or a program behind
// a wrapper it does not read, so the caller withholds the whole command.
// words are separated by single spaces.
func MaskCommandPasswords(line string) (shown, rest string, ok bool) {
	words := strings.Split(line, " ")
	var out, kept []string
	program := ""
	for i := 0; i < len(words); i++ {
		word := words[i]
		found := noPassword
		switch {
		case commandSeparators[word]:
			program = ""
		case program == "":
			if name, _, ok := strings.Cut(word, "="); ok && passwordEnvNames[name] {
				found = passwordAt{at: i, offset: len(name) + 1}
			} else if !shellAssignment.MatchString(word) && !commandWrappers[word] && !strings.HasPrefix(word, "-") {
				program = filepath.Base(word)
			}
		case mysqlClient(program):
			found = mysqlPasswordAt(words, i)
		case program == "sshpass":
			switch {
			case word == "-p":
				found = passwordAt{at: i + 1, anyValue: true}
			case strings.HasPrefix(word, "-p"):
				found = passwordAt{at: i, offset: 2}
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
			found = curlPasswordAt(words, i)
		}
		if end, offset, after, ok := passwordSpan(words, found); ok {
			out = append(out, words[i:found.at]...)
			out = append(out, words[found.at][:offset]+Redacted+after)
			if found.keep {
				kept = append(kept, words[found.at:end+1]...)
			} else if after = strings.TrimLeft(after, `'"`); after != "" {
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
	rest = strings.Join(kept, " ")
	return strings.Join(out, " "), rest, !passesPasswordIn(rest)
}

var (
	segmentSeparator = regexp.MustCompile(`[;&|]`)
	// redirection is a word that starts a shell redirection, such as
	// `>`, `<`, or `2>&1`, which is no password.
	redirection = regexp.MustCompile(`^[0-9]*[<>]`)
)

// passesPasswordIn reports whether text, a command with the passwords
// it masked taken out, still passes one: a MySQL client or sshpass with
// a literal after -p or --password anywhere, or a password variable set
// to a literal.
func passesPasswordIn(text string) bool {
	for _, segment := range segmentSeparator.Split(text, -1) {
		args := strings.Fields(segment)
		for i, arg := range args {
			args[i] = strings.Trim(arg, "'\"`")
		}
		for i, arg := range args {
			if name, value, ok := strings.Cut(arg, "="); ok && passwordEnvNames[name] && strings.Trim(value, "'\"") != "" && !notLiteral(strings.Trim(value, "'\"")) {
				return true
			}
			base := filepath.Base(arg)
			if base == "sshpass" && sshpassPasswordAt(args[i:]) > 0 {
				return true
			}
			if strings.HasPrefix(base, "mysql") || strings.HasPrefix(base, "mariadb") {
				if slices.ContainsFunc(args[i+1:], mysqlPasswordArg) || mysqlSpacedPassword(args[i+1:]) {
					return true
				}
			}
		}
	}
	return false
}

// mysqlSpacedPassword reports whether args hold --password=, or -p or
// --password followed by a literal word.
func mysqlSpacedPassword(args []string) bool {
	for i, arg := range args {
		if value, ok := strings.CutPrefix(arg, "--password="); ok && value != "" && !notLiteral(value) {
			return true
		}
		if (arg == "-p" || arg == "--password") && i+1 < len(args) {
			next := args[i+1]
			if !strings.HasPrefix(next, "-") && !redirection.MatchString(next) && !notLiteral(next) {
				return true
			}
		}
	}
	return false
}

// passwordAt is where a password may start: words[at][offset:]. at is
// -1 when there is none. anyValue lets it start with `-`, and keep leaves
// the value in rest, as a word the command may read as something else.
type passwordAt struct {
	at, offset     int
	anyValue, keep bool
}

var noPassword = passwordAt{at: -1}

// mysqlPasswordAt finds a MySQL client's password at words[i]: glued to
// -p or --password=, or the next word after --password. The word after a
// bare -p is the database to the client, but often a password typed by
// mistake: it is masked and still checked.
func mysqlPasswordAt(words []string, i int) passwordAt {
	word := words[i]
	switch {
	case word == "-p":
		return passwordAt{at: i + 1, keep: true}
	case word == "--password":
		return passwordAt{at: i + 1}
	case strings.HasPrefix(word, "--password="):
		return passwordAt{at: i, offset: len("--password=")}
	case strings.HasPrefix(word, "-p"):
		return passwordAt{at: i, offset: 2}
	}
	return noPassword
}

// curlPasswordAt finds the password after the first `:` of a curl user
// option's value. A user with no `:` makes curl ask for the password.
func curlPasswordAt(words []string, i int) passwordAt {
	offset, glued, ok := curlUserAt(words[i])
	if !ok {
		return noPassword
	}
	at := i
	if !glued {
		at, offset = i+1, 0
	}
	if at >= len(words) {
		return noPassword
	}
	colon := strings.Index(words[at][offset:], ":")
	if colon < 0 {
		return noPassword
	}
	return passwordAt{at: at, offset: offset + colon + 1}
}

// closingQuote is a closing quote with only shell separators after it.
var closingQuote = regexp.MustCompile(`^['"][;&|)]*$`)

// passwordSpan reads the password at p: to its closing quote, across
// words, when it or its option value opens with one, else to the end of
// the word or a shell separator. end is the last word it covers, start
// moves past an opening quote, and after is the text that follows the
// password in the last word. A missing value, an option, a separator, a
// reference or command substitution, or a value it cannot read whole is
// no password.
func passwordSpan(words []string, p passwordAt) (end, start int, after string, ok bool) {
	if p.at < 0 || p.at >= len(words) {
		return 0, 0, "", false
	}
	offset := p.offset
	value := words[p.at][offset:]
	if value == "" || commandSeparators[value] || !p.anyValue && p.offset == 0 && strings.HasPrefix(value, "-") || notLiteral(value) || redirection.MatchString(value) {
		return 0, 0, "", false
	}
	quote := byte(0)
	switch {
	case value[0] == '\'' || value[0] == '"':
		quote, offset, value = value[0], offset+1, value[1:]
	case strings.Count(words[p.at][:offset], "'")%2 == 1:
		quote = '\''
	case strings.Count(words[p.at][:offset], `"`)%2 == 1:
		quote = '"'
	}
	if quote == 0 {
		cut := len(value)
		if i := strings.IndexAny(value, `;&|)'"\`); i >= 0 {
			cut = i
		}
		if cut == 0 || strings.ContainsAny(value[cut:], `'"\`) || PureReference(value[:cut]) {
			return 0, 0, "", false
		}
		return p.at, offset, value[cut:], true
	}
	text := strings.Join(append([]string{value}, words[p.at+1:]...), " ")
	closing := strings.IndexByte(text, quote)
	secret := text
	if closing >= 0 {
		secret = text[:closing]
	}
	if secret == "" || strings.Contains(secret, `\`) || quote == '"' && notLiteral(secret) {
		return 0, 0, "", false
	}
	if closing < 0 {
		return len(words) - 1, offset, "", true
	}
	after = text[closing:]
	if space := strings.IndexByte(after, ' '); space >= 0 {
		after = after[:space]
	}
	if !closingQuote.MatchString(after) {
		return 0, 0, "", false
	}
	return p.at + strings.Count(secret, " "), offset, after, true
}

// notLiteral reports whether value is a reference or a command
// substitution, which hold no password as written.
func notLiteral(value string) bool {
	return PureReference(value) || strings.HasPrefix(value, "$(") || strings.HasPrefix(value, "`")
}
