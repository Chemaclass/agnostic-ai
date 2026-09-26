package cli

import (
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// localHook is the identity of one local hook spec. Native hook settings
// keep no spec name, so an imported hook is matched on its event,
// matcher, and handlers instead.
type localHook struct {
	name     string
	event    string
	matcher  string
	handlers map[string]bool
}

// newLocalHooks indexes the local layer's hook specs.
func newLocalHooks(entries []spec.Entry) []localHook {
	var out []localHook
	for _, e := range entries {
		h := localHook{
			name:     e.Name,
			event:    hookEventKey(e.Meta),
			matcher:  hookMatcher(e.Meta),
			handlers: map[string]bool{},
		}
		for _, k := range hookHandlerKeys(e.Meta) {
			h.handlers[k] = true
		}
		out = append(out, h)
	}
	return out
}

// hookEventKey folds case, dashes, and underscores, so `pre-tool-use`,
// `pre_tool_use`, and `PreToolUse` name one event.
func hookEventKey(meta map[string]any) string {
	event, _ := meta["event"].(string)
	return strings.NewReplacer("-", "", "_", "").Replace(strings.ToLower(strings.TrimSpace(event)))
}

func hookMatcher(meta map[string]any) string {
	m, _ := meta["matcher"].(string)
	return m
}

// hookHandlerKeys returns one key per handler a hook spec declares: each
// command of a command hook, or the payload of an http, mcp_tool, or
// prompt hook.
func hookHandlerKeys(meta map[string]any) []string {
	str := func(k string) string { s, _ := meta[k].(string); return s }
	switch str("type") {
	case "", "command":
		var keys []string
		for _, c := range hookCommands(meta) {
			keys = append(keys, "command\x00"+c)
		}
		return keys
	case "http":
		return []string{"http\x00" + str("url")}
	case "mcp_tool":
		return []string{"mcp_tool\x00" + str("server") + "\x00" + str("tool")}
	case "prompt":
		return []string{"prompt\x00" + str("prompt")}
	}
	return nil
}

// hookCommands returns a command hook's commands, one or a list.
func hookCommands(meta map[string]any) []string {
	switch c := meta["command"].(type) {
	case string:
		return []string{c}
	case []any:
		var out []string
		for _, v := range c {
			if s, ok := v.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// hookOwners returns the local hooks that own handlers of the imported
// hook doc, and whether every handler it holds is local.
func (g *localImportGuard) hookOwners(meta map[string]any) ([]string, bool) {
	event, matcher := hookEventKey(meta), hookMatcher(meta)
	keys := hookHandlerKeys(meta)
	if event == "" || len(keys) == 0 {
		return nil, false
	}
	var owners []string
	all := true
	for _, k := range keys {
		owner := g.hookOwner(event, matcher, k)
		if owner == "" {
			all = false
			continue
		}
		owners = append(owners, owner)
	}
	return owners, all
}

// hookOwner names the local hook that declares handler key for event
// and matcher, or returns "".
func (g *localImportGuard) hookOwner(event, matcher, key string) string {
	for _, h := range g.hooks {
		if h.event == event && h.matcher == matcher && h.handlers[key] {
			return h.name
		}
	}
	return ""
}

// localHookName reports whether an imported hook doc holds only local
// handlers, and names the local hooks it came from.
func (g *localImportGuard) localHookName(data []byte) (string, bool) {
	e, err := spec.ParseYAMLBytes(spec.KindHook, data)
	if err != nil {
		return "", false
	}
	owners, all := g.hookOwners(e.Meta)
	if !all || len(owners) == 0 {
		return "", false
	}
	for _, o := range owners[1:] {
		g.skipped[string(spec.KindHook)+" "+o] = true
	}
	return owners[0], true
}

// stripLocalHandlers drops the local commands from an imported command
// hook that also holds shared ones: Claude groups every hook of one
// event and matcher into a single native entry. Any other write comes
// back unchanged.
func (g *localImportGuard) stripLocalHandlers(path string, data []byte) []byte {
	if len(g.hooks) == 0 {
		return data
	}
	if kind, _, _, ok := g.locate(path); !ok || kind != spec.KindHook {
		return data
	}
	e, err := spec.ParseYAMLBytes(spec.KindHook, data)
	if err != nil {
		return data
	}
	if typ, _ := e.Meta["type"].(string); typ != "" && typ != "command" {
		return data
	}
	owners, all := g.hookOwners(e.Meta)
	if all || len(owners) == 0 {
		return data
	}
	event, matcher := hookEventKey(e.Meta), hookMatcher(e.Meta)
	var kept []any
	for _, c := range hookCommands(e.Meta) {
		if g.hookOwner(event, matcher, "command\x00"+c) == "" {
			kept = append(kept, c)
		}
	}
	if len(kept) == 1 {
		e.Meta["command"] = kept[0]
	} else {
		e.Meta["command"] = kept
	}
	out, err := yaml.Marshal(e.Meta)
	if err != nil {
		return data
	}
	for _, o := range owners {
		g.skipped[string(spec.KindHook)+" "+o] = true
	}
	return out
}
