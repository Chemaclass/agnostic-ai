package preview

import (
	"regexp"
	"strings"
)

// Credential words as label text writes them. A multi-word label may be
// spaced, hyphenated, underscored, or joined: `API key`, `api-key`,
// `apikey`.
const (
	multiWordLabel = `(?:api|private|access|secret)[ _-]?keys?|client[ _-]?secrets?`
	strongLabel    = `pass(?:word|wd|phrase)s?|secrets?|credentials?|` + multiWordLabel
	weakLabel      = `tokens?|auth(?:orization)?|bearer|cookies?|sessions?(?:[ _-]?ids?)?`
	labelEmphasis  = "[*_~`]*"
	credentialKey  = `(?:` + multiWordLabel + `|pass(?:word|wd|phrase)?|secret|token|credentials?|auth(?:orization)?|bearer|cookie|session)[A-Za-z0-9_.-]*["'*_` + "`" + `]*\s*[:=]`
)

var (
	// credentialAssignment is a credential-named key with a value after
	// it, in any layout: `password:`, `API key =`, `**Password:**`,
	// `<config token="...">`, `{password: x}`.
	credentialAssignment = regexp.MustCompile(`(?i)(?:^|[^A-Za-z])` + credentialKey)
	// anyCredentialKey is credentialAssignment inside a longer word too,
	// as in `oauth:`, for the whole-text check.
	anyCredentialKey = regexp.MustCompile(`(?i)` + credentialKey)
	// credentialWord is a strong credential word anywhere, even one that is
	// only named, for the whole-text check.
	credentialWord = regexp.MustCompile(`(?i)\b(?:` + strongLabel + `)\b`)
	// credentialStatement is a strong credential word stated as having a
	// value, as in "the password is x". A word that is only named, as in
	// "never commit secrets", does not count.
	credentialStatement = regexp.MustCompile(`(?i)\b(?:` + strongLabel + `)\s+(?:is|are|was|were)\s+\S`)
	// credentialLabel is a credential word that labels a table cell or an
	// XML tag.
	credentialLabel = regexp.MustCompile(`(?im)(?:<\s*/?\s*` + labelEmphasis + `(?:` + strongLabel + `|` + weakLabel + `)[\s/>]|(?:\|\s*|^\s*)` + labelEmphasis + `(?:` + strongLabel + `|` + weakLabel + `)` + labelEmphasis + `(?:\s*\||[^A-Za-z|\n]*\|))`)
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

// ProseSensitive is Sensitive for prose that is not a file, such as a
// chat title or prompt: the same signs of a credential, plus one in a
// URL, without reading the text as YAML, JSON or a shell script. A
// prompt that only looks structured, such as "[WIP] Fix: login crash",
// is then not withheld as content that could not be parsed.
func ProseSensitive(text string) bool {
	return labelsCredential(text) || credentialWord.MatchString(text) ||
		anyCredentialKey.MatchString(text) || URLCredentialIn(text)
}

// Sensitive reports whether text shows any sign of a credential: Display
// hides or withholds part of it, it labels a credential anywhere, a
// credential-named key has any value, or a strong credential word
// (password, secret, API key) appears at all. Layouts are endless, so a
// caller that withholds the whole text on this never shows a value
// sitting next to such a word; guidance that only names one is withheld
// too. Display stays narrower, so previews of guidance still show.
func Sensitive(path, text string) bool {
	shown := Display(path, text)
	return shown.Hidden || shown.Withheld || labelsCredential(text) ||
		credentialWord.MatchString(text) || anyCredentialKey.MatchString(text)
}
