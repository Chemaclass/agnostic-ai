package preview

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

const Redacted = redacted

var quotedText = regexp.MustCompile(`"(?:\\.|[^"\\])*"`)
var textURL = regexp.MustCompile(`https?://[^\s<>"']+`)
var plainPathSegment = regexp.MustCompile(`^(?:[A-Za-z][A-Za-z._-]{0,31}|v[0-9]{1,3})$`)

// RedactText sanitizes display text and any explicitly resolved credential values.
func RedactText(text string, values ...string) string {
	for _, value := range values {
		if value == "" || PureReference(value) {
			continue
		}
		for _, form := range []string{value, strings.Trim(strconv.Quote(value), `"`), url.QueryEscape(value), url.PathEscape(value)} {
			text = strings.ReplaceAll(text, form, Redacted)
		}
	}
	text = quotedText.ReplaceAllStringFunc(text, func(quoted string) string {
		value, err := strconv.Unquote(quoted)
		if err != nil {
			return quoted
		}
		shown := RedactText(value, values...)
		if shown != value {
			return strconv.Quote(shown)
		}
		return quoted
	})
	text = textURL.ReplaceAllStringFunc(text, RedactURL)
	detector := strings.ReplaceAll(text, Redacted, "${REDACTED}")
	// Text reads as shell only as a `shell(...)` or `Bash(...)` rule or
	// when it looks like a command: prose that fails to parse as shell
	// would be hidden whole.
	rule := strings.HasPrefix(detector, "shell(") || strings.HasPrefix(detector, "Bash(")
	if CredentialDetected(detector) || rule && CredentialInRule(detector) {
		return Redacted
	}
	if wholeURL(detector) {
		return text
	}
	if commandSensitiveSyntax(detector) && !strings.ContainsAny(detector, "\r\n") && !rule && CredentialInRule("shell("+detector+")") {
		return Redacted
	}
	return text
}

// RedactURL preserves public URL structure and environment references.
func RedactURL(raw string) string {
	rest, fragment, hasFragment := strings.Cut(raw, "#")
	rest, query, hasQuery := strings.Cut(rest, "?")
	if scheme, after, ok := strings.Cut(rest, "://"); ok {
		authority, path := after, ""
		if i := strings.Index(after, "/"); i >= 0 {
			authority, path = after[:i], after[i:]
		}
		if at := strings.LastIndex(authority, "@"); at >= 0 && !PureReference(strings.ReplaceAll(authority[:at], ":", "")) {
			authority = Redacted + authority[at:]
		}
		segments := strings.Split(path, "/")
		for i, segment := range segments {
			if segment != "" && !PureReference(segment) && !plainPathSegment.MatchString(segment) {
				segments[i] = Redacted
			}
		}
		rest = scheme + "://" + authority + strings.Join(segments, "/")
	}
	pairs := func(text string) string {
		parts := strings.Split(text, "&")
		for i, part := range parts {
			key, value, hasValue := strings.Cut(part, "=")
			if hasValue {
				if value != "" && !PureReference(value) {
					parts[i] = key + "=" + Redacted
				}
			} else if part != "" && !PureReference(part) {
				parts[i] = Redacted
			}
		}
		return strings.Join(parts, "&")
	}
	if hasQuery {
		rest += "?" + pairs(query)
	}
	if hasFragment {
		rest += "#" + pairs(fragment)
	}
	return rest
}
