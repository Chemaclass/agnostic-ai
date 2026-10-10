package preview

import (
	"net/url"
	"regexp"
	"strings"
	"unicode"
)

// strongCredentialWords are the last words of a name that make
// import treat its value as a credential, so `api_token` counts and
// `token_id` does not.
var strongCredentialWords = map[string]bool{
	"token": true, "secret": true, "password": true, "passwd": true, "pwd": true, "pass": true,
	"accesstoken": true, "authtoken": true, "credential": true, "credentials": true, "cookie": true, "bearer": true,
}

// weakCredentialWords count too, but a short value after one in a
// separate argument may be a mode or a name, as in `--require-api-key
// run`. `cred` and `auth` count only in a key, such as `X_AUTH`, since a
// flag such as `--auth oauth` picks a method.
var weakCredentialWords = map[string]bool{
	"apikey": true, "authentication": true, "authorization": true,
}

// mcpPublicKeyWords are the words before a final `key` that make it no
// credential, so `sort_key` and `public_key` do not count and
// `openai_key` does.
var mcpPublicKeyWords = map[string]bool{
	"sort": true, "cache": true, "public": true, "partition": true, "primary": true, "foreign": true,
	"idempotency": true, "routing": true, "object": true, "row": true, "hash": true, "map": true,
	"lookup": true, "group": true, "dedup": true, "shard": true, "index": true, "unique": true,
	"composite": true, "natural": true, "surrogate": true, "ssh": true, "gpg": true, "pgp": true,
	"s3": true, "id": true,
}

// mcpCredentialQueryNames count only as a whole query or fragment
// parameter name, where `code` or `sig` is a signed or one-time value,
// so `country_code` does not.
var mcpCredentialQueryNames = map[string]bool{
	"auth": true, "sig": true, "signature": true, "code": true, "session": true, "jwt": true, "bearer": true,
}

var mcpGluedCredentialWord = regexp.MustCompile(`(password|passwd|secret|token)$`)

type CredentialStrength int

const (
	NoCredential CredentialStrength = iota
	WeakCredential
	StrongCredential
)

// NameStrength reports whether name holds a credential by its last
// word, and how surely. A one-word name that ends in a strong word
// counts too, as `PGPASSWORD` does. key marks an env, header, or block
// key, where `cred` and `auth` count as well.
func NameStrength(name string, key bool) CredentialStrength {
	words := nameWords(name)
	n := len(words)
	switch {
	case n == 0:
		return NoCredential
	case strongCredentialWords[words[n-1]]:
		return StrongCredential
	case weakCredentialWords[words[n-1]], key && (words[n-1] == "cred" || words[n-1] == "auth"):
		return WeakCredential
	case words[n-1] == "key":
		if n == 1 || !mcpPublicKeyWords[words[n-2]] {
			return WeakCredential
		}
		return NoCredential
	case n == 1 && mcpGluedCredentialWord.MatchString(words[0]):
		return StrongCredential
	}
	return NoCredential
}

// CredentialName reports whether a flag or parameter name holds a
// credential.
func CredentialName(name string) bool { return NameStrength(name, false) != NoCredential }

// CredentialKey reports whether an env, header, or block key holds a
// credential.
func CredentialKey(name string) bool { return NameStrength(name, true) != NoCredential }

func CredentialParam(name string) bool {
	if unescaped, err := url.QueryUnescape(name); err == nil {
		name = unescaped
	}
	return CredentialName(name) || mcpCredentialQueryNames[strings.ToLower(name)]
}

// nameWords splits a name into lower-case words at punctuation and at
// camelCase boundaries, so `clientSecret` and `APIKey` both end in a
// credential word.
func nameWords(name string) []string {
	runes := []rune(name)
	var words []string
	var word []rune
	flush := func() {
		if len(word) > 0 {
			words = append(words, strings.ToLower(string(word)))
			word = nil
		}
	}
	for i, r := range runes {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			flush()
			continue
		}
		if unicode.IsUpper(r) && len(word) > 0 {
			nextLower := i+1 < len(runes) && unicode.IsLower(runes[i+1])
			if !unicode.IsUpper(runes[i-1]) || nextLower {
				flush()
			}
		}
		word = append(word, r)
	}
	flush()
	return words
}
