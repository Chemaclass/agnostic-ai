// Package cursor emits Cursor editor configs.
//
// Rules emit as .cursor/rules/*.mdc with alwaysApply=true; agents emit
// as rules with alwaysApply=false. Both honor a frontmatter override.
// Skills emit natively as one folder per skill under
// .cursor/skills/<name>/SKILL.md (the Agent Skills layout Cursor 2.4+
// discovers), including bundled asset files. Commands emit to
// .cursor/commands/<name>.md, Cursor's standard project commands
// location. Hooks land in .cursor/hooks.json, MCP servers in
// .cursor/mcp.json, Bugbot review guidance in BUGBOT.md, background
// agent bootstrap in .cursor/environment.json, and ignore lists in
// .cursorignore.
package cursor

import (
	"fmt"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	target             = "cursor"
	defaultDir         = ".cursor/rules"
	defaultExt         = ".mdc"
	defaultSkillsDir   = ".cursor/skills"
	defaultCommandsDir = ".cursor/commands"
	defaultMCPFile     = ".cursor/mcp.json"
	defaultReviewFile  = "BUGBOT.md"
	defaultEnvironFile = ".cursor/environment.json"
	defaultIgnoreFile  = ".cursorignore"
	defaultHooksFile   = ".cursor/hooks.json"
)

var caps = emit.Capabilities{
	Target:   target,
	Supports: []spec.Kind{spec.KindAgent, spec.KindSkill, spec.KindRule, spec.KindHook, spec.KindMCP, spec.KindCommand, spec.KindReview, spec.KindEnvironment, spec.KindIgnore},
}

// environRoutingKeys are the agnostic-ai spec fields stripped after
// emit.ResolveMeta before the remaining keys pass through to
// `.cursor/environment.json`. ResolveMeta already removes the target
// allow/deny keys and the x-<target> namespace; these three are the spec
// identity/scoping fields it leaves in place that Cursor has no use for.
var environRoutingKeys = map[string]struct{}{
	"name": {}, "scope": {}, "description": {},
}

// Adapter emits Cursor configs.
type Adapter struct{}

// New returns a Cursor adapter.
func New() *Adapter { return &Adapter{} }

// Name returns the target identifier.
func (Adapter) Name() string { return target }

// Emit writes one .mdc per rule and agent, one native skill folder per
// skill under `.cursor/skills/`, one command per spec under
// `.cursor/commands/`, plus an `.cursor/mcp.json` when MCP entries
// exist. Agents also emit as Custom Commands at the commands dir so
// they stay invocable by name.
func (Adapter) Emit(b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if err := emit.ReportUnsupported(caps, b, cfg.OnUnsupported); err != nil {
		return err
	}
	if err := emit.RulesDirectory(b, emit.RulesDirOpts{
		Dir: emit.OutputRulesDir(cfg, target, defaultDir),
		Ext: defaultExt,
		// Skills emit natively under .cursor/skills/; a flattened
		// skill-<name>.mdc copy would double-expose each skill.
		SkipSkills:  true,
		FormatRule:  func(e spec.Entry) string { return emit.WithHeader(mdc(e, true), emit.FormatMarkdown) },
		FormatAgent: func(e spec.Entry) string { return emit.WithHeader(mdc(e, false), emit.FormatMarkdown) },
	}, dryRun); err != nil {
		return err
	}
	skillsDir := emit.OutputSkillsDir(cfg, target, defaultSkillsDir)
	for _, s := range b.Skills {
		if err := emitSkill(s, skillsDir, dryRun); err != nil {
			return err
		}
	}
	if err := emitReviews(b, cfg, dryRun); err != nil {
		return err
	}
	if err := emitEnvironment(b, cfg, dryRun); err != nil {
		return err
	}
	if err := emit.WriteIgnoreFile(b.Ignores, emit.OutputIgnoreFile(cfg, target, defaultIgnoreFile), dryRun); err != nil {
		return err
	}
	commandsDir := emit.OutputCommandsDir(cfg, target, defaultCommandsDir)
	for _, a := range b.Agents {
		path := commandsDir + "/" + a.Name + ".md"
		if err := emit.WriteFile(path, emit.WithHeader(command(a), emit.FormatMarkdown), dryRun); err != nil {
			return err
		}
	}
	for _, c := range b.Commands {
		path := commandsDir + "/" + c.Name + ".md"
		if err := emit.WriteFile(path, emit.WithHeader(command(c), emit.FormatMarkdown), dryRun); err != nil {
			return err
		}
	}
	if err := emitHooks(b.HooksFor(target), cfg, dryRun); err != nil {
		return err
	}
	return emit.WriteMCPFile(b.MCPs, emit.MCPSchemaServersMap, emit.OutputMCPFile(cfg, target, defaultMCPFile), dryRun)
}

// hooksDoc is the `.cursor/hooks.json` shape per the Cursor hooks docs:
// a numeric `version` plus a `hooks` map keyed by lifecycle event name,
// each event holding an array of `{command, matcher?}` entries.
type hooksDoc struct {
	Version int            `json:"version"`
	Hooks   map[string]any `json:"hooks"`
}

// emitHooks writes the managed `.cursor/hooks.json` from the hook specs
// scoped to cursor. The file is overwritten each sync; a no-op when no
// hooks resolve to output. The path is overridable via
// `outputs.cursor.hooks-file`. Hook scripts stashed under
// `.agnostic-ai/scripts/` materialize into `.cursor/hooks/` so the
// emitted `command:` paths resolve.
func emitHooks(hooks []spec.Entry, cfg *config.Config, dryRun bool) error {
	byEvent := buildHooks(hooks)
	if len(byEvent) == 0 {
		return nil
	}
	raw, err := emit.MarshalJSONIndent(hooksDoc{Version: 1, Hooks: byEvent})
	if err != nil {
		return fmt.Errorf("cursor hooks: %w", err)
	}
	path := emit.OutputHooksFile(cfg, target, defaultHooksFile)
	if err := emit.WriteFile(path, string(raw)+"\n", dryRun); err != nil {
		return err
	}
	return materializeHookScripts(hooks, dryRun)
}

// buildHooks groups hook specs by their `event` frontmatter into Cursor's
// `hooks.<event> = [{command, matcher?}, ...]` shape. Cursor passes the
// event name through verbatim (no cross-tool translation), so a cursor
// hook spec sets `event:` to a Cursor lifecycle name (e.g.
// `beforeShellExecution`, `afterFileEdit`). `matcher` is omitted when
// absent. A `command:` list yields one entry per element.
func buildHooks(hooks []spec.Entry) map[string]any {
	byEvent := map[string][]map[string]any{}
	for _, h := range hooks {
		event, _ := h.Meta["event"].(string)
		if event == "" {
			continue
		}
		matcher, _ := h.Meta["matcher"].(string)
		cmds := hookCommands(h.Meta["command"])
		if len(cmds) == 0 {
			continue
		}
		for _, cmd := range cmds {
			entry := map[string]any{"command": emit.RewriteHookPath(cmd, target)}
			if matcher != "" {
				entry["matcher"] = matcher
			}
			byEvent[event] = append(byEvent[event], entry)
		}
	}
	out := map[string]any{}
	for k, v := range byEvent {
		out[k] = v
	}
	return out
}

// materializeHookScripts copies each hook's stashed script body from
// `.agnostic-ai/scripts/` into `.cursor/hooks/` so the emitted
// hooks.json references a script that exists. Hooks whose command is a
// free-form shell expression carry no stashed body and skip silently.
func materializeHookScripts(hooks []spec.Entry, dryRun bool) error {
	for _, h := range hooks {
		for _, raw := range hookCommands(h.Meta["command"]) {
			sourceTool, _ := emit.SourceToolFromHookCommand(raw)
			rewritten := emit.RewriteHookPath(raw, target)
			if err := emit.MaterializeHookScript(rewritten, target, sourceTool, dryRun); err != nil {
				return err
			}
		}
	}
	return nil
}

// hookCommands normalizes a `command:` field that may be a string or a
// list of strings into a single []string. Empty strings drop out.
func hookCommands(raw any) []string {
	switch v := raw.(type) {
	case string:
		if v == "" {
			return nil
		}
		return []string{v}
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	case []string:
		out := make([]string, 0, len(v))
		for _, s := range v {
			if s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

// command renders a Cursor Custom Command file. The Cursor docs
// describe these as Markdown with optional frontmatter (`description`,
// `model`); the body is the prompt the IDE sends when the user invokes
// the command.
func command(e spec.Entry) string {
	m := emit.ResolveMeta(e.Meta, target)
	desc, _ := m["description"].(string)
	model, _ := m["model"].(string)
	var b strings.Builder
	b.WriteString("---\n")
	if desc != "" {
		b.WriteString("description: " + desc + "\n")
	}
	if model != "" {
		b.WriteString("model: " + model + "\n")
	}
	b.WriteString("---\n\n")
	b.WriteString(e.Body)
	return b.String()
}

// emitReviews writes Cursor Bugbot review guidance as a `BUGBOT.md` per
// scope. Cursor reads a root `BUGBOT.md` plus optional per-directory files,
// so review specs honor `EffectiveScope` the same way rules do: an unscoped
// spec lands at the repo root, a spec under `reviews/backend/` lands at
// `backend/BUGBOT.md`. Specs sharing a scope concatenate into that scope's
// single file. The basename is overridable via `outputs.cursor.review-file`.
func emitReviews(b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if len(b.Reviews) == 0 {
		return nil
	}
	base := emit.OutputReviewFile(cfg, target, defaultReviewFile)
	byScope := map[string][]spec.Entry{}
	var scopeOrder []string
	for _, r := range b.Reviews {
		scope := r.EffectiveScope()
		if _, ok := byScope[scope]; !ok {
			scopeOrder = append(scopeOrder, scope)
		}
		byScope[scope] = append(byScope[scope], r)
	}
	for _, scope := range scopeOrder {
		if scopeEscapesRoot(scope) {
			// A frontmatter `scope: ../x` would anchor BUGBOT.md outside the
			// repo (review files sit at the project root, not under a tool
			// dir). Skip it rather than write beyond the project.
			continue
		}
		var sb strings.Builder
		for i, r := range byScope[scope] {
			if i > 0 {
				sb.WriteString("\n\n")
			}
			sb.WriteString(strings.TrimRight(r.Body, "\n"))
		}
		path := filepath.Join(scope, base)
		if err := emit.WriteFile(path, emit.WithHeader(sb.String()+"\n", emit.FormatMarkdown), dryRun); err != nil {
			return err
		}
	}
	return nil
}

// emitEnvironment writes Cursor's background-agent bootstrap config to
// `.cursor/environment.json`. The spec body is the environment.json content:
// every key except the agnostic-ai routing fields passes through verbatim,
// so the author controls Cursor's schema (install, terminals, ...) while
// agnostic-ai single-sources it. Multiple environment specs merge by
// top-level key, last spec winning. The path is overridable via
// `outputs.cursor.environment-file`.
func emitEnvironment(b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if len(b.Environments) == 0 {
		return nil
	}
	merged := map[string]any{}
	for _, e := range b.Environments {
		// Resolve the x-<target> namespace first so a cursor-specific
		// override wins and other targets' blocks are dropped, exactly like
		// every other adapter. Then strip the spec identity fields Cursor
		// has no schema for.
		for k, v := range emit.ResolveMeta(e.Meta, target) {
			if _, skip := environRoutingKeys[k]; skip {
				continue
			}
			merged[k] = v
		}
	}
	if len(merged) == 0 {
		return nil
	}
	raw, err := emit.MarshalJSONIndent(merged)
	if err != nil {
		return fmt.Errorf("cursor environment: %w", err)
	}
	path := emit.OutputEnvironmentFile(cfg, target, defaultEnvironFile)
	return emit.WriteFile(path, string(raw)+"\n", dryRun)
}

// scopeEscapesRoot reports whether a cleaned scope points at or above the
// repo root (a leading `..`), which would let an emitted file land outside
// the project tree.
func scopeEscapesRoot(scope string) bool {
	clean := filepath.ToSlash(filepath.Clean(scope))
	return clean == ".." || strings.HasPrefix(clean, "../")
}

func mdc(e spec.Entry, alwaysApplyDefault bool) string {
	m := emit.ResolveMeta(e.Meta, target)
	desc, _ := m["description"].(string)
	globs, _ := m["globs"].(string)
	always := alwaysApplyDefault
	if v, ok := m["alwaysApply"].(bool); ok {
		always = v
	}
	// An alwaysApply:false rule (agents, skills) defaults to a broad
	// `**/*` auto-attach when it declares no globs. An alwaysApply:true
	// rule ignores globs entirely, so synthesizing one there is pure
	// round-trip noise against a hand-authored source; omit it (#443).
	if globs == "" && !always {
		globs = "**/*"
	}
	var b strings.Builder
	b.WriteString("---\n")
	// Emit an empty description as a bare `description:` key (no trailing
	// space) so it byte-matches a hand-authored rule on round-trip (#443).
	if desc != "" {
		b.WriteString("description: " + desc + "\n")
	} else {
		b.WriteString("description:\n")
	}
	// Quote the globs value only when YAML needs it (a leading `*`, etc.)
	// rather than always double-quoting, which rewrote `apps/foo/**` to
	// `"apps/foo/**"` and broke byte round-trips (#443).
	if globs != "" {
		b.WriteString("globs: " + mdcScalar(globs) + "\n")
	}
	fmt.Fprintf(&b, "alwaysApply: %t\n", always)
	b.WriteString("---\n\n")
	b.WriteString(e.Body)
	return b.String()
}

// mdcScalar renders s as a YAML scalar with minimal quoting: a plain
// value like `apps/foo/**` stays unquoted, while a value YAML cannot
// represent plainly (a leading `*`, a colon, ...) is quoted just enough
// to stay valid.
func mdcScalar(s string) string {
	out, err := yaml.Marshal(s)
	if err != nil {
		return fmt.Sprintf("%q", s)
	}
	return strings.TrimRight(string(out), "\n")
}
