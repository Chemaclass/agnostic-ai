package preview

import (
	"bytes"
	"net/url"
	"regexp"
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
