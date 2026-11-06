package preview

import (
	"regexp"
	"strings"
)

var (
	// credentialAssignment is a credential-named key with a value after
	// it, in any layout: `password:`, `token =`, `**Password:**`,
	// `<config token="...">`, `{password: x}`.
	credentialAssignment = regexp.MustCompile(`(?i)(?:^|[^A-Za-z])(?:pass(?:word|wd|phrase)?|secret|token|api[_-]?key|access[ _-]?key|private[_-]?key|credentials?|auth(?:orization)?|bearer|cookie|session|client[ _-]?secret)[A-Za-z0-9_.-]*["'*_` + "`" + `]*\s*[:=]`)
	// credentialStatement is a strong credential word stated as having a
	// value, as in "the password is x". A word that is only named, as in
	// "never commit secrets", does not count.
	credentialStatement = regexp.MustCompile(`(?i)\b(?:pass(?:word|wd|phrase)|secret|credential|api[ _-]?key|private[ _-]?key|client[ _-]?secret)s?\s+(?:is|are|was|were)\s+\S`)
	// credentialLabel is a weaker credential word that labels a table
	// cell or an XML tag.
	credentialLabel = regexp.MustCompile(`(?im)(?:<\s*/?\s*[*_~` + "`" + `]*(?:tokens?|auth(?:orization)?|bearer|cookies?|sessions?(?:[ _-]?ids?)?|access[ _-]?keys?|pass(?:word|wd|phrase)s?|secrets?|credentials?)[\s/>]|(?:\|\s*|^\s*)[*_~` + "`" + `]*(?:tokens?|auth(?:orization)?|bearer|cookies?|sessions?(?:[ _-]?ids?)?|access[ _-]?keys?|pass(?:word|wd|phrase)s?|secrets?|credentials?)[*_~` + "`" + `]*(?:\s*\||[^A-Za-z|\n]*\|))`)
	privateKeyBlock = regexp.MustCompile(`-----BEGIN [A-Z0-9 ]*PRIVATE KEY`)
	// plainAssignedValue is a value no credential takes: null, a boolean, a
	// number, or a pure reference, ending the field.
	plainAssignedValue = regexp.MustCompile(`(?i)^\s*["']?(?:null|~|true|false|-?[0-9]+(?:\.[0-9]+)?|\$\{?[A-Za-z_][A-Za-z0-9_]*\}?)["']?\s*(?:[,}\])]|$)`)
)

// mysqlInText is a MySQL client named in prose with the rest of its line.
var mysqlInText = regexp.MustCompile(`(?:^|[\s` + "`" + `(])((?:mysql|mysqldump|mysqladmin|mysqlimport|mysqlcheck|mysqlshow|mysqlslap|mariadb(?:-[a-z]+)?)[^\n` + "`" + `]*)`)

// passesMySQLPassword reports whether text runs a MySQL client with a
// literal password glued to -p, outside a fenced shell block too.
func passesMySQLPassword(text string) bool {
	for _, m := range mysqlInText.FindAllStringSubmatch(text, -1) {
		for _, arg := range strings.Fields(m[1])[1:] {
			if mysqlPasswordArg(strings.Trim(arg, `"'.,;:)`)) {
				return true
			}
		}
	}
	return false
}

// assignsCredential reports whether a credential-named key is given a
// value other than a plain one, such as `"input_tokens":null`.
func assignsCredential(text string) bool {
	for _, at := range credentialAssignment.FindAllStringIndex(text, -1) {
		line, _, _ := strings.Cut(text[at[1]:], "\n")
		if !plainAssignedValue.MatchString(line) {
			return true
		}
	}
	return false
}

// labelsCredential reports whether prose text shows a sign of a
// credential: a credential-named key with a value, a strong credential
// word stated with a value, a weaker one labeling a table cell or a tag,
// a private key block, or a recognizable token or credential. A secret
// written as free prose with no label or shape is not detectable.
func labelsCredential(text string) bool {
	return assignsCredential(text) || credentialStatement.MatchString(text) ||
		credentialLabel.MatchString(text) || privateKeyBlock.MatchString(text) ||
		TokenIn(text) || CredentialDetected(text) || passesMySQLPassword(text)
}

// Sensitive reports whether Display hides or withholds any part of text,
// or text labels a credential anywhere, structured parts included. A
// caller that must not show a value the detectors missed elsewhere in the
// same text withholds the whole text then. Display applies the labels to
// prose only, so a structured file that names a credential key with a
// reference still previews.
func Sensitive(path, text string) bool {
	shown := Display(path, text)
	return shown.Hidden || shown.Withheld || labelsCredential(text)
}
