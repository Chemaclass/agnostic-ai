package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// importJSONMCPMap reads a JSON file at srcPath, extracts the
// flat-or-dotted key, and writes one yaml per server into dstDir.
// Common helper for amp / opencode / vscode-style MCP shapes where
// servers are a map keyed by name.
func importJSONMCPMap(target, srcPath, mapKey, dstDir string) (int, error) {
	servers, err := readJSONMapAt(srcPath, mapKey)
	if err != nil || len(servers) == 0 {
		return 0, err
	}
	return writeMCPYAMLs(target, servers, dstDir)
}

// readJSONMapAt loads srcPath as JSON and returns the map at mapKey.
// Supports a dotted key like "amp.mcpServers" so callers can target a
// nested key without writing custom decoders. Missing file or missing
// key returns (nil, nil); only IO and parse failures bubble up.
func readJSONMapAt(srcPath, mapKey string) (map[string]any, error) {
	data, err := os.ReadFile(srcPath)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", srcPath, err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", srcPath, err)
	}
	v, ok := doc[mapKey]
	if !ok {
		return nil, nil
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, nil
	}
	out := map[string]any{}
	for k, val := range m {
		sub, ok := val.(map[string]any)
		if !ok {
			continue
		}
		out[k] = sub
	}
	return out, nil
}

// writeMCPYAMLs writes one project MCP spec per server, read from
// target's native file. target's own environment references become the
// spec's `${NAME}`, and every literal `env` or `headers` value becomes a
// reference too: import cannot tell a token from a setting, and a spec
// is a file meant to be committed (#1619). So does each credential in a
// `url` or `args` URL.
func writeMCPYAMLs(target string, servers map[string]any, dstDir string) (int, error) {
	for _, raw := range servers {
		if server, ok := raw.(map[string]any); ok {
			// codexMCPDocs escapes Codex's literals itself, before it
			// spells forwarded variables as `${NAME}`.
			if target != "codex" {
				adapters.EscapeMCPLiterals(target, server)
			}
			adapters.ReadMCPEnvRefs(target, server)
		}
	}
	refs := referenceMCPLiterals(servers)
	count, err := writeMCPSpecs(servers, dstDir)
	if err != nil {
		return count, err
	}
	reportMCPLiteralRefs(refs)
	return count, nil
}

// writeMCPSpecs writes one yaml file per server into dstDir. Each
// destination doc has `name: <key>` prepended; server fields pass
// through verbatim so transport-specific keys (command/args/env or
// url/headers) survive a round-trip. When the source JSON omits an
// explicit `type` field, the transport is inferred from the entry's
// shape (`url` present → `type: http`) so re-emit picks the same
// branch in adapter buildMCPEntry helpers and the round-trip
// converges.
func writeMCPSpecs(servers map[string]any, dstDir string) (int, error) {
	names := make([]string, 0, len(servers))
	for k := range servers {
		names = append(names, k)
	}
	sort.Strings(names)
	if err := spec.ValidateMCPNames(names); err != nil {
		return 0, err
	}
	count := 0
	for _, name := range names {
		entry, _ := servers[name].(map[string]any)
		doc := map[string]any{"name": name}
		for k, v := range entry {
			doc[k] = v
		}
		if _, hasType := doc["type"]; !hasType {
			if _, hasURL := doc["url"]; hasURL {
				doc["type"] = "http"
			}
		}
		raw, err := yaml.Marshal(doc)
		if err != nil {
			return count, fmt.Errorf("marshal mcp %s: %w", name, err)
		}
		path := filepath.Join(dstDir, spec.MCPFileName(name))
		if err := importWriteFile(path, raw, 0o644); err != nil {
			return count, fmt.Errorf("write %s: %w", path, err)
		}
		count++
	}
	return count, nil
}

// mcpLiteralRef is one value import rewrote so the spec holds no
// secret: a literal replaced with a reference, or a reference whose
// default was removed.
type mcpLiteralRef struct {
	server, field, key string
	// value is the reference the spec now holds.
	value    string
	variable string
	// defaulted marks a `${NAME:-default}` that lost its default.
	defaulted bool
	// ran is the command a `$(...)` value ran, never its arguments.
	ran string
	// prompted is the id of a VS Code `${input:id}` prompt the value used.
	prompted string
	// leftOut marks a server import did not write, because field holds
	// URLs it cannot rewrite.
	leftOut bool
}

type mcpLiteral struct {
	server, field, key string
	prefix, secret     string
	// replace writes text in place of the secret.
	replace func(text string)
	// value is what the field holds now.
	value func() string
}

// mcpURLValue is a url or argument split around its credentials, so
// each one can become a reference while the rest stays as written.
type mcpURLValue struct {
	pieces []string
	write  func(string)
}

func (v *mcpURLValue) set(i int, text string) {
	v.pieces[i] = text
	v.write(v.String())
}

func (v *mcpURLValue) String() string { return strings.Join(v.pieces, "") }

// mcpStrongCredentialWords are the last words of a name that make
// import treat its value as a credential, so `api_token` counts and
// `token_id` does not.
var mcpStrongCredentialWords = map[string]bool{
	"token": true, "secret": true, "password": true, "passwd": true, "pwd": true, "pass": true,
	"accesstoken": true, "authtoken": true, "credential": true, "credentials": true, "cookie": true, "bearer": true,
}

// mcpWeakCredentialWords count too, but a short value after one in a
// separate argument may be a mode or a name, as in `--require-api-key
// run`. `cred` and `auth` count only in a key, such as `X_AUTH`, since a
// flag such as `--auth oauth` picks a method.
var mcpWeakCredentialWords = map[string]bool{
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

type mcpCredentialStrength int

const (
	mcpNoCredential mcpCredentialStrength = iota
	mcpWeakCredential
	mcpStrongCredential
)

// mcpNameStrength reports whether name holds a credential by its last
// word, and how surely. A one-word name that ends in a strong word
// counts too, as `PGPASSWORD` does. key marks an env, header, or block
// key, where `cred` and `auth` count as well.
func mcpNameStrength(name string, key bool) mcpCredentialStrength {
	words := mcpNameWords(name)
	n := len(words)
	switch {
	case n == 0:
		return mcpNoCredential
	case mcpStrongCredentialWords[words[n-1]]:
		return mcpStrongCredential
	case mcpWeakCredentialWords[words[n-1]], key && (words[n-1] == "cred" || words[n-1] == "auth"):
		return mcpWeakCredential
	case words[n-1] == "key":
		if n == 1 || !mcpPublicKeyWords[words[n-2]] {
			return mcpWeakCredential
		}
		return mcpNoCredential
	case n == 1 && mcpGluedCredentialWord.MatchString(words[0]):
		return mcpStrongCredential
	}
	return mcpNoCredential
}

// mcpCredentialName reports whether a flag or parameter name holds a
// credential.
func mcpCredentialName(name string) bool { return mcpNameStrength(name, false) != mcpNoCredential }

// mcpCredentialKey reports whether an env, header, or block key holds a
// credential.
func mcpCredentialKey(name string) bool { return mcpNameStrength(name, true) != mcpNoCredential }

func mcpCredentialParam(name string) bool {
	if unescaped, err := url.QueryUnescape(name); err == nil {
		name = unescaped
	}
	return mcpCredentialName(name) || mcpCredentialQueryNames[strings.ToLower(name)]
}

// mcpNameWords splits a name into lower-case words at punctuation and at
// camelCase boundaries, so `clientSecret` and `APIKey` both end in a
// credential word.
func mcpNameWords(name string) []string {
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

func mcpCredentialValue(value string) bool {
	return value != "" && !spec.OnlyEscapedEnvRefs(value)
}

type mcpCredentialSpan struct {
	name       string
	start, end int
}

var (
	mcpSchemePattern     = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.\-]*://`)
	mcpQueryParamPattern = regexp.MustCompile(`[?&;#]([^=&;?#\s'"]+)=([^&;?#\s]*)`)
	mcpRefDefaultPattern = regexp.MustCompile(`\$?\$\{[A-Za-z_][A-Za-z0-9_]*:-([^{}]*)\}`)
	mcpPlainRefPattern   = regexp.MustCompile(`\$?\$\{[A-Za-z_][A-Za-z0-9_]*\}`)
)

// mcpSingleURL reports whether value is one URL import can rewrite: no
// whitespace, quotes, or escaped `$${`, and a scheme or a reference at
// the start. Any other value, such as a `sh -c` command, is never
// rewritten.
func mcpSingleURL(value string) bool {
	if strings.ContainsAny(value, " \t\r\n'\"`") || strings.Contains(value, "$${") {
		return false
	}
	return mcpSchemePattern.MatchString(value) || strings.HasPrefix(value, "${")
}

// mcpRefSentinel stands in for a plain `${NAME}` or `$${NAME}` while the
// detector reads a value.
const mcpRefSentinel = "\x00"

func mcpLiteralCredential(value string) bool {
	return strings.Trim(strings.ReplaceAll(value, mcpRefSentinel, ""), `'"`) != ""
}

// mcpDetectorText writes each `${NAME:-default}` as its default, so the
// default is read in URL context, and each plain reference as
// mcpRefSentinel.
func mcpDetectorText(value string) string {
	for {
		expanded := mcpRefDefaultPattern.ReplaceAllString(value, "$1")
		if expanded == value {
			break
		}
		value = expanded
	}
	return mcpPlainRefPattern.ReplaceAllString(value, mcpRefSentinel)
}

// mcpCredentialDetected reports whether value holds a literal credential
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
func mcpCredentialDetected(value string) bool {
	for _, text := range []string{mcpDetectorText(value), mcpDetectorText(mcpShellUnquote.Replace(value))} {
		for _, t := range []string{text, mcpEncodedSeparators.Replace(text)} {
			if mcpURLCredentialIn(t) || mcpTokenIn(t) || mcpUserPasswordIn(t) || mcpAssignmentIn(t) || mcpBearerIn(t) || mcpFlagIn(t) || mcpKeyValueIn(t) {
				return true
			}
		}
	}
	return false
}

var (
	mcpShellUnquote      = strings.NewReplacer(`'`, "", `"`, "", `\`, "")
	mcpEncodedSeparators = strings.NewReplacer("%26", "&", "%3D", "=", "%3d", "=", "%3F", "?", "%3f", "?", "%23", "#")
)

// mcpTokenPrefixes are token formats that name their issuer: GitHub,
// GitLab, Slack, npm, Google API keys, Stripe, OpenAI and Anthropic
// `sk-`, AWS access key ids, and JWTs.
const mcpTokenPrefixes = `gh[pousr]_[A-Za-z0-9_]{8,}|github_pat_[A-Za-z0-9_]{8,}|glpat-[A-Za-z0-9_\-]{8,}|xox[bpa]-[A-Za-z0-9\-]{8,}|xapp-[A-Za-z0-9\-]{8,}|npm_[A-Za-z0-9]{30,}|AIza[A-Za-z0-9_\-]{35}|(?:sk|rk)_live_[A-Za-z0-9]{8,}|sk_test_[A-Za-z0-9]{8,}|sk-(?:proj|ant)-[A-Za-z0-9_\-]{8,}|sk-[A-Za-z0-9]{20,}|AKIA[0-9A-Z]{16}|eyJ[A-Za-z0-9_\-]+\.eyJ[A-Za-z0-9_\-]+\.[A-Za-z0-9_\-]+`

var (
	mcpTokenPattern      = regexp.MustCompile(`^(?:` + mcpTokenPrefixes + `)$`)
	mcpEmbeddedToken     = regexp.MustCompile(`(?:^|[^A-Za-z0-9_])(` + mcpTokenPrefixes + `)(?:[^A-Za-z0-9]|$)`)
	mcpUserPasswordAt    = regexp.MustCompile(`(?:^|[\s'"=,])([A-Za-z0-9._%+\-\x00]+):([^\s@/:'"]+)@(\S*)`)
	mcpAssignment        = regexp.MustCompile(`(?:^|[^A-Za-z0-9_\x00])([A-Za-z_][A-Za-z0-9_\-]*)=([^\s;&|#]+)`)
	mcpShellVariable     = regexp.MustCompile(`^\$[A-Za-z_][A-Za-z0-9_]*$`)
	mcpBearer            = regexp.MustCompile(`(?i)(?:^|[^A-Za-z0-9])bearer\s+([^\s'",}\\]+)`)
	mcpPlainValue        = regexp.MustCompile(`^(?:[/~@]|\.\.?/|[A-Za-z]:[\\/]|(?i:true|false|yes|no|on|off)$|[0-9]+$|[a-z]+(?:-[a-z]+)+$)|(?i)\.(?:json|txt|pem|key|env|ya?ml|toml|crt|p12|db)$`)
	mcpHeaderLine        = regexp.MustCompile(`(?m)^\s*([A-Za-z0-9_\-]+)\s*:[ \t]*(\S.*?)\s*$`)
	mcpJSONPair          = regexp.MustCompile(`"([^"]+)"\s*:\s*"([^"]*)"`)
	mcpSchemeWord        = regexp.MustCompile(`^[A-Za-z]+\s+`)
	mcpImageDigestSuffix = regexp.MustCompile(`^sha[0-9]+:`)
)

// mcpToken reports whether value is a whole token by its prefix. An
// `sk-` token other than `sk-proj-` or `sk-ant-` needs a digit, so a
// package name such as `sk-learn-mcp-server-tools` is not one.
func mcpToken(value string) bool {
	if !mcpTokenPattern.MatchString(value) {
		return false
	}
	if strings.HasPrefix(value, "sk-proj-") || strings.HasPrefix(value, "sk-ant-") || !strings.HasPrefix(value, "sk-") {
		return true
	}
	return strings.ContainsAny(value, "0123456789")
}

// mcpSecretValue reports whether text, a value in detector text, is a
// literal that may be a secret: not a reference (after an optional
// scheme word, as in `Bearer ${TOKEN}`), a `$NAME` expansion, a file
// path or name, an `@` value, a boolean, a number, or lower-case words
// joined by dashes such as `streamable-http`. A value in its own
// argument after a flag must not start with `-`, and after a weak name
// it needs at least 8 characters, so `--require-api-key run` passes.
func mcpSecretValue(text string, separate bool, strength mcpCredentialStrength) bool {
	v := strings.Trim(text, `'"`)
	if !mcpLiteralCredential(mcpSchemeWord.ReplaceAllString(v, "")) || strings.HasPrefix(v, "$") || mcpPlainValue.MatchString(v) {
		return false
	}
	return !separate || !strings.HasPrefix(v, "-") && (strength == mcpStrongCredential || len(v) >= 8)
}

// mcpRefOnly reports whether value is only references, after an
// optional scheme word such as `Bearer ${TOKEN}`.
func mcpRefOnly(value string) bool {
	value = mcpSchemeWord.ReplaceAllString(strings.TrimSpace(value), "")
	return spec.OnlyEnvRefs(value) || mcpShellVariable.MatchString(value)
}

// mcpBearerIn finds a literal `Bearer` token, as in `-H
// 'Authorization: Bearer ...'` or a JSON headers value. The token needs
// a digit, one of `-._~+/=`, or at least 20 characters, so prose such as
// "uses bearer authentication" does not count.
func mcpBearerIn(text string) bool {
	for _, m := range mcpBearer.FindAllStringSubmatch(text, -1) {
		token := m[1]
		if strings.Contains(token, mcpRefSentinel) || strings.HasPrefix(token, "$") {
			continue
		}
		if len(token) >= 20 || strings.ContainsAny(token, "0123456789-._~+/=") {
			return true
		}
	}
	return false
}

// mcpKeyValueIn finds a line `Name: value` or a JSON pair `"name":
// "value"` whose name is a credential key and whose value is a literal,
// as in an `X-Api-Key: ...` headers value or `{"apiKey": "..."}`.
func mcpKeyValueIn(text string) bool {
	for _, pattern := range []*regexp.Regexp{mcpHeaderLine, mcpJSONPair} {
		for _, m := range pattern.FindAllStringSubmatch(text, -1) {
			if mcpCredentialKey(m[1]) && mcpSecretValue(m[2], false, mcpWeakCredential) {
				return true
			}
		}
	}
	return false
}

// mcpFlagIn finds a `--name value` or `--name=value` among the shell
// words of text whose name is a credential and whose value is a secret.
func mcpFlagIn(text string) bool {
	words := strings.Fields(text)
	for i, word := range words {
		name, value, hasValue := mcpCredentialFlag(word)
		switch {
		case name == "":
		case hasValue && mcpSecretValue(value, false, mcpWeakCredential):
			return true
		case !hasValue && i+1 < len(words) && mcpSecretValue(words[i+1], true, mcpNameStrength(name, false)):
			return true
		}
	}
	return false
}

// mcpCredentialHeader reports whether value, a `--header` or `-H`
// argument, is `Name: value` with a credential name and a literal value.
func mcpCredentialHeader(value string) bool {
	name, v, ok := strings.Cut(value, ":")
	v = strings.TrimSpace(v)
	return ok && mcpCredentialKey(strings.TrimSpace(name)) && !mcpRefOnly(v) && mcpSecretValue(mcpDetectorText(v), false, mcpWeakCredential)
}

func mcpTokenIn(text string) bool {
	for _, m := range mcpEmbeddedToken.FindAllStringSubmatch(text, -1) {
		if mcpToken(m[1]) {
			return true
		}
	}
	return false
}

// mcpUserPasswordIn finds a scheme-less `user:password@host`, as in
// `root:pw@db:3306` or an scp-style `user:pw@host:path`. An image digest
// such as `node:20@sha256:...` is not one, and neither is a Windows path
// such as `C:\Users\me@corp\a.db`.
func mcpUserPasswordIn(text string) bool {
	for _, m := range mcpUserPasswordAt.FindAllStringSubmatch(text, -1) {
		drive := len(m[1]) == 1 && unicode.IsLetter(rune(m[1][0]))
		if !drive && mcpLiteralCredential(m[2]) && !mcpImageDigestSuffix.MatchString(m[3]) {
			return true
		}
	}
	return false
}

// mcpAssignmentIn finds a `NAME=value` whose name ends in a credential
// word, as in `TOKEN=x` inside a `sh -c` command. A shell expansion such
// as `$TOKEN` or `$(cat f)` is not a literal.
func mcpAssignmentIn(text string) bool {
	for _, m := range mcpAssignment.FindAllStringSubmatch(text, -1) {
		if mcpCredentialKey(m[1]) && mcpSecretValue(m[2], false, mcpWeakCredential) {
			return true
		}
	}
	return false
}

func mcpURLCredentialIn(text string) bool {
	for _, m := range mcpQueryParamPattern.FindAllStringSubmatch(text, -1) {
		if mcpCredentialParam(m[1]) && mcpLiteralCredential(m[2]) {
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
			if !hidden && (!ok || port != "" && strings.Trim(port, "0123456789"+mcpRefSentinel) == "") {
				continue
			}
			if at = strings.Index(rest, "@"); at < 0 {
				continue
			}
		}
		if colon := strings.Index(rest[:at], ":"); colon >= 0 && mcpLiteralCredential(rest[colon+1:at]) {
			return true
		}
	}
}

// splitMCPURLCredentials finds the `user:password@` password and each
// credential query parameter in value, one URL. It returns the value
// split into pieces and, for each credential, its piece index and name.
// ok is false when the user part has more than one `@`.
func splitMCPURLCredentials(value string) (pieces []string, creds map[int]string, ok bool) {
	spans, ok := mcpURLCredentialSpans(value, maskMCPRefs(value))
	if !ok || len(spans) == 0 {
		return nil, nil, ok
	}
	creds = map[int]string{}
	written := 0
	for _, s := range spans {
		pieces = append(pieces, value[written:s.start], value[s.start:s.end])
		creds[len(pieces)-1] = s.name
		written = s.end
	}
	return append(pieces, value[written:]), creds, true
}

// mcpURLCredentialSpans finds the credentials in the URL that word is.
// A URL whose base is a reference (`${API_BASE}/mcp?token=x`) has no
// parseable password, but its query still counts. It reads the text by
// hand because a URL may hold `${NAME}` references that net/url rejects.
func mcpURLCredentialSpans(word, mask string) ([]mcpCredentialSpan, bool) {
	var spans []mcpCredentialSpan
	pos := 0
	if scheme := strings.Index(mask, "://"); scheme >= 0 {
		pos = scheme + 3
		end := pos + indexAnyOrLen(mask[pos:], "/?#")
		authority := mask[pos:end]
		if strings.Count(authority, "@") > 1 {
			return nil, false
		}
		if at := strings.LastIndex(authority, "@"); at >= 0 {
			user := authority[:at]
			colon := strings.Index(user, ":")
			if colon >= 0 {
				user = user[:colon]
			}
			if mcpToken(word[pos : pos+len(user)]) {
				spans = append(spans, mcpCredentialSpan{"token", pos, pos + len(user)})
			}
			if colon >= 0 {
				if start, stop := pos+colon+1, pos+at; mcpCredentialValue(word[start:stop]) {
					spans = append(spans, mcpCredentialSpan{"password", start, stop})
				}
			}
		}
		pos = end
	} else if q := strings.Index(mask, "?"); q < 0 || !strings.Contains(word[:q], "${") {
		return nil, true
	}
	end := pos + indexAnyOrLen(mask[pos:], "#")
	if q := strings.Index(mask[pos:end], "?"); q >= 0 {
		spans = append(spans, mcpParamSpans(word, mask, pos+q+1, end)...)
	}
	// A fragment such as `#access_token=...` carries parameters too.
	if end < len(mask) {
		start := end + 1
		if q := strings.Index(mask[start:], "?"); q >= 0 {
			start += q + 1
		}
		spans = append(spans, mcpParamSpans(word, mask, start, len(mask))...)
	}
	return spans, true
}

func mcpParamSpans(word, mask string, start, end int) []mcpCredentialSpan {
	var spans []mcpCredentialSpan
	for start <= end {
		stop := start + indexAnyOrLen(mask[start:end], "&;")
		if eq := strings.Index(mask[start:stop], "="); eq >= 0 {
			if name := word[start : start+eq]; mcpCredentialParam(name) && mcpCredentialValue(word[start+eq+1:stop]) {
				spans = append(spans, mcpCredentialSpan{name, start + eq + 1, stop})
			}
		}
		start = stop + 1
	}
	return spans
}

// maskMCPRefs returns value with every `${...}` token, nested ones
// included, written over with `_`, so a search for a URL delimiter never
// lands inside a reference such as `${DB_USER:-admin}`. Indexes in the
// result match value.
func maskMCPRefs(value string) string {
	masked := []byte(value)
	for i := 0; i+1 < len(value); i++ {
		if value[i] != '$' || value[i+1] != '{' {
			continue
		}
		end, depth := len(value)-1, 0
		for j := i + 1; j < len(value); j++ {
			if value[j] == '{' {
				depth++
			} else if value[j] == '}' {
				if depth--; depth == 0 {
					end = j
					break
				}
			}
		}
		for k := i; k <= end; k++ {
			masked[k] = '_'
		}
		i = end
	}
	return string(masked)
}

func indexAnyOrLen(s, chars string) int {
	if i := strings.IndexAny(s, chars); i >= 0 {
		return i
	}
	return len(s)
}

var (
	mcpCommandPattern = regexp.MustCompile(`\$\(\s*([^\s)]+)`)
	mcpInputPattern   = regexp.MustCompile(`\$\{input:([^}]+)\}`)
)

// referenceMCPLiterals replaces every literal `env` and `headers` value
// with a reference, and strips the default from a `${NAME:-default}`,
// since a default is a value too. A value with any text around its
// references counts as a literal and is replaced whole, since that text
// may be the secret (`postgres://u:pw@${HOST}/db`); so does one with a
// `${...}` sync cannot write, such as `${input:id}`, which would
// otherwise reach the spec and drop from every target. A `Bearer ` prefix
// stays outside the reference. A URL credential in `url` or `args`
// becomes a reference too (see mcpURLLiterals). See mcpLiteralNames for
// the variable names.
func referenceMCPLiterals(servers map[string]any) []mcpLiteralRef {
	var refs []mcpLiteralRef
	var literals []mcpLiteral
	referenced := map[string]bool{}
	for _, name := range slices.Sorted(maps.Keys(servers)) {
		server, _ := servers[name].(map[string]any)
		for _, field := range []string{"env", "headers"} {
			values, _ := server[field].(map[string]any)
			for _, key := range slices.Sorted(maps.Keys(values)) {
				value, _ := values[key].(string)
				if value == "" {
					continue
				}
				// An escaped placeholder is text a server expands itself,
				// not a secret.
				if spec.OnlyEscapedEnvRefs(value) {
					continue
				}
				if spec.OnlyEnvRefs(value) {
					stripped, defaulted := spec.StripEnvRefDefaults(value)
					for _, t := range spec.EnvRefTokens(stripped) {
						if t.Known() {
							referenced[t.Name] = true
						}
					}
					values[key] = stripped
					for _, variable := range defaulted {
						refs = append(refs, mcpLiteralRef{server: name, field: field, key: key, value: stripped, variable: variable, defaulted: true})
					}
					continue
				}
				l := mcpLiteral{server: name, field: field, key: key, secret: value,
					replace: func(text string) { values[key] = text },
					value:   func() string { return values[key].(string) }}
				if token, ok := strings.CutPrefix(value, "Bearer "); ok && field == "headers" && token != "" {
					l.prefix, l.secret = "Bearer ", token
				}
				literals = append(literals, l)
			}
		}
		urlLiterals, urlRefs, unreadable := mcpURLLiterals(name, server, referenced)
		if unreadable != "" {
			delete(servers, name)
			literals = slices.DeleteFunc(literals, func(l mcpLiteral) bool { return l.server == name })
			refs = slices.DeleteFunc(refs, func(r mcpLiteralRef) bool { return r.server == name })
			refs = append(refs, mcpLiteralRef{server: name, field: unreadable, leftOut: true})
			continue
		}
		literals = append(literals, urlLiterals...)
		refs = append(refs, urlRefs...)
	}
	names := mcpLiteralNames(literals, referenced)
	for i, variable := range names {
		literals[i].replace(literals[i].prefix + spec.EnvRef(variable))
	}
	for i, variable := range names {
		l := literals[i]
		ref := mcpLiteralRef{server: l.server, field: l.field, key: l.key, value: l.value(), variable: variable}
		// A URL credential reports only its reference: a command or prompt
		// id inside it is part of the secret.
		if l.field != "env" && l.field != "headers" {
			refs = append(refs, ref)
			continue
		}
		if m := mcpCommandPattern.FindStringSubmatch(l.secret); m != nil {
			ref.ran = m[1]
		}
		if m := mcpInputPattern.FindStringSubmatch(l.secret); m != nil {
			ref.prompted = m[1]
		}
		refs = append(refs, ref)
	}
	return refs
}

// mcpURLLiterals returns the credentials in server's `url` and `args`:
// a URL password, a URL username that is a token, each query or fragment
// parameter mcpCredentialParam names, the value of a `--name` or
// `--name=value` flag whose name ends in a credential word, and an
// argument that is a whole token. The rest of the value stays as
// written. A credential that is already a reference stays one but loses
// its default, and so does a reference whose default holds a URL
// credential. Any other value with a credential, a value that still
// shows one after the rewrite, or one in `command`, `cwd`,
// `requestOptions`, or an `x-<target>` block sets unreadable to its
// field, and then the server must be left out. It records the references
// a value already holds in referenced. A literal reports only its own
// reference, never the value around it.
func mcpURLLiterals(name string, server map[string]any, referenced map[string]bool) (literals []mcpLiteral, refs []mcpLiteralRef, unreadable string) {
	record := func(value string) {
		for _, t := range spec.EnvRefTokens(value) {
			if t.Known() {
				referenced[t.Name] = true
			}
		}
	}
	// split writes pieces back, with each credential piece creds names
	// as a reference.
	split := func(field string, pieces []string, creds map[int]string, write func(string)) {
		scrubbed := slices.Clone(pieces)
		for i := range creds {
			scrubbed[i] = spec.EnvRef("X")
		}
		if mcpCredentialDetected(strings.Join(scrubbed, "")) {
			unreadable = field
			return
		}
		v := &mcpURLValue{pieces: pieces, write: write}
		for _, i := range slices.Sorted(maps.Keys(creds)) {
			if !mcpCredentialValue(pieces[i]) {
				continue
			}
			if spec.OnlyEnvRefs(pieces[i]) {
				stripped, defaulted := spec.StripEnvRefDefaults(pieces[i])
				v.set(i, stripped)
				for _, variable := range defaulted {
					refs = append(refs, mcpLiteralRef{server: name, field: field, key: creds[i], value: stripped, variable: variable, defaulted: true})
				}
				continue
			}
			literals = append(literals, mcpLiteral{server: name, field: field, key: creds[i], secret: pieces[i],
				replace: func(text string) { v.set(i, text) },
				value:   func() string { return v.pieces[i] }})
		}
	}
	add := func(field, value string, write func(string)) {
		if unreadable != "" {
			return
		}
		record(value)
		if !mcpSingleURL(value) {
			if mcpCredentialDetected(value) {
				unreadable = field
			}
			return
		}
		var stripped []mcpLiteralRef
		for _, t := range spec.EnvRefTokens(value) {
			if t.Known() && t.HasDefault && mcpCredentialDetected(t.Default) {
				value = strings.Replace(value, t.Text, spec.EnvRef(t.Name), 1)
				stripped = append(stripped, mcpLiteralRef{server: name, field: field, key: "reference", value: spec.EnvRef(t.Name), variable: t.Name, defaulted: true})
			}
		}
		pieces, creds, ok := splitMCPURLCredentials(value)
		if !ok || creds == nil && mcpCredentialDetected(value) {
			unreadable = field
			return
		}
		write(value)
		if creds == nil {
			refs = append(refs, stripped...)
			return
		}
		split(field, pieces, creds, write)
		if unreadable == "" {
			refs = append(refs, stripped...)
		}
	}
	// addCredential reads value as one credential named key.
	addCredential := func(field, key, prefix, value string, write func(string)) {
		if unreadable != "" {
			return
		}
		record(value)
		split(field, []string{prefix, value}, map[int]string{1: key}, write)
	}
	// flagValue reports whether value is a credential flag's value: a
	// reference, whose default import strips, or a literal secret.
	flagValue := func(value string, separate bool, strength mcpCredentialStrength) bool {
		return spec.OnlyEnvRefs(value) && (!separate || !strings.HasPrefix(value, "-")) || mcpSecretValue(mcpDetectorText(value), separate, strength)
	}
	if value, ok := server["url"].(string); ok {
		add("url", value, func(s string) { server["url"] = s })
	}
	// Codex, OpenCode, and Zed import args as []string, the others as []any.
	var args []string
	var setArg func(i int, s string)
	switch raw := server["args"].(type) {
	case []any:
		args = make([]string, len(raw))
		for i, arg := range raw {
			args[i], _ = arg.(string)
		}
		setArg = func(i int, s string) { raw[i] = s }
	case []string:
		args, setArg = raw, func(i int, s string) { raw[i] = s }
	}
	for i := 0; i < len(args); i++ {
		at := i
		field := "args[" + strconv.Itoa(at) + "]"
		write := func(s string) { setArg(at, s) }
		if header, ok := strings.CutPrefix(args[at], "--header="); ok && mcpCredentialHeader(header) && unreadable == "" {
			unreadable = field
		}
		if (args[at] == "--header" || args[at] == "-H") && at+1 < len(args) && mcpCredentialHeader(args[at+1]) && unreadable == "" {
			unreadable = "args[" + strconv.Itoa(at+1) + "]"
		}
		flag, value, hasValue := mcpCredentialFlag(args[at])
		switch {
		case flag != "" && hasValue && flagValue(value, false, mcpWeakCredential):
			addCredential(field, flag, args[at][:len(args[at])-len(value)], value, write)
		// A short flag such as `-p` may be a port or profile, so only a
		// long flag reads the next argument as its value.
		case flag != "" && !hasValue && at+1 < len(args) && flagValue(args[at+1], true, mcpNameStrength(flag, false)):
			next := at + 1
			i = next
			addCredential("args["+strconv.Itoa(next)+"]", flag, "", args[next], func(s string) { setArg(next, s) })
		case mcpToken(args[at]):
			addCredential(field, "token", "", args[at], write)
		case args[at] != "":
			add(field, args[at], write)
		}
	}
	for _, field := range slices.Sorted(maps.Keys(server)) {
		if field == "command" || field == "cwd" || field == "requestOptions" || strings.HasPrefix(field, "x-") {
			walkMCPStrings(field, field, server[field], func(path, key, value string) {
				if unreadable != "" {
					return
				}
				named := mcpCredentialKey(key) && !mcpRefOnly(value) && mcpSecretValue(mcpDetectorText(value), false, mcpWeakCredential)
				if named || mcpCredentialDetected(value) {
					unreadable = path
				}
			})
		}
	}
	return literals, refs, unreadable
}

// mcpCredentialFlag returns the name of a `--name` or `--name=value`
// argument whose name ends in a credential word, and the value after
// `=`, so `--api-key` counts and `--key-id`, `--token-file`, or
// `--no-token` does not.
func mcpCredentialFlag(arg string) (name, value string, hasValue bool) {
	rest, ok := strings.CutPrefix(arg, "--")
	if !ok {
		return "", "", false
	}
	name, value, hasValue = strings.Cut(rest, "=")
	if !mcpFlagName.MatchString(name) || strings.HasPrefix(name, "no-") || !mcpCredentialName(name) {
		return "", "", false
	}
	return name, value, hasValue
}

var mcpFlagName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.\-]*$`)

// walkMCPStrings calls visit with each string in value, its path such
// as `x-claude.env.TOKEN` or `requestOptions.proxy`, and the map key it
// sits under.
func walkMCPStrings(path, key string, value any, visit func(path, key, value string)) {
	switch v := value.(type) {
	case string:
		visit(path, key, v)
	case map[string]any:
		for _, k := range slices.Sorted(maps.Keys(v)) {
			walkMCPStrings(path+"."+k, k, v[k], visit)
		}
	case []any:
		for i, item := range v {
			walkMCPStrings(path+"["+strconv.Itoa(i)+"]", key, item, visit)
		}
	case []string:
		for i, item := range v {
			walkMCPStrings(path+"["+strconv.Itoa(i)+"]", key, item, visit)
		}
	}
}

// mcpLiteralNames picks one variable per literal. An `env` value reads
// the variable its key names, and a header reads `<SERVER>_<HEADER>` in
// upper case. A name that two different values would share, or that the
// import already references, becomes `<SERVER>_<KEY>`; one still shared
// gets a `_2`, `_3` suffix in source order. Equal values share a name.
func mcpLiteralNames(literals []mcpLiteral, referenced map[string]bool) []string {
	qualified := func(l mcpLiteral) string { return strings.ToUpper(spec.EnvVarName(l.server + "_" + l.key)) }
	base := func(l mcpLiteral) string {
		if l.field == "env" {
			return spec.EnvVarName(l.key)
		}
		return qualified(l)
	}
	secrets := map[string]map[string]bool{}
	for _, l := range literals {
		n := base(l)
		if secrets[n] == nil {
			secrets[n] = map[string]bool{}
		}
		secrets[n][l.secret] = true
	}
	owner := map[string]string{}
	names := make([]string, len(literals))
	for i, l := range literals {
		n := base(l)
		if referenced[n] || len(secrets[n]) > 1 {
			n = qualified(l)
		}
		candidate := n
		for k := 2; ; k++ {
			if secret, taken := owner[candidate]; taken && secret == l.secret {
				break
			} else if !taken && !referenced[candidate] {
				owner[candidate] = l.secret
				break
			}
			candidate = n + "_" + strconv.Itoa(k)
		}
		names[i] = candidate
	}
	return names
}

func reportMCPLiteralRefs(refs []mcpLiteralRef) {
	reportMCPLiteralRefsWithHint(refs, "import does not copy env or header values or URL credentials into specs; export each variable above in the shell that starts your tool")
}

func reportMCPLiteralRefsWithHint(refs []mcpLiteralRef, hint string) {
	if len(refs) == 0 {
		return
	}
	for _, r := range refs {
		if r.leftOut {
			keptf("%s MCP server %s: left out; %s holds a credential import cannot rewrite\n", bang(), r.server, r.field)
			continue
		}
		if r.defaulted {
			keptf("%s MCP server %s: %s %s now reads %s without its default; set %s\n", bang(), r.server, r.field, r.key, r.value, r.variable)
			continue
		}
		was := ""
		switch {
		case r.ran != "":
			was = fmt.Sprintf(" (the value ran %s)", r.ran)
		case r.prompted != "":
			was = fmt.Sprintf(" (the value prompted for input %s)", r.prompted)
		}
		keptf("%s MCP server %s: %s %s now reads %s; set %s%s\n", bang(), r.server, r.field, r.key, r.value, r.variable, was)
	}
	keptf("  hint: %s\n", hint)
}
