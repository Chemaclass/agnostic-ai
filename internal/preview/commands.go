package preview

import (
	"bytes"
	"net/url"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

var commandFlag = regexp.MustCompile(`--([A-Za-z][A-Za-z0-9_-]*)`)

func commandSensitiveSyntax(body string) bool {
	if strings.Contains(body, "=") {
		return true
	}
	for _, match := range commandFlag.FindAllStringSubmatch(body, -1) {
		if CredentialName(match[1]) {
			return true
		}
	}
	return strings.Contains(body, "Authorization:") || strings.Contains(body, "authorization:") || strings.Contains(body, "-H") || strings.Contains(body, "--header=") || strings.Contains(body, " --header ") || sensitiveURL(body)
}

func sensitiveCommand(body string) bool {
	if !commandSensitiveSyntax(body) {
		return false
	}
	file, err := syntax.NewParser().Parse(strings.NewReader(body), "")
	if err != nil {
		if wholeURL(body) {
			return sensitiveURL(body)
		}
		return true
	}
	return sensitiveShellTree(file)
}

func shellWord(word *syntax.Word) string {
	if word == nil {
		return ""
	}
	var out bytes.Buffer
	var writeParts func([]syntax.WordPart)
	writeParts = func(parts []syntax.WordPart) {
		for _, part := range parts {
			switch p := part.(type) {
			case *syntax.Lit:
				out.WriteString(p.Value)
			case *syntax.SglQuoted:
				out.WriteString(p.Value)
			case *syntax.DblQuoted:
				writeParts(p.Parts)
			default:
				_ = syntax.NewPrinter().Print(&out, part)
			}
		}
	}
	writeParts(word.Parts)
	return out.String()
}

func pureArgument(value string) bool {
	if PureReference(value) {
		return true
	}
	_, rest, ok := strings.Cut(value, ":")
	return ok && PureReference(strings.TrimSpace(rest))
}

// powerShellAssignment is a `name = value` in any PowerShell spelling: a
// variable with an optional scope such as `$env:` or `${env:NAME}`, a
// hashtable key, quoted or not, and `+=`. value stops at its closing quote
// or at a space, `;`, or `}`.
var powerShellAssignment = regexp.MustCompile(`(?i)(?:\$\{?(?:[a-z]+:)?|["']?)([A-Za-z_][A-Za-z0-9_-]*)["'}]?\s*\]?\s*\+?=\s*("[^"]*"|'[^']*'|[^\s;}]+)`)

// powerShellQuoted is a quoted PowerShell string, read as a possible
// `Name: value` header.
var powerShellQuoted = regexp.MustCompile(`"([^"]*)"|'([^']*)'`)

// powerShellFlag is a `-Name:value` or `-Name value` parameter.
var powerShellFlag = regexp.MustCompile(`(?:^|\s)-([A-Za-z][A-Za-z0-9_-]*)(?::|\s+)("[^"]*"|'[^']*'|[^\s;}]+)`)

// sensitivePowerShell reads a Windows hook command, which is PowerShell:
// a shell parser fails on its `if (...) { }` blocks, so the shell fallback
// would hide every command with an `=` in it. It hides what the shell
// reading finds when the text parses as shell, a credential the text
// detectors find, a quoted credential header, or a literal given to a
// credential name as an assignment, hashtable key, or parameter.
func sensitivePowerShell(body string) bool {
	if file, err := syntax.NewParser().Parse(strings.NewReader(body), ""); err == nil && sensitiveShellTree(file) {
		return true
	}
	if CredentialDetected(body) {
		return true
	}
	for _, m := range powerShellQuoted.FindAllStringSubmatch(body, -1) {
		if powerShellHeader(m[1] + m[2]) {
			return true
		}
	}
	for _, pattern := range []*regexp.Regexp{powerShellAssignment, powerShellFlag} {
		for _, m := range pattern.FindAllStringSubmatch(body, -1) {
			if CredentialKey(m[1]) && SecretValue(m[2], false, WeakCredential) {
				return true
			}
		}
	}
	return false
}

// powerShellHeader reports a `Name: value` credential header whose value,
// after an optional scheme word such as Bearer, is one literal word with
// a digit: a `$` reference reads the secret at run time, and a value with
// spaces, or a plain word like "required", is prose.
func powerShellHeader(text string) bool {
	_, value, ok := strings.Cut(text, ":")
	if !ok {
		return false
	}
	fields := strings.Fields(value)
	if len(fields) == 2 {
		fields = fields[1:]
	}
	return len(fields) == 1 && !strings.HasPrefix(fields[0], "$") && strings.ContainsAny(fields[0], "0123456789") && CredentialHeader(text)
}

func sensitiveShell(body string) bool {
	file, err := syntax.NewParser().Parse(strings.NewReader(body), "")
	if err != nil {
		return commandSensitiveSyntax(body)
	}
	return sensitiveShellTree(file)
}

func sensitiveShellTree(file *syntax.File) bool {
	hidden := false
	syntax.Walk(file, func(node syntax.Node) bool {
		if hidden {
			return false
		}
		switch n := node.(type) {
		case *syntax.Assign:
			if n.Name != nil && CredentialKey(n.Name.Value) && (n.Array != nil || n.Value != nil && !PureReference(shellWord(n.Value))) {
				hidden = true
			}
		case *syntax.CallExpr:
			nextSensitive, header := false, false
			words := make([]string, len(n.Args))
			for i, word := range n.Args {
				words[i] = shellWord(word)
			}
			if passesPasswordArg(words) {
				hidden = true
				break
			}
			for _, word := range n.Args {
				value := shellWord(word)
				name, inline, hasValue := strings.Cut(strings.TrimPrefix(value, "--"), "=")
				flag := strings.HasPrefix(value, "--") && CredentialName(name)
				headerValue, headerFlag := attachedHeader(value)
				assignmentKey, assignmentValue, assignment := strings.Cut(value, "=")
				if assignment && CredentialKey(assignmentKey) && !PureReference(strings.Trim(assignmentValue, "\"'")) {
					hidden = true
					break
				}
				if nextSensitive && !PureReference(value) && (!header || !pureArgument(value)) || headerFlag && !pureArgument(headerValue) || flag && hasValue && !PureReference(inline) || sensitiveURL(value) {
					hidden = true
					break
				}
				if key, rest, ok := strings.Cut(value, ":"); ok && CredentialKey(key) && strings.TrimSpace(rest) != "" && !PureReference(strings.TrimSpace(rest)) {
					hidden = true
					break
				}
				header = value == "-H" || value == "--header"
				nextSensitive = flag && !hasValue || header
			}
		}
		return !hidden
	})
	return hidden
}

func attachedHeader(value string) (string, bool) {
	if header, ok := strings.CutPrefix(value, "--header="); ok {
		return header, true
	}
	if strings.HasPrefix(value, "-H") && len(value) > 2 {
		return value[2:], true
	}
	return "", false
}

func wholeURL(body string) bool {
	reference := referenceTokens.FindStringIndex(body)
	if reference == nil {
		return false
	}
	normalized := referenceTokens.ReplaceAllString(body, "previewref")
	if strings.ContainsAny(normalized, " \t\r\n") {
		return false
	}
	parsed, err := url.Parse(normalized)
	if err != nil {
		return false
	}
	anchored := parsed.Scheme != "" && parsed.Host != ""
	anchored = anchored || reference[0] == 0 && reference[1] < len(body) && strings.ContainsRune("/?#", rune(body[reference[1]]))
	if !anchored {
		return false
	}
	file, err := syntax.NewParser().Parse(strings.NewReader(normalized), "")
	if err != nil || len(file.Stmts) != 1 {
		return false
	}
	statement := file.Stmts[0]
	call, ok := statement.Cmd.(*syntax.CallExpr)
	if !ok || len(call.Args) != 1 || len(call.Assigns) != 0 || len(statement.Redirs) != 0 || statement.Background || statement.Negated {
		return false
	}
	for _, part := range call.Args[0].Parts {
		if _, literal := part.(*syntax.Lit); !literal {
			return false
		}
	}
	return true
}

func sensitiveHelper(body string) bool {
	file, err := syntax.NewParser().Parse(strings.NewReader(body), "")
	if err != nil || len(file.Stmts) != 1 {
		return true
	}
	stmt := file.Stmts[0]
	call, ok := stmt.Cmd.(*syntax.CallExpr)
	if !ok || len(call.Args) == 0 || len(call.Assigns) != 0 || len(stmt.Redirs) != 0 || stmt.Background || stmt.Negated {
		return true
	}

	if !PureReference(shellWord(call.Args[0])) {
		for _, part := range call.Args[0].Parts {
			switch value := part.(type) {
			case *syntax.Lit, *syntax.SglQuoted:
			case *syntax.DblQuoted:
				for _, inner := range value.Parts {
					if _, literal := inner.(*syntax.Lit); !literal {
						return true
					}
				}
			default:
				return true
			}
		}
	}
	for _, word := range call.Args[1:] {
		if !PureReference(shellWord(word)) {
			return true
		}
	}
	return sensitiveShellTree(file)
}

// mysqlClients take the password glued to -p, as in `mysql -psecret`.
var mysqlClients = regexp.MustCompile(`^(?:mysql|mysqldump|mysqladmin|mysqlimport|mysqlcheck|mysqlshow|mysqlslap|mariadb(?:-[a-z]+)?)$`)

func mysqlClient(command string) bool {
	return mysqlClients.MatchString(filepath.Base(command))
}

// mysqlPasswordArg reports whether arg is -p with a literal password
// glued to it. A bare -p prompts for the password instead.
func mysqlPasswordArg(arg string) bool {
	value, ok := strings.CutPrefix(arg, "-p")
	return ok && value != "" && !PureReference(value)
}

// passesPasswordArg reports whether a command's words, its name first,
// pass a literal password to -p: glued to it for a MySQL client, glued
// or as the next word for sshpass. Other tools read -p as a port.
func passesPasswordArg(args []string) bool {
	if len(args) == 0 {
		return false
	}
	if mysqlClient(args[0]) {
		return slices.ContainsFunc(args[1:], mysqlPasswordArg)
	}
	return sshpassPasswordAt(args) > 0
}

// sshpassPasswordAt is the index of the literal password in an sshpass
// command's words, -p and its value as one word or two, or 0 for none.
func sshpassPasswordAt(args []string) int {
	if len(args) == 0 || programName(args[0]) != "sshpass" {
		return 0
	}
	for i := 1; i < len(args) && strings.HasPrefix(args[i], "-"); i++ {
		switch {
		case args[i] == "-p":
			if i+1 < len(args) && !notLiteral(args[i+1]) {
				return i + 1
			}
			return 0
		case strings.HasPrefix(args[i], "-p"):
			if !notLiteral(args[i][2:]) {
				return i
			}
			return 0
		case args[i] == "-f" || args[i] == "-d" || args[i] == "-P":
			i++
		}
	}
	return 0
}
