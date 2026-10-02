package emit

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

type mcpEnvRefForms struct {
	env, headers spec.EnvRefSyntax
	// url and args are the forms the tool expands in a remote server's
	// `url` and in each element of a stdio server's `args`.
	url, args spec.EnvRefSyntax
	// defaults marks a tool that documents `${NAME:-default}`.
	defaults bool
	// reading lists the other forms the tool expands, which import
	// reads back.
	reading spec.EnvRefReading
}

// mcpEnvRefTargets lists the reference form each target's vendor
// documents for MCP `env` and `headers` values (#1619), and for `url`
// and `args` (#1633). A target or field not listed documents none, so a
// reference there is left out with a note rather than written as text
// the tool never expands. Codex forwards env and header variables by
// name instead; see forwardCodexMCPEnvRefs.
//
// A `${NAME:-default}` is left out where the tool documents no default,
// not narrowed to `${NAME}`: the default may be what lets the server
// start, and Factory fails a connection on an unset variable.
var mcpEnvRefTargets = map[string]mcpEnvRefForms{
	// url, args: https://code.claude.com/docs/en/mcp#environment-variable-expansion-in-mcp-json
	"claude": {env: spec.EnvRefDollar, headers: spec.EnvRefDollar, url: spec.EnvRefDollar, args: spec.EnvRefDollar, defaults: true},
	// url, args: ResolvedURL and ResolvedArgs in https://github.com/charmbracelet/crush/blob/bdcf796cb1ff241b0eb18139071d46a84dc1ee91/internal/config/config.go
	"crush": {env: spec.EnvRefDollar, headers: spec.EnvRefDollar, url: spec.EnvRefDollar, args: spec.EnvRefDollar, defaults: true, reading: spec.EnvRefReading{Unbraced: true}},
	// url, args: expand_mcp_variables in https://github.com/OpenHands/software-agent-sdk/blob/35410b3af87e672b0264ba711755053c9d517bef/openhands-sdk/openhands/sdk/skills/utils.py
	"openhands": {env: spec.EnvRefDollar, headers: spec.EnvRefDollar, url: spec.EnvRefDollar, args: spec.EnvRefDollar, defaults: true},
	"factory":   {env: spec.EnvRefDollar, headers: spec.EnvRefDollar},
	"kiro":      {env: spec.EnvRefDollar, headers: spec.EnvRefDollar},
	// https://geminicli.com/docs/reference/configuration: every string value in settings.json, defaults included
	"gemini": {env: spec.EnvRefDollar, headers: spec.EnvRefDollar, url: spec.EnvRefDollar, args: spec.EnvRefDollar, defaults: true, reading: spec.EnvRefReading{Unbraced: true, Percent: true}},
	// url: https://ampcode.com/docs/customize/mcp
	"amp": {headers: spec.EnvRefDollar, url: spec.EnvRefDollar},
	// url, args: https://cursor.com/docs/mcp#config-interpolation
	"cursor": {env: spec.EnvRefDollarEnv, headers: spec.EnvRefDollarEnv, url: spec.EnvRefDollarEnv, args: spec.EnvRefDollarEnv},
	// url, args: https://docs.devin.ai/desktop/cascade/mcp#config-interpolation
	"windsurf": {env: spec.EnvRefDollarEnv, headers: spec.EnvRefDollarEnv, url: spec.EnvRefDollarEnv, args: spec.EnvRefDollarEnv},
	// url, args: https://opencode.ai/docs/config#variables
	"opencode": {env: spec.EnvRefBraceEnv, headers: spec.EnvRefBraceEnv, url: spec.EnvRefBraceEnv, args: spec.EnvRefBraceEnv},
	// https://docs.continue.dev/guides/configuring-models-rules-tools#working-with-secrets
	// args: https://docs.continue.dev/customize/deep-dives/mcp#how-to-work-with-secrets-in-mcp-servers
	// url: fillTemplateVariables over the whole file in https://github.com/continuedev/continue/blob/5522c6f44ca0ac3528b37244818fbfa39b5af470/packages/config-yaml/src/load/unroll.ts
	"continue": {env: spec.EnvRefSecrets, headers: spec.EnvRefSecrets, url: spec.EnvRefSecrets, args: spec.EnvRefSecrets},
}

// ReadMCPEnvRefs rewrites target's own reference form in a native
// server's `env`, `headers`, `url`, and `args` values back to the spec's
// `${NAME}`, in place. A literal url or argument stays as it is.
func ReadMCPEnvRefs(target string, server map[string]any) {
	forms := mcpEnvRefTargets[target]
	for _, f := range forms.fields() {
		values := f.values(server)
		for key, v := range values {
			if s, ok := v.(string); ok {
				values[key] = f.syntax.Read(s, forms.reading)
			}
		}
	}
	// `%NAME%` is documented for Gemini `env` only.
	reading := spec.EnvRefReading{Unbraced: forms.reading.Unbraced}
	for _, field := range mcpLaunchFields {
		syntax := forms.launchSyntax(field)
		if v, ok := server[field]; ok && syntax != spec.EnvRefNone {
			server[field] = mapLaunchValue(v, func(s string) string { return syntax.ReadLaunch(s, reading) })
		}
	}
}

type mcpEnvRefField struct {
	name   string
	syntax spec.EnvRefSyntax
}

func (f mcpEnvRefForms) fields() []mcpEnvRefField {
	fields := []mcpEnvRefField{{"env", f.env}, {"headers", f.headers}}
	if f.headers == spec.EnvRefSecrets {
		fields = append(fields, mcpEnvRefField{"requestOptions.headers", f.headers})
	}
	return fields
}

func (f mcpEnvRefField) values(block map[string]any) map[string]any {
	key := f.name
	if parent, child, nested := strings.Cut(key, "."); nested {
		block, _ = block[parent].(map[string]any)
		key = child
	}
	values, _ := block[key].(map[string]any)
	return values
}

func (f mcpEnvRefField) setValues(block, values map[string]any) {
	if parent, child, nested := strings.Cut(f.name, "."); nested {
		old, _ := block[parent].(map[string]any)
		copied := maps.Clone(old)
		setOrDelete(copied, child, values)
		setOrDelete(block, parent, copied)
		return
	}
	setOrDelete(block, f.name, values)
}

// RewriteMCPEnvRefs returns mcps with every `${NAME}` in an `env`,
// `headers`, `url`, or `args` value, top level or under `x-<target>`,
// written in target's own form. An env or header value target cannot
// reference is left out and noted, so a reference never lands as a
// literal. A server whose emitted url or args, as view reads them,
// holds such a reference is left out whole, since dropping one argument
// changes the command. Neither
// the slice nor its Meta maps are mutated.
func RewriteMCPEnvRefs(target string, view MCPLaunchView, mcps []spec.Entry) []spec.Entry {
	out := make([]spec.Entry, 0, len(mcps))
	for _, e := range mcps {
		if field, token, why, ok := unwritableLaunchRef(target, view, e.Meta); ok {
			NoteFieldNoOp(target, spec.KindMCP, field, 1,
				fmt.Sprintf("server %s reads %s in `%s`: %s, so sync leaves the server out instead of writing the reference as text", e.Name, token.Display(), field, why))
			continue
		}
		if hasMCPEnvRef(e.Meta) || hasMCPEnvRef(xBlock(e.Meta, target)) {
			meta := rewriteMCPEnvRefBlock(target, e.Name, e.Meta)
			if x := xBlock(e.Meta, target); x != nil {
				meta[XPrefix+target] = rewriteMCPEnvRefBlock(target, e.Name, x)
			}
			e.Meta = meta
		}
		if hasLaunchRef(e.Meta) || hasLaunchRef(xBlock(e.Meta, target)) {
			e.Meta = rewriteLaunchRefs(target, e.Meta)
		}
		out = append(out, e)
	}
	return out
}

// mcpLaunchFields are the fields that say where a server runs, as
// opposed to the credentials in `env` and `headers`.
var mcpLaunchFields = []string{"url", "args"}

func (f mcpEnvRefForms) launchSyntax(field string) spec.EnvRefSyntax {
	if field == "url" {
		return f.url
	}
	return f.args
}

// launchStrings returns the url, or each string argument.
func launchStrings(v any) []string {
	switch v := v.(type) {
	case string:
		return []string{v}
	case []any:
		var out []string
		for _, arg := range v {
			if s, ok := arg.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return v
	}
	return nil
}

// mapLaunchValue applies fn to the url, or to each string argument,
// and returns a copy.
func mapLaunchValue(v any, fn func(string) string) any {
	switch v := v.(type) {
	case string:
		return fn(v)
	case []any:
		out := make([]any, len(v))
		for i, arg := range v {
			if s, ok := arg.(string); ok {
				out[i] = fn(s)
			} else {
				out[i] = arg
			}
		}
		return out
	case []string:
		out := make([]string, len(v))
		for i, s := range v {
			out[i] = fn(s)
		}
		return out
	}
	return v
}

// launchRefs returns the environment references in a url or argument.
// An editor variable such as `${workspaceFolder}`, and any other
// `${...}`, is the tool's own text and not a reference.
func launchRefs(value string) []spec.EnvRefToken {
	var refs []spec.EnvRefToken
	for _, t := range spec.EnvRefTokens(value) {
		if t.Known() && !t.EditorVariable() {
			refs = append(refs, t)
		}
	}
	return refs
}

func hasLaunchRef(block map[string]any) bool {
	for _, field := range mcpLaunchFields {
		for _, s := range launchStrings(block[field]) {
			if len(launchRefs(s)) > 0 {
				return true
			}
		}
	}
	return false
}

// MCPLaunchView says which `url` and `args` values a target's MCP
// writer emits, so a server is left out only for a reference the
// written file would hold. The zero value is a writer that reads the
// top-level fields and ignores `x-<target>` for them.
type MCPLaunchView struct {
	// Resolved marks a writer that reads ResolveMeta(meta, target), so
	// an `x-<target>` key replaces the top-level one, `type` included,
	// before the transport is picked.
	Resolved bool
	// Passthrough lists the launch fields the writer copies from
	// `x-<target>` as written, whatever the transport.
	Passthrough []string
}

// LaunchPassthrough returns the launch fields a MergeCustomTargetMeta
// call with these exclusions copies through.
func LaunchPassthrough(excluded ...string) []string {
	var out []string
	for _, field := range mcpLaunchFields {
		if !slices.Contains(excluded, field) {
			out = append(out, field)
		}
	}
	return out
}

// emittedLaunchValues returns, per launch field, the strings view's
// writer puts in the file: `url` for a remote transport, `args` for
// stdio, plus any field copied through from `x-<target>`.
func (v MCPLaunchView) emittedLaunchValues(target string, meta map[string]any) map[string][]string {
	base := meta
	if v.Resolved {
		base = ResolveMeta(meta, target)
	}
	// Writers spell remote transports many ways (http, sse,
	// streamable-http, ...), so anything not stdio counts as remote.
	field := "args"
	switch transport, _ := base["type"].(string); transport {
	case "stdio", "local":
	case "":
		if _, hasCommand := base["command"]; !hasCommand && base["url"] != nil {
			field = "url"
		}
	default:
		field = "url"
	}
	out := map[string][]string{field: launchStrings(base[field])}
	if !v.Resolved {
		x := xBlock(meta, target)
		for _, f := range v.Passthrough {
			out[f] = append(out[f], launchStrings(x[f])...)
		}
	}
	return out
}

// unwritableLaunchRef returns the first reference in the url or args
// target's writer emits that target cannot write.
func unwritableLaunchRef(target string, view MCPLaunchView, meta map[string]any) (string, spec.EnvRefToken, string, bool) {
	forms := mcpEnvRefTargets[target]
	values := view.emittedLaunchValues(target, meta)
	for _, field := range mcpLaunchFields {
		for _, s := range values[field] {
			for _, t := range launchRefs(s) {
				switch {
				case forms.launchSyntax(field) == spec.EnvRefNone:
					return field, t, "this tool documents no environment reference in that field", true
				case t.HasDefault && !forms.defaults:
					return field, t, "this tool documents no default value for a reference", true
				}
			}
		}
	}
	return "", spec.EnvRefToken{}, "", false
}

func rewriteLaunchRefs(target string, meta map[string]any) map[string]any {
	forms := mcpEnvRefTargets[target]
	rewrite := func(block map[string]any) map[string]any {
		out := maps.Clone(block)
		for _, field := range mcpLaunchFields {
			if v, ok := block[field]; ok {
				out[field] = mapLaunchValue(v, forms.launchSyntax(field).WriteLaunch)
			}
		}
		return out
	}
	out := rewrite(meta)
	if x := xBlock(meta, target); x != nil {
		out[XPrefix+target] = rewrite(x)
	}
	return out
}

func xBlock(meta map[string]any, target string) map[string]any {
	x, _ := meta[XPrefix+target].(map[string]any)
	return x
}

func hasMCPEnvRef(block map[string]any) bool {
	for _, name := range []string{"env", "headers", "requestOptions.headers"} {
		values := (mcpEnvRefField{name: name}).values(block)
		for _, v := range values {
			if s, ok := v.(string); ok && spec.HasEnvRef(s) {
				return true
			}
		}
	}
	return false
}

func rewriteMCPEnvRefBlock(target, server string, block map[string]any) map[string]any {
	out := maps.Clone(block)
	if target == "codex" {
		forwardCodexMCPEnvRefs(server, out)
		return out
	}
	forms := mcpEnvRefTargets[target]
	for _, f := range forms.fields() {
		values := f.values(block)
		if values == nil {
			continue
		}
		rewritten := make(map[string]any, len(values))
		for _, key := range slices.Sorted(maps.Keys(values)) {
			s, _ := values[key].(string)
			if !spec.HasEnvRef(s) {
				rewritten[key] = values[key]
				continue
			}
			if token, why, ok := forms.unwritable(f.syntax, s); ok {
				noteMCPEnvRefDropped(target, server, f.name, key, token, why)
				continue
			}
			rewritten[key] = f.syntax.Write(s)
		}
		f.setValues(out, rewritten)
	}
	return out
}

// unwritable returns the first token in value that syntax cannot write,
// and why.
func (f mcpEnvRefForms) unwritable(syntax spec.EnvRefSyntax, value string) (spec.EnvRefToken, string, bool) {
	for _, t := range spec.EnvRefTokens(value) {
		switch {
		case !t.Known():
			return t, "only `${NAME}` and `${NAME:-default}` are environment references in a spec", true
		case syntax == spec.EnvRefNone:
			return t, "this field has no environment reference form", true
		case t.HasDefault && !f.defaults:
			return t, "this tool documents no default value for a reference", true
		}
	}
	return spec.EnvRefToken{}, "", false
}

// unforwardable returns the first token in value, for a value Codex
// cannot forward by name.
func unforwardable(value string) spec.EnvRefToken {
	tokens := spec.EnvRefTokens(value)
	for _, t := range tokens {
		if !t.Known() || t.HasDefault {
			return t
		}
	}
	return tokens[0]
}

// forwardCodexMCPEnvRefs moves references into the keys Codex forwards
// by name: an `env` value of exactly `${KEY}` joins `env_vars`, a header
// `Authorization: Bearer ${NAME}` becomes `bearer_token_env_var`, and
// any other header of exactly `${NAME}` joins `env_http_headers`.
// developers.openai.com/codex/config-reference documents no inline
// expansion, so every other reference is left out.
func forwardCodexMCPEnvRefs(server string, block map[string]any) {
	if env, ok := block["env"].(map[string]any); ok {
		kept := map[string]any{}
		forwarded, _ := block["env_vars"].([]any)
		forwarded = slices.Clone(forwarded)
		for _, key := range slices.Sorted(maps.Keys(env)) {
			s, _ := env[key].(string)
			if !spec.HasEnvRef(s) {
				kept[key] = env[key]
				continue
			}
			name, whole := spec.WholeEnvRef(s)
			switch {
			case whole && name == key:
				if !forwardsEnvVar(forwarded, name) {
					forwarded = append(forwarded, name)
				}
			case whole:
				NoteFieldNoOp("codex", spec.KindMCP, "env."+key, 1,
					fmt.Sprintf("server %s: `env_vars` cannot rename a variable, so sync leaves `env.%s` out of .codex/config.toml. Name the variable after the key (%s) to forward it", server, key, spec.EnvRef(key)))
			default:
				noteMCPEnvRefDropped("codex", server, "env", key, unforwardable(s), "Codex forwards only a whole `${NAME}`, as `env_vars`")
			}
		}
		setOrDelete(block, "env", kept)
		if len(forwarded) > 0 {
			block["env_vars"] = forwarded
		}
	}
	if headers, ok := block["headers"].(map[string]any); ok {
		kept := map[string]any{}
		envHeaders, _ := block["env_http_headers"].(map[string]any)
		envHeaders = maps.Clone(envHeaders)
		if envHeaders == nil {
			envHeaders = map[string]any{}
		}
		for _, key := range slices.Sorted(maps.Keys(headers)) {
			s, _ := headers[key].(string)
			if !spec.HasEnvRef(s) {
				kept[key] = headers[key]
				continue
			}
			if token, bearer := strings.CutPrefix(s, "Bearer "); bearer && strings.EqualFold(key, "Authorization") {
				if name, whole := spec.WholeEnvRef(token); whole && block["bearer_token_env_var"] == nil {
					block["bearer_token_env_var"] = name
					continue
				}
			}
			if name, whole := spec.WholeEnvRef(s); whole {
				if _, set := envHeaders[key]; !set {
					envHeaders[key] = name
				}
				continue
			}
			noteMCPEnvRefDropped("codex", server, "headers", key, unforwardable(s), "Codex reads a header from the environment only as a whole value or as `Authorization: Bearer ${NAME}`")
		}
		setOrDelete(block, "headers", kept)
		setOrDelete(block, "env_http_headers", envHeaders)
	}
}

func forwardsEnvVar(entries []any, name string) bool {
	for _, entry := range entries {
		switch v := entry.(type) {
		case string:
			if v == name {
				return true
			}
		case map[string]any:
			if v["name"] == name {
				return true
			}
		}
	}
	return false
}

func setOrDelete(block map[string]any, key string, values map[string]any) {
	if len(values) == 0 {
		delete(block, key)
		return
	}
	block[key] = values
}

func noteMCPEnvRefDropped(target, server, field, key string, token spec.EnvRefToken, why string) {
	NoteFieldNoOp(target, spec.KindMCP, field+"."+key, 1,
		fmt.Sprintf("server %s reads %s: %s, so sync leaves the key out instead of writing the reference as text", server, token.Display(), why))
}
