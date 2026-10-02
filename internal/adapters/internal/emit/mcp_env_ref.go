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
	// defaults marks a tool that documents `${NAME:-default}`.
	defaults bool
	// reading lists the other forms the tool expands, which import
	// reads back.
	reading spec.EnvRefReading
}

// mcpEnvRefTargets lists the reference form each target's vendor
// documents for MCP `env` and `headers` values (#1619). A target not
// listed documents none, so a reference there is left out with a note
// rather than written as text the tool never expands. Codex forwards
// variables by name instead; see forwardCodexMCPEnvRefs.
//
// A `${NAME:-default}` is left out where the tool documents no default,
// not narrowed to `${NAME}`: the default may be what lets the server
// start, and Factory fails a connection on an unset variable.
var mcpEnvRefTargets = map[string]mcpEnvRefForms{
	"claude":    {env: spec.EnvRefDollar, headers: spec.EnvRefDollar, defaults: true},
	"crush":     {env: spec.EnvRefDollar, headers: spec.EnvRefDollar, defaults: true, reading: spec.EnvRefReading{Unbraced: true}},
	"openhands": {env: spec.EnvRefDollar, headers: spec.EnvRefDollar, defaults: true},
	"factory":   {env: spec.EnvRefDollar, headers: spec.EnvRefDollar},
	"kiro":      {env: spec.EnvRefDollar, headers: spec.EnvRefDollar},
	"gemini":    {env: spec.EnvRefDollar, reading: spec.EnvRefReading{Unbraced: true, Percent: true}},
	"amp":       {headers: spec.EnvRefDollar},
	"cursor":    {env: spec.EnvRefDollarEnv, headers: spec.EnvRefDollarEnv},
	"windsurf":  {env: spec.EnvRefDollarEnv, headers: spec.EnvRefDollarEnv},
	"opencode":  {env: spec.EnvRefBraceEnv, headers: spec.EnvRefBraceEnv},
}

// ReadMCPEnvRefs rewrites target's own reference form in a native
// server's `env` and `headers` values back to the spec's `${NAME}`, in
// place.
func ReadMCPEnvRefs(target string, server map[string]any) {
	forms := mcpEnvRefTargets[target]
	for _, f := range forms.fields() {
		values, _ := server[f.name].(map[string]any)
		for key, v := range values {
			if s, ok := v.(string); ok {
				values[key] = f.syntax.Read(s, forms.reading)
			}
		}
	}
}

type mcpEnvRefField struct {
	name   string
	syntax spec.EnvRefSyntax
}

func (f mcpEnvRefForms) fields() []mcpEnvRefField {
	return []mcpEnvRefField{{"env", f.env}, {"headers", f.headers}}
}

// RewriteMCPEnvRefs returns mcps with every `${NAME}` in an `env` or
// `headers` value, top level or under `x-<target>`, written in target's
// own form. A value target cannot reference is left out and noted, so a
// reference never lands as a literal. Neither the slice nor its Meta
// maps are mutated.
func RewriteMCPEnvRefs(target string, mcps []spec.Entry) []spec.Entry {
	out := make([]spec.Entry, len(mcps))
	for i, e := range mcps {
		if hasMCPEnvRef(e.Meta) || hasMCPEnvRef(xBlock(e.Meta, target)) {
			meta := rewriteMCPEnvRefBlock(target, e.Name, e.Meta)
			if x := xBlock(e.Meta, target); x != nil {
				meta[XPrefix+target] = rewriteMCPEnvRefBlock(target, e.Name, x)
			}
			e.Meta = meta
		}
		out[i] = e
	}
	return out
}

func xBlock(meta map[string]any, target string) map[string]any {
	x, _ := meta[XPrefix+target].(map[string]any)
	return x
}

func hasMCPEnvRef(block map[string]any) bool {
	for _, field := range []string{"env", "headers"} {
		values, _ := block[field].(map[string]any)
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
		values, ok := block[f.name].(map[string]any)
		if !ok {
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
		setOrDelete(out, f.name, rewritten)
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
			if name, whole := spec.WholeEnvRef(s); whole && name == key {
				if !forwardsEnvVar(forwarded, name) {
					forwarded = append(forwarded, name)
				}
				continue
			}
			noteMCPEnvRefDropped("codex", server, "env", key, unforwardable(s), "Codex forwards a variable only whole and under its own name, as `env_vars`")
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
