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
	// unbraced marks a tool that also expands a bare `$NAME`, which
	// import reads back.
	unbraced bool
}

// mcpEnvRefTargets lists the reference form each target's vendor
// documents for MCP `env` and `headers` values (#1619). A target not
// listed documents none, so a reference there is left out with a note
// rather than written as text the tool never expands. Codex forwards
// variables by name instead; see forwardCodexMCPEnvRefs.
var mcpEnvRefTargets = map[string]mcpEnvRefForms{
	"claude":    {env: spec.EnvRefDollar, headers: spec.EnvRefDollar},
	"crush":     {env: spec.EnvRefDollar, headers: spec.EnvRefDollar, unbraced: true},
	"openhands": {env: spec.EnvRefDollar, headers: spec.EnvRefDollar},
	"factory":   {env: spec.EnvRefDollar, headers: spec.EnvRefDollar},
	"kiro":      {env: spec.EnvRefDollar, headers: spec.EnvRefDollar},
	"gemini":    {env: spec.EnvRefDollar, unbraced: true},
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
				values[key] = f.syntax.Read(s, forms.unbraced)
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
			if s, ok := v.(string); ok && len(spec.EnvRefNames(s)) > 0 {
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
	for _, f := range mcpEnvRefTargets[target].fields() {
		values, ok := block[f.name].(map[string]any)
		if !ok {
			continue
		}
		rewritten := make(map[string]any, len(values))
		for _, key := range slices.Sorted(maps.Keys(values)) {
			s, _ := values[key].(string)
			names := spec.EnvRefNames(s)
			switch {
			case len(names) == 0:
				rewritten[key] = values[key]
			case f.syntax == spec.EnvRefNone:
				noteMCPEnvRefDropped(target, server, f.name, key, names[0], "this field has no environment reference form")
			default:
				rewritten[key] = f.syntax.Write(s)
			}
		}
		setOrDelete(out, f.name, rewritten)
	}
	return out
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
			names := spec.EnvRefNames(s)
			if len(names) == 0 {
				kept[key] = env[key]
				continue
			}
			if name, whole := spec.WholeEnvRef(s); whole && name == key {
				if !forwardsEnvVar(forwarded, name) {
					forwarded = append(forwarded, name)
				}
				continue
			}
			noteMCPEnvRefDropped("codex", server, "env", key, names[0], "Codex forwards a variable only under its own name, as `env_vars`")
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
			names := spec.EnvRefNames(s)
			if len(names) == 0 {
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
			noteMCPEnvRefDropped("codex", server, "headers", key, names[0], "Codex reads a header from the environment only as a whole value or as `Authorization: Bearer ${NAME}`")
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

func noteMCPEnvRefDropped(target, server, field, key, name, why string) {
	NoteFieldNoOp(target, spec.KindMCP, field+"."+key, 1,
		fmt.Sprintf("server %s reads %s: %s, so sync leaves the key out instead of writing the reference as text", server, spec.EnvRef(name), why))
}
