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

// mcpCredentialParams are the words of a query parameter name that make
// import treat it as a credential.
var mcpCredentialParams = map[string]bool{
	"token": true, "key": true, "secret": true, "password": true, "passwd": true, "pwd": true,
	"apikey": true, "accesstoken": true, "authtoken": true,
}

func mcpCredentialParam(name string) bool {
	if unescaped, err := url.QueryUnescape(name); err == nil {
		name = unescaped
	}
	for _, word := range mcpNameWords(name) {
		if mcpCredentialParams[word] {
			return true
		}
	}
	return false
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
	mcpQueryParamPattern = regexp.MustCompile(`[?&]([^=&?#\s'"]+)=([^&?#\s'"]*)`)
	mcpRefOnlyPattern    = regexp.MustCompile(`^\$?\$\{[A-Za-z_][A-Za-z0-9_]*\}[;&|)}]*$`)
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

func mcpLiteralCredential(value string) bool {
	return value != "" && !mcpRefOnlyPattern.MatchString(value)
}

// mcpURLCredentialDetected reports whether value holds a literal URL
// password or a literal credential query parameter anywhere, inside
// references and escaped references included. An `@` after a `/` counts
// unless the text before the `/` is a host with a port, as in
// `https://host:8443/@scope/pkg`.
func mcpURLCredentialDetected(value string) bool {
	for _, m := range mcpQueryParamPattern.FindAllStringSubmatch(value, -1) {
		if mcpCredentialParam(m[1]) && mcpLiteralCredential(m[2]) {
			return true
		}
	}
	for rest := value; ; {
		sep := strings.Index(rest, "://")
		if sep < 0 {
			return false
		}
		rest = rest[sep+3:]
		at := strings.Index(rest[:indexAnyOrLen(rest, " \t\r\n")], "@")
		if at < 0 {
			continue
		}
		user, mask := rest[:at], maskMCPRefs(rest[:at])
		if slash := strings.Index(mask, "/"); slash >= 0 {
			if _, port, ok := strings.Cut(mask[:slash], ":"); !ok || port != "" && mcpPortOrRef(port) {
				continue
			}
		}
		if colon := strings.Index(mask, ":"); colon >= 0 && mcpLiteralCredential(user[colon+1:]) {
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

// mcpPortOrRef reports whether the text after a host's `:` is a port, or
// a masked reference standing for one.
func mcpPortOrRef(s string) bool {
	return strings.Trim(s, "0123456789_") == ""
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
			if colon := strings.Index(authority[:at], ":"); colon >= 0 {
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
		for start := pos + q + 1; start <= end; {
			stop := start + indexAnyOrLen(mask[start:end], "&")
			if eq := strings.Index(mask[start:stop], "="); eq >= 0 {
				if name := word[start : start+eq]; mcpCredentialParam(name) && mcpCredentialValue(word[start+eq+1:stop]) {
					spans = append(spans, mcpCredentialSpan{name, start + eq + 1, stop})
				}
			}
			start = stop + 1
		}
	}
	return spans, true
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
// a URL password and each query parameter mcpCredentialParam names. The
// rest of the URL stays as written. A credential that is already a
// reference stays one but loses its default, and so does a reference
// whose default holds a URL credential. Only a value that is one URL is
// rewritten; any other value with a credential, or a URL that still
// shows one after the rewrite, sets unreadable to its field, and then
// the server must be left out. It records the references a value
// already holds in referenced. A literal reports only its own reference,
// never the URL around it.
func mcpURLLiterals(name string, server map[string]any, referenced map[string]bool) (literals []mcpLiteral, refs []mcpLiteralRef, unreadable string) {
	add := func(field, value string, write func(string)) {
		if unreadable != "" {
			return
		}
		for _, t := range spec.EnvRefTokens(value) {
			if t.Known() {
				referenced[t.Name] = true
			}
		}
		if !mcpSingleURL(value) {
			if mcpURLCredentialDetected(value) {
				unreadable = field
			}
			return
		}
		var stripped []mcpLiteralRef
		for _, t := range spec.EnvRefTokens(value) {
			if t.Known() && t.HasDefault && mcpURLCredentialDetected(t.Default) {
				value = strings.Replace(value, t.Text, spec.EnvRef(t.Name), 1)
				stripped = append(stripped, mcpLiteralRef{server: name, field: field, key: "reference", value: spec.EnvRef(t.Name), variable: t.Name, defaulted: true})
			}
		}
		pieces, creds, ok := splitMCPURLCredentials(value)
		scrubbed := slices.Clone(pieces)
		for i := range creds {
			scrubbed[i] = spec.EnvRef("X")
		}
		if !ok || mcpURLCredentialDetected(strings.Join(scrubbed, "")) || creds == nil && mcpURLCredentialDetected(value) {
			unreadable = field
			return
		}
		write(value)
		refs = append(refs, stripped...)
		if creds == nil {
			return
		}
		v := &mcpURLValue{pieces: pieces, write: write}
		for _, i := range slices.Sorted(maps.Keys(creds)) {
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
	if value, ok := server["url"].(string); ok {
		add("url", value, func(s string) { server["url"] = s })
	}
	args, _ := server["args"].([]any)
	for i, arg := range args {
		if value, ok := arg.(string); ok {
			add("args["+strconv.Itoa(i)+"]", value, func(s string) { args[i] = s })
		}
	}
	return literals, refs, unreadable
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
			keptf("%s MCP server %s: left out; %s holds a URL credential import cannot rewrite\n", bang(), r.server, r.field)
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
