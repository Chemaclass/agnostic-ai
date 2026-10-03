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

// mcpCredentialParams are the query parameter names, split on
// punctuation, that import treats as a credential.
var mcpCredentialParams = map[string]bool{
	"token": true, "key": true, "secret": true, "password": true, "passwd": true, "pwd": true,
	"apikey": true, "accesstoken": true, "authtoken": true,
}

func mcpCredentialParam(name string) bool {
	if unescaped, err := url.QueryUnescape(name); err == nil {
		name = unescaped
	}
	for _, word := range strings.FieldsFunc(strings.ToLower(name), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		if mcpCredentialParams[word] {
			return true
		}
	}
	return false
}

func mcpCredentialValue(value string) bool {
	return value != "" && !spec.OnlyEscapedEnvRefs(value)
}

// splitMCPURLCredentials finds the `user:password@` password and each
// credential query parameter in the first URL inside value. A URL whose
// base is a reference (`${API_BASE}/mcp?token=x`) has no parseable
// password, but its query still counts. It returns the value split into
// pieces and, for each credential, its piece index and name. It reads
// the text by hand because a URL may hold `${NAME}` references that
// net/url rejects.
func splitMCPURLCredentials(value string) ([]string, map[int]string) {
	var pieces []string
	creds := map[int]string{}
	text, rest := "", value
	credential := func(name, secret string) {
		pieces = append(pieces, text, secret)
		creds[len(pieces)-1] = name
		text = ""
	}
	if scheme := strings.Index(value, "://"); scheme >= 0 {
		text, rest = value[:scheme+3], value[scheme+3:]
		end := strings.IndexAny(rest, "/?#")
		if end < 0 {
			end = len(rest)
		}
		authority := rest[:end]
		if at := strings.LastIndex(authority, "@"); at >= 0 {
			if user, password, ok := strings.Cut(authority[:at], ":"); ok && mcpCredentialValue(password) {
				text += user + ":"
				credential("password", password)
				authority = authority[at:]
			}
		}
		text += authority
		rest = rest[end:]
	} else if base, _, ok := strings.Cut(value, "?"); !ok || !strings.Contains(base, "${") {
		return nil, nil
	}
	fragment := ""
	if hash := strings.Index(rest, "#"); hash >= 0 {
		rest, fragment = rest[:hash], rest[hash:]
	}
	path, query, hasQuery := strings.Cut(rest, "?")
	text += path
	if hasQuery {
		text += "?"
		for i, pair := range strings.Split(query, "&") {
			if i > 0 {
				text += "&"
			}
			name, secret, ok := strings.Cut(pair, "=")
			if ok && mcpCredentialParam(name) && mcpCredentialValue(secret) {
				text += name + "="
				credential(name, secret)
				continue
			}
			text += pair
		}
	}
	if len(creds) == 0 {
		return nil, nil
	}
	return append(pieces, text+fragment), creds
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
		urlLiterals, urlRefs := mcpURLLiterals(name, server, referenced)
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
// reference stays one but loses its default. It records the references
// a URL already holds in referenced. A literal reports only its own
// reference, never the URL around it.
func mcpURLLiterals(name string, server map[string]any, referenced map[string]bool) ([]mcpLiteral, []mcpLiteralRef) {
	var literals []mcpLiteral
	var refs []mcpLiteralRef
	add := func(field, value string, write func(string)) {
		for _, t := range spec.EnvRefTokens(value) {
			if t.Known() {
				referenced[t.Name] = true
			}
		}
		pieces, creds := splitMCPURLCredentials(value)
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
	return literals, refs
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
