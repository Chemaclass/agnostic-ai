package cli

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// hookOwners indexes one layer's hook specs by what a target renders
// from them. Native hook settings keep no spec name, so an imported
// hook is matched on its event, matcher, and handler instead.
type hookOwners struct {
	entries []spec.Entry
	cfg     *config.Config
	// byTarget maps, per target, an event and handler to the specs that
	// render it. Built on first use: only the imported targets need one.
	byTarget map[string]map[string][]hookOwnerMatcher
}

// hookOwnerMatcher is one spec's matcher for an event and handler.
type hookOwnerMatcher struct {
	matcher, name string
}

func newHookOwners(entries []spec.Entry, cfg *config.Config) *hookOwners {
	return &hookOwners{entries: entries, cfg: cfg, byTarget: map[string]map[string][]hookOwnerMatcher{}}
}

// owner names the hook spec whose target rendering holds handler key
// for event and matcher, or returns "". A command handler is what sync
// writes. Another handler's payload counts in the spec's own view and its
// `x-<target>` view. Both sit under the event and matcher sync writes. A
// target may join matchers on emit, so a handler
// whose matcher covers a spec's is that spec's, even when another spec
// feeds it too.
func (l *hookOwners) owner(target, event, matcher, key string) string {
	index, ok := l.byTarget[target]
	if !ok {
		index = map[string][]hookOwnerMatcher{}
		for _, e := range l.entries {
			if !e.EmitsTo(target) {
				continue
			}
			native, reason := e.NativeHook(target)
			if reason != "" {
				continue
			}
			// Sync reads the event and matcher after the `x-<target>`
			// override on every target.
			meta := adapters.TargetHook(target, native).Meta
			var keys []string
			if commandHook(native.Meta) {
				keys = syncedHookKeys(l.cfg, target, native)
			} else {
				keys = append(writtenHookKeys(native.Meta), writtenHookKeys(adapters.ResolveMeta(native.Meta, target))...)
			}
			for _, k := range keys {
				id := hookIdentity(hookEventKey(meta), k)
				index[id] = append(index[id], hookOwnerMatcher{matcher: hookMatcher(meta), name: e.Name})
			}
		}
		l.byTarget[target] = index
	}
	for _, o := range index[hookIdentity(event, key)] {
		if adapters.HookMatcherCovers(target, matcher, o.matcher) {
			return o.name
		}
	}
	return ""
}

func hookIdentity(event, key string) string {
	return event + "\x00" + key
}

// hookEventKey folds case, dashes, and underscores, so `pre-tool-use`,
// `pre_tool_use`, and `PreToolUse` name one event.
func hookEventKey(meta map[string]any) string {
	event, _ := meta["event"].(string)
	return foldHookEvent(event)
}

func foldHookEvent(event string) string {
	return strings.NewReplacer("-", "", "_", "").Replace(strings.ToLower(strings.TrimSpace(event)))
}

func hookMatcher(meta map[string]any) string {
	m, _ := meta["matcher"].(string)
	return m
}

func commandHook(meta map[string]any) bool {
	kind, _ := meta["type"].(string)
	return kind == "" || kind == "command"
}

// writtenHookKeys returns one key per handler a native hook or an
// imported hook doc holds, as written: each command of a command hook
// with its args folded in, or the payload of an http, mcp_tool, or
// prompt hook.
func writtenHookKeys(meta map[string]any) []string {
	str := func(k string) string { s, _ := meta[k].(string); return s }
	switch str("type") {
	case "", "command":
		// Args go with `command`; the handlers under `x-gemini.hooks`
		// stand alone.
		var args []string
		if _, ok := meta["command"]; ok {
			args = stringSliceFromAny(meta["args"])
		}
		var keys []string
		for _, c := range hookCommands(meta) {
			keys = append(keys, hookCommandKey(adapters.ExecFormCommand(c, args)))
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

// syncedHookKeys returns one key per command handler sync writes for h
// on target, as the native file spells it. A target hook run cannot
// render, such as Zed, gets its path rewrite and args fold instead.
func syncedHookKeys(cfg *config.Config, target string, h spec.Entry) []string {
	commands, ok := adapters.SyncedHookCommands(cfg, target, h)
	if !ok {
		meta := adapters.ResolveMeta(h.Meta, target)
		args := stringSliceFromAny(meta["args"])
		for _, c := range hookCommands(meta) {
			commands = append(commands, adapters.ExecFormCommand(adapters.RewriteHookPath(c, target, meta), args))
		}
	}
	keys := make([]string, 0, len(commands))
	for _, c := range commands {
		keys = append(keys, hookCommandKey(c))
	}
	return keys
}

// hookCommandKey is the key of one command handler as the native file
// holds it, after the importer removes what sync adds around it, such as
// the target export.
func hookCommandKey(command string) string {
	return "command\x00" + command
}

// hookCommands returns a command hook's commands, one or a list, or the
// commands of the handler group Gemini keeps under `x-gemini.hooks`.
func hookCommands(meta map[string]any) []string {
	var out []string
	switch c := meta["command"].(type) {
	case string:
		return []string{c}
	case []any:
		for _, v := range c {
			if s, ok := v.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	handlers, _ := meta["hooks"].([]any)
	for _, h := range handlers {
		handler, _ := h.(map[string]any)
		if s, _ := handler["command"].(string); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// feedsOnlyLocalHooks reports whether every handler of one Claude hook
// event, given as its raw matcher groups, comes from a local hook spec.
// Nil-safe.
func (g *localImportGuard) feedsOnlyLocalHooks(event string, raw json.RawMessage) bool {
	if g == nil {
		return false
	}
	var groups []struct {
		Matcher string           `json:"matcher"`
		Hooks   []map[string]any `json:"hooks"`
	}
	if err := json.Unmarshal(raw, &groups); err != nil {
		return false
	}
	handlers := 0
	for _, group := range groups {
		for _, h := range group.Hooks {
			keys := writtenHookKeys(h)
			if len(keys) == 0 {
				return false
			}
			for _, k := range keys {
				if g.hooks.owner("claude", foldHookEvent(event), group.Matcher, k) == "" {
					return false
				}
			}
			handlers++
		}
	}
	return handlers > 0
}

// hookCommandsInEveryView returns the commands a hook spec declares and
// those its `x-<target>` overrides declare.
func hookCommandsInEveryView(meta map[string]any) []string {
	commands := hookCommands(meta)
	for k := range meta {
		if target, ok := strings.CutPrefix(k, "x-"); ok {
			commands = append(commands, hookCommands(adapters.ResolveMeta(meta, target))...)
		}
	}
	return commands
}

// dropsHookCommand reports whether a native command handler comes from
// a local or shared hook spec, and names that spec in the closing note.
// Importers that fold a group's handlers into one spec call it per
// handler first, so the imported spec keeps no field of such a handler.
// Nil-safe.
func (g *localImportGuard) dropsHookCommand(target, event, matcher, command string) bool {
	if g == nil {
		return false
	}
	return g.ownsHookHandler(target, foldHookEvent(event), matcher, hookCommandKey(command))
}

// ownsHookHandler reports whether a local or shared hook spec renders
// one native handler, and names that spec in the closing note. A local
// spec wins: sync renders both layers into the same native file.
func (g *localImportGuard) ownsHookHandler(target, event, matcher, key string) bool {
	if owner := g.hooks.owner(target, event, matcher, key); owner != "" {
		g.skipped[string(spec.KindHook)+" "+owner] = true
		return true
	}
	if owner := g.sharedHooks.owner(target, event, matcher, key); owner != "" {
		g.synced[owner] = true
		return true
	}
	return false
}

// ownsHookDoc reports whether every handler of an imported hook doc
// comes from a local or shared hook spec, and names those specs in the
// closing notes. Importers that write one spec per native handler rely
// on it. The doc counts as written, or as sync writes it: an importer
// may map a handler back to a portable form, such as Copilot's path
// relative to `cwd`.
func (g *localImportGuard) ownsHookDoc(data []byte) bool {
	e, err := spec.ParseYAMLBytes(spec.KindHook, data)
	if err != nil {
		return false
	}
	target := g.source
	if target == "" {
		target, _ = e.Meta["target"].(string)
	}
	native, reason := e.NativeHook(target)
	if reason != "" {
		return false
	}
	meta := adapters.ResolveMeta(native.Meta, target)
	event, matcher := hookEventKey(meta), hookMatcher(meta)
	if event == "" {
		return false
	}
	views := [][]string{writtenHookKeys(meta)}
	if commandHook(meta) {
		views = append(views, syncedHookKeys(g.cfg, target, native))
	}
	for _, keys := range views {
		if len(keys) == 0 || slices.ContainsFunc(keys, func(k string) bool {
			return g.hooks.owner(target, event, matcher, k) == "" && g.sharedHooks.owner(target, event, matcher, k) == ""
		}) {
			continue
		}
		for _, k := range keys {
			g.ownsHookHandler(target, event, matcher, k)
		}
		return true
	}
	return false
}

// leavesHookScript reports whether the hook script named base runs only
// in local hooks: a local hook command names it and no shared hook spec
// does. It names those local hooks in the closing note. Nil-safe.
func (g *localImportGuard) leavesHookScript(base string) bool {
	if g == nil || g.hooksDir == "" {
		return false
	}
	var owners []string
	for _, e := range g.hooks.entries {
		if slices.ContainsFunc(hookCommandsInEveryView(e.Meta), func(c string) bool { return runsHookScript(c, base) }) {
			owners = append(owners, e.Name)
		}
	}
	if len(owners) == 0 || g.sharedHookRuns(base) {
		return false
	}
	for _, o := range owners {
		g.skipped[string(spec.KindHook)+" "+o] = true
	}
	return true
}

// sharedHookRuns reports whether a shared hook spec names the script
// base anywhere in its text. A spec file the run wrote for a local hook
// counts as it was before the run. A spec it cannot read counts as
// naming it, so the script is still captured.
func (g *localImportGuard) sharedHookRuns(base string) bool {
	found := false
	err := filepath.WalkDir(g.hooksDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		if prior, local := g.saved[path]; local {
			if prior == nil {
				return nil
			}
			data, err = prior.data, nil
		}
		if err != nil {
			return err
		}
		if runsHookScript(string(data), base) {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found || (err != nil && !errors.Is(err, fs.ErrNotExist))
}

// runsHookScript reports whether text names `<dir>/hooks/<base>`.
func runsHookScript(text, base string) bool {
	needle := "/hooks/" + base
	for rest := text; ; {
		i := strings.Index(rest, needle)
		if i < 0 {
			return false
		}
		rest = rest[i+len(needle):]
		if rest == "" || !strings.ContainsAny(rest[:1], "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789._-") {
			return true
		}
	}
}
