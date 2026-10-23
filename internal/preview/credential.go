package preview

import (
	"bytes"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"mvdan.cc/sh/v3/syntax"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

var (
	queryParamPattern = regexp.MustCompile(`[?&;#]([^=&;?#\s'"]+)=([^&;?#\s]*)`)
	refDefaultPattern = regexp.MustCompile(`\$?\$\{[A-Za-z_][A-Za-z0-9_]*:-([^{}]*)\}`)
	plainRefPattern   = regexp.MustCompile(`\$?\$\{[A-Za-z_][A-Za-z0-9_]*\}`)
)

// RefSentinel stands in for a plain `${NAME}` or `$${NAME}` while the
// detector reads a value.
const RefSentinel = "\x00"

func LiteralCredential(value string) bool {
	return strings.Trim(strings.ReplaceAll(value, RefSentinel, ""), `'"`) != ""
}

// DetectorText writes each `${NAME:-default}` as its default, so the
// default is read in URL context, and each plain reference as
// RefSentinel.
func DetectorText(value string) string {
	for {
		expanded := refDefaultPattern.ReplaceAllString(value, "$1")
		if expanded == value {
			break
		}
		value = expanded
	}
	return plainRefPattern.ReplaceAllString(value, RefSentinel)
}

// CredentialDetected reports whether value holds a literal credential
// anywhere, reference defaults and escaped references included: a URL
// password or credential query or fragment parameter, a token by its
// prefix, a scheme-less `user:password@host`, a `NAME=value` or a
// `--name value` whose name ends in a credential word, or a literal
// `Bearer` token. It errs toward finding one, since a
// false find only leaves a server out with a warning. A password is a
// literal after a `:` and before an `@` in the text from `://` to the
// next `/`, `?`, or `#`. An `@` after that counts too, unless the text
// before it is a host with a port, as in `https://host:8443/@scope`. It
// reads value as written and again with shell quotes and backslashes
// removed, so `?'token=x'` counts too, and each of those again with
// percent-encoded separators decoded once.
func CredentialDetected(value string) bool {
	for _, text := range []string{DetectorText(value), DetectorText(shellUnquote.Replace(value))} {
		for _, t := range []string{text, encodedSeparators.Replace(text)} {
			if URLCredentialIn(t) || TokenIn(t) || userPasswordIn(t) || assignmentIn(t) || bearerIn(t) || flagIn(t) || keyValueIn(t) {
				return true
			}
		}
	}
	return false
}

var (
	shellUnquote      = strings.NewReplacer(`'`, "", `"`, "", `\`, "")
	encodedSeparators = strings.NewReplacer("%26", "&", "%3D", "=", "%3d", "=", "%3F", "?", "%3f", "?", "%23", "#")
)

// tokenPrefixes are token formats that name their issuer: GitHub,
// GitLab, Slack, npm, Google API keys, Stripe, OpenAI and Anthropic
// `sk-`, AWS access key ids, and JWTs.
const tokenPrefixes = `gh[pousr]_[A-Za-z0-9_]{8,}|github_pat_[A-Za-z0-9_]{8,}|glpat-[A-Za-z0-9_\-]{8,}|xox[bpa]-[A-Za-z0-9\-]{8,}|xapp-[A-Za-z0-9\-]{8,}|npm_[A-Za-z0-9]{30,}|AIza[A-Za-z0-9_\-]{35}|(?:sk|rk)_live_[A-Za-z0-9]{8,}|sk_test_[A-Za-z0-9]{8,}|sk-(?:proj|ant)-[A-Za-z0-9_\-]{8,}|sk-[A-Za-z0-9]{20,}|AKIA[0-9A-Z]{16}|eyJ[A-Za-z0-9_\-]+\.eyJ[A-Za-z0-9_\-]+\.[A-Za-z0-9_\-]+`

var (
	tokenPattern      = regexp.MustCompile(`^(?:` + tokenPrefixes + `)$`)
	embeddedToken     = regexp.MustCompile(`(?:^|[^A-Za-z0-9_])(` + tokenPrefixes + `)(?:[^A-Za-z0-9]|$)`)
	userPasswordAt    = regexp.MustCompile(`(?:^|[\s'"=,])([A-Za-z0-9._%+\-\x00]+):([^\s@/:'"]+)@(\S*)`)
	assignmentPattern = regexp.MustCompile(`(?:^|[^A-Za-z0-9_\x00])([A-Za-z_][A-Za-z0-9_\-]*)=([^\s;&|#]+)`)
	shellVariable     = regexp.MustCompile(`^\$[A-Za-z_][A-Za-z0-9_]*$`)
	bearerPattern     = regexp.MustCompile(`(?i)(?:^|[^A-Za-z0-9])bearer\s+([^\s'",}\\]+)`)
	plainValue        = regexp.MustCompile(`^(?:[/~@]|\.\.?/|[A-Za-z]:[\\/]|(?i:true|false|yes|no|on|off)$|[0-9]+$|[a-z]+(?:-[a-z]+)+$)|(?i)\.(?:json|txt|pem|key|env|ya?ml|toml|crt|p12|db)$`)
	headerLine        = regexp.MustCompile(`(?m)^\s*([A-Za-z0-9_\-]+)\s*:[ \t]*(\S.*?)\s*$`)
	jsonPair          = regexp.MustCompile(`"([^"]+)"\s*:\s*"([^"]*)"`)
	schemeWord        = regexp.MustCompile(`^[A-Za-z]+\s+`)
	imageDigestSuffix = regexp.MustCompile(`^sha[0-9]+:`)
)

// Token reports whether value is a whole token by its prefix. An
// `sk-` token other than `sk-proj-` or `sk-ant-` needs a digit, so a
// package name such as `sk-learn-mcp-server-tools` is not one.
func Token(value string) bool {
	if !tokenPattern.MatchString(value) {
		return false
	}
	if strings.HasPrefix(value, "sk-proj-") || strings.HasPrefix(value, "sk-ant-") || !strings.HasPrefix(value, "sk-") {
		return true
	}
	return strings.ContainsAny(value, "0123456789")
}

// SecretValue reports whether text, a value in detector text, is a
// literal that may be a secret: not a reference (after an optional
// scheme word, as in `Bearer ${TOKEN}`), a `$NAME` expansion, a file
// path or name, an `@` value, a boolean, a number, or lower-case words
// joined by dashes such as `streamable-http`. A value in its own
// argument after a flag must not start with `-`, and after a weak name
// it needs at least 8 characters, so `--require-api-key run` passes.
func SecretValue(text string, separate bool, strength CredentialStrength) bool {
	v := strings.Trim(text, `'"`)
	if !LiteralCredential(schemeWord.ReplaceAllString(v, "")) || strings.HasPrefix(v, "$") || plainValue.MatchString(v) {
		return false
	}
	return !separate || !strings.HasPrefix(v, "-") && (strength == StrongCredential || len(v) >= 8)
}

// RefOnly reports whether value is only references, after an
// optional scheme word such as `Bearer ${TOKEN}`.
func RefOnly(value string) bool {
	value = schemeWord.ReplaceAllString(strings.TrimSpace(value), "")
	return spec.OnlyEnvRefs(value) || shellVariable.MatchString(value)
}

// bearerIn finds a literal `Bearer` token, as in `-H
// 'Authorization: Bearer ...'` or a JSON headers value. The token needs
// a digit, one of `-._~+/=`, or at least 20 characters, so prose such as
// "uses bearer authentication" does not count.
func bearerIn(text string) bool {
	for _, token := range BearerTokens(text) {
		if strings.Contains(token, RefSentinel) || strings.HasPrefix(token, "$") {
			continue
		}
		if len(token) >= 20 || strings.ContainsAny(token, "0123456789-._~+/=") {
			return true
		}
	}
	return false
}

// BearerTokens returns the word after each `Bearer` in text.
func BearerTokens(text string) []string {
	var tokens []string
	for _, m := range bearerPattern.FindAllStringSubmatch(text, -1) {
		tokens = append(tokens, m[1])
	}
	return tokens
}

// StripSchemeWord drops a leading one-word scheme such as `Bearer `.
func StripSchemeWord(value string) string { return schemeWord.ReplaceAllString(value, "") }

// keyValueIn finds a line `Name: value` or a JSON pair `"name":
// "value"` whose name is a credential key and whose value is a literal,
// as in an `X-Api-Key: ...` headers value or `{"apiKey": "..."}`.
func keyValueIn(text string) bool {
	for _, pattern := range []*regexp.Regexp{headerLine, jsonPair} {
		for _, m := range pattern.FindAllStringSubmatch(text, -1) {
			if CredentialKey(m[1]) && SecretValue(m[2], false, WeakCredential) {
				return true
			}
		}
	}
	return false
}

// flagIn finds a `--name value` or `--name=value` among the shell
// words of text whose name is a credential and whose value is a secret.
func flagIn(text string) bool {
	words := strings.Fields(text)
	for i, word := range words {
		name, value, hasValue := CredentialFlag(word)
		switch {
		case name == "":
		case hasValue && SecretValue(value, false, WeakCredential):
			return true
		case !hasValue && i+1 < len(words) && SecretValue(words[i+1], true, NameStrength(name, false)):
			return true
		}
	}
	return false
}

// CredentialHeader reports whether value, a `--header` or `-H`
// argument, is `Name: value` with a credential name and a literal value.
func CredentialHeader(value string) bool {
	name, v, ok := strings.Cut(value, ":")
	v = strings.TrimSpace(v)
	return ok && CredentialKey(strings.TrimSpace(name)) && !RefOnly(v) && SecretValue(DetectorText(v), false, WeakCredential)
}

func TokenIn(text string) bool {
	for _, m := range embeddedToken.FindAllStringSubmatch(text, -1) {
		if Token(m[1]) {
			return true
		}
	}
	return false
}

// userPasswordIn finds a scheme-less `user:password@host`, as in
// `root:pw@db:3306` or an scp-style `user:pw@host:path`. An image digest
// such as `node:20@sha256:...` is not one, and neither is a Windows path
// such as `C:\Users\me@corp\a.db`.
func userPasswordIn(text string) bool {
	for _, m := range userPasswordAt.FindAllStringSubmatch(text, -1) {
		drive := len(m[1]) == 1 && unicode.IsLetter(rune(m[1][0]))
		if !drive && LiteralCredential(m[2]) && !imageDigestSuffix.MatchString(m[3]) {
			return true
		}
	}
	return false
}

// assignmentIn finds a `NAME=value` whose name ends in a credential
// word, as in `TOKEN=x` inside a `sh -c` command. A shell expansion such
// as `$TOKEN` or `$(cat f)` is not a literal.
func assignmentIn(text string) bool {
	for _, m := range assignmentPattern.FindAllStringSubmatch(text, -1) {
		if CredentialKey(m[1]) && SecretValue(m[2], false, WeakCredential) {
			return true
		}
	}
	return false
}

func URLCredentialIn(text string) bool {
	for _, m := range queryParamPattern.FindAllStringSubmatch(text, -1) {
		if CredentialParam(m[1]) && LiteralCredential(m[2]) {
			return true
		}
	}
	for rest := text; ; {
		sep := strings.Index(rest, "://")
		if sep < 0 {
			return false
		}
		rest = rest[sep+3:]
		authority := rest[:indexAnyOrLen(rest, "/?#")]
		at := strings.LastIndex(authority, "@")
		if at < 0 {
			hostPort := authority
			if strings.HasPrefix(hostPort, "[") {
				hostPort = hostPort[indexAnyOrLen(hostPort, "]"):]
			}
			host, port, ok := strings.Cut(hostPort, ":")
			// `redis://:1234#5@host` hides an all-digit password before
			// `#`, or before `/` when no host precedes it.
			next := len(authority)
			hidden := ok && next < len(rest) && (rest[next] == '#' || host == "")
			if !hidden && (!ok || port != "" && strings.Trim(port, "0123456789"+RefSentinel) == "") {
				continue
			}
			if at = strings.Index(rest, "@"); at < 0 {
				continue
			}
		}
		if colon := strings.Index(rest[:at], ":"); colon >= 0 && LiteralCredential(rest[colon+1:at]) {
			return true
		}
	}
}

func indexAnyOrLen(s, chars string) int {
	if i := strings.IndexAny(s, chars); i >= 0 {
		return i
	}
	return len(s)
}

// CredentialFlag returns the name of a `--name` or `--name=value`
// argument whose name ends in a credential word, and the value after
// `=`, so `--api-key` counts and `--key-id`, `--token-file`, or
// `--no-token` does not.
func CredentialFlag(arg string) (name, value string, hasValue bool) {
	rest, ok := strings.CutPrefix(arg, "--")
	if !ok {
		return "", "", false
	}
	name, value, hasValue = strings.Cut(rest, "=")
	if !flagNamePattern.MatchString(name) || strings.HasPrefix(name, "no-") || !CredentialName(name) {
		return "", "", false
	}
	return name, value, hasValue
}

var flagNamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.\-]*$`)

// CredentialInRule reports whether a permission rule holds a literal
// credential. A command rule, in the neutral `shell(...)` or the Claude
// `Bash(...)` spelling, is also read as shell, nested `sh -c` included.
func CredentialInRule(rule string) bool {
	if CredentialDetected(rule) {
		return true
	}
	command, shell := strings.CutPrefix(rule, "shell(")
	if !shell {
		command, shell = strings.CutPrefix(rule, "Bash(")
	}
	if !shell {
		return false
	}
	return commandCredentialDetected(strings.TrimSuffix(command, ")"), 0)
}

// RedactRule returns rule as written, or, when it holds a credential,
// its tool name around Redacted, as in `shell(<redacted>)`.
func RedactRule(rule string) string {
	if !CredentialInRule(rule) {
		return rule
	}
	if name, _, ok := strings.Cut(rule, "("); ok && strings.HasSuffix(rule, ")") {
		return name + "(" + Redacted + ")"
	}
	return Redacted
}
func commandCredentialDetected(command string, depth int) bool {
	if depth >= 4 || CredentialDetected(command) {
		return true
	}
	file, err := syntax.NewParser().Parse(strings.NewReader(command), "")
	if err != nil {
		return Display("permission.sh", command).Hidden
	}
	credential := false
	syntax.Walk(file, func(node syntax.Node) bool {
		if credential {
			return false
		}
		if assignment, ok := node.(*syntax.Assign); ok {
			credential = assignment.Name != nil && CredentialKey(assignment.Name.Value) &&
				(assignment.Array != nil || literalHeaderValue(unescapedWord(assignment.Value)))
			return !credential
		}
		call, ok := node.(*syntax.CallExpr)
		if !ok {
			return true
		}
		args := make([]string, len(call.Args))
		for i, word := range call.Args {
			args[i] = unescapedWord(word)
		}
		if nestedShellCredential(args, depth) {
			credential = true
			return false
		}
		for i, arg := range args {
			if name, value, ok := strings.Cut(arg, "="); ok && flagNamePattern.MatchString(strings.TrimLeft(name, "_")) && CredentialKey(name) && literalHeaderValue(value) {
				credential = true
			}
			header, attached := strings.CutPrefix(arg, "--header=")
			if strings.HasPrefix(arg, "-H") && len(arg) > 2 {
				header, attached = arg[2:], true
			}
			if attached && credentialHeader(header) {
				credential = true
			}
			if (arg == "-H" || arg == "--header") && i+1 < len(args) {
				credential = credential || credentialHeader(args[i+1])
			}
			if flag, value, inline := CredentialFlag(arg); flag != "" {
				if !inline && i+1 < len(args) {
					value = args[i+1]
				}
				if !strings.HasPrefix(value, "-") && literalHeaderValue(value) {
					credential = true
				}
			}
		}
		return !credential
	})
	return credential
}

func unescapedWord(word *syntax.Word) string {
	if word == nil {
		return ""
	}
	var text bytes.Buffer
	var writeParts func([]syntax.WordPart)
	writeParts = func(parts []syntax.WordPart) {
		for _, part := range parts {
			switch p := part.(type) {
			case *syntax.Lit:
				text.WriteString(strings.ReplaceAll(p.Value, `\`, ""))
			case *syntax.SglQuoted:
				text.WriteString(p.Value)
			case *syntax.DblQuoted:
				writeParts(p.Parts)
			default:
				_ = syntax.NewPrinter().Print(&text, part)
			}
		}
	}
	writeParts(word.Parts)
	return text.String()
}

func nestedShellCredential(args []string, depth int) bool {
	for i, arg := range args {
		switch filepath.Base(arg) {
		case "sh", "bash", "dash", "ash", "zsh", "ksh":
		default:
			continue
		}
		for j := i + 1; j < len(args); j++ {
			flag := args[j]
			if !strings.HasPrefix(flag, "-") || flag == "--" {
				break
			}
			if !strings.HasPrefix(flag, "--") && strings.Contains(flag, "c") && j+1 < len(args) {
				if commandCredentialDetected(args[j+1], depth+1) {
					return true
				}
				break
			}
		}
	}
	return false
}

func credentialHeader(header string) bool {
	name, value, ok := strings.Cut(header, ":")
	return ok && CredentialKey(strings.TrimSpace(name)) && literalHeaderValue(value)
}

func literalHeaderValue(value string) bool {
	value = DetectorText(strings.TrimSpace(value))
	return !RefOnly(value) && LiteralCredential(schemeWord.ReplaceAllString(value, ""))
}
