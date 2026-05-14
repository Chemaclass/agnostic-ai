// Package gemini emits configs for Gemini CLI.
//
// Gemini CLI natively supports hierarchical `GEMINI.md` files (each
// subdirectory adds context for that subtree) and reads slash commands
// from `.gemini/commands/<name>.toml`. This adapter routes rules by
// their source-layout scope (or `globs` frontmatter prefix) into one
// `GEMINI.md` per scope, and emits one TOML per agent under
// `.gemini/commands/`. Skills are reference-only in the root
// `GEMINI.md` by default; setting `outputs.gemini.emit-skills-as-commands: true`
// also writes a TOML per skill with a `skill-` filename prefix.
package gemini

import (
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	target              = "gemini"
	defaultRootFile     = "GEMINI.md"
	defaultCommandsDir  = ".gemini/commands"
	defaultSettingsFile = ".gemini/settings.json"
	skillFilenamePrefix = "skill-"
)

var caps = emit.Capabilities{
	Target:   target,
	Supports: []spec.Kind{spec.KindAgent, spec.KindSkill, spec.KindRule, spec.KindHook, spec.KindMCP},
}

// Adapter emits Gemini CLI configs.
type Adapter struct{}

// New returns a Gemini adapter.
func New() *Adapter { return &Adapter{} }

// Name returns the target identifier.
func (Adapter) Name() string { return target }

// Emit writes the per-scope `GEMINI.md` files plus one TOML per agent
// (and per skill when opted in) under `.gemini/commands/`.
func (Adapter) Emit(b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if err := emit.ReportUnsupported(caps, b, cfg.OnUnsupported); err != nil {
		return err
	}

	commandsDir := emit.OutputCommandsDir(cfg, target, defaultCommandsDir)

	if err := emitAgentCommands(b.Agents, commandsDir, dryRun); err != nil {
		return err
	}
	if emit.EmitSkillsAsCommands(cfg, target) {
		if err := emitSkillCommands(b.Skills, commandsDir, dryRun); err != nil {
			return err
		}
	}
	if err := emitGEMINITree(b, cfg, commandsDir, dryRun); err != nil {
		return err
	}
	return emitSettings(b, emit.OutputMCPFile(cfg, target, defaultSettingsFile), dryRun)
}

// emitSettings writes (or merges into) .gemini/settings.json with the
// `mcpServers` and `hooks` keys. Routes through emit.MergeJSONFile so
// any user-managed Gemini settings survive the sync.
func emitSettings(b spec.Bundle, path string, dryRun bool) error {
	keys := map[string]any{}
	if servers := buildMCPServers(b.MCPs); len(servers) > 0 {
		keys["mcpServers"] = servers
	}
	if hooks := buildHooks(b.Hooks); len(hooks) > 0 {
		keys["hooks"] = hooks
	}
	if len(keys) == 0 {
		return nil
	}
	return emit.MergeJSONFile(path, keys, dryRun)
}

// buildMCPServers renders Gemini-shaped MCP servers. Stdio specs emit
// {command, args, env}; HTTP / SSE specs emit {httpUrl, headers} -
// Gemini uses `httpUrl`, not the standard `url`.
func buildMCPServers(mcps []spec.Entry) map[string]any {
	out := map[string]any{}
	for _, e := range mcps {
		if e.Name == "" {
			continue
		}
		entry := buildMCPServer(e)
		if len(entry) == 0 {
			continue
		}
		out[e.Name] = entry
	}
	return out
}

func buildMCPServer(e spec.Entry) map[string]any {
	transport, _ := e.Meta["type"].(string)
	if transport == "" {
		transport = "stdio"
	}
	out := map[string]any{}
	switch transport {
	case "stdio":
		if cmd, _ := e.Meta["command"].(string); cmd != "" {
			out["command"] = cmd
		}
		if args := emit.StringSlice(e.Meta["args"]); len(args) > 0 {
			out["args"] = args
		}
	case "http", "sse":
		if url, _ := e.Meta["url"].(string); url != "" {
			out["httpUrl"] = url
		}
		if h := emit.StringMap(e.Meta["headers"]); len(h) > 0 {
			out["headers"] = h
		}
	}
	if env := emit.StringMap(e.Meta["env"]); len(env) > 0 {
		out["env"] = env
	}
	return out
}

// buildHooks groups hook specs by their `event` frontmatter into the
// Gemini hooks shape: `hooks.<event> = [{matcher, command}, ...]`.
// `matcher` is omitted when absent so the hook fires unconditionally.
// A spec's `command:` field accepts a string or a list of strings; each
// list entry becomes one `{matcher, command}` pair under the same event.
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
			entry := map[string]any{"command": cmd}
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

func emitAgentCommands(agents []spec.Entry, dir string, dryRun bool) error {
	for _, a := range agents {
		path := filepath.Join(dir, a.Name+".toml")
		if err := emit.WriteFile(path, commandTOML(a), dryRun); err != nil {
			return err
		}
	}
	return nil
}

func emitSkillCommands(skills []spec.Entry, dir string, dryRun bool) error {
	for _, s := range skills {
		path := filepath.Join(dir, skillFilenamePrefix+s.Name+".toml")
		if err := emit.WriteFile(path, commandTOML(s), dryRun); err != nil {
			return err
		}
	}
	return nil
}

// emitGEMINITree writes one GEMINI.md per scope. The root file lists
// agents and skills as references; scoped files contain only their
// own rules.
func emitGEMINITree(b spec.Bundle, cfg *config.Config, commandsDir string, dryRun bool) error {
	if len(b.Rules) == 0 && len(b.Agents) == 0 && len(b.Skills) == 0 {
		return nil
	}

	rootFile := emit.OutputFile(cfg, target, defaultRootFile)
	rootDir := filepath.Dir(rootFile)
	rootBase := filepath.Base(rootFile)

	byScope := emit.GroupRulesByScope(b.Rules)
	scopes := slices.Sorted(maps.Keys(byScope))
	if !slices.Contains(scopes, "") && (len(b.Agents) > 0 || len(b.Skills) > 0) {
		scopes = append([]string{""}, scopes...)
	}

	for _, scope := range scopes {
		var sb strings.Builder
		writeHeader(&sb, scope)
		writeRulesSection(&sb, byScope[scope])
		if scope == "" {
			writeAgentsSection(&sb, b.Agents, commandsDir)
			writeSkillsSection(&sb, b.Skills, commandsDir, emit.EmitSkillsAsCommands(cfg, target))
		}
		path := filepath.Join(rootDir, scope, rootBase)
		if err := emit.WriteFile(path, sb.String(), dryRun); err != nil {
			return err
		}
	}
	return nil
}

// commandTOML renders one slash-command TOML. Schema:
//
//	description = "<spec description>"
//	prompt = """
//	<spec body>
//	"""
func commandTOML(e spec.Entry) string {
	var sb strings.Builder
	if d := e.Description(); d != "" {
		emit.WriteTOMLString(&sb, "description", d)
	}
	body := strings.TrimSpace(e.Body)
	if body == "" {
		body = e.Description()
	}
	emit.WriteTOMLMultiline(&sb, "prompt", body)
	return sb.String()
}

func writeHeader(sb *strings.Builder, scope string) {
	if scope == "" {
		sb.WriteString("# GEMINI.md\n\n")
	} else {
		sb.WriteString("# GEMINI.md (" + scope + ")\n\n")
		sb.WriteString("Scoped to `" + scope + "/**`. Inherits root rules.\n\n")
	}
	sb.WriteString("Generated by agnostic-ai. Do not edit by hand.\n\n")
}

func writeRulesSection(sb *strings.Builder, rules []spec.Entry) {
	if len(rules) == 0 {
		return
	}
	sb.WriteString("## Rules\n\n")
	for _, r := range rules {
		emit.WriteSection(sb, r.Name, r)
	}
}

// writeAgentsSection lists agents in the root GEMINI.md without
// duplicating their bodies. The real definitions live in
// `<commandsDir>/<name>.toml`.
func writeAgentsSection(sb *strings.Builder, agents []spec.Entry, commandsDir string) {
	if len(agents) == 0 {
		return
	}
	sb.WriteString("## Agents\n\n")
	sb.WriteString("Custom Gemini CLI slash commands. Definitions live in `" + commandsDir + "/`.\n\n")
	for _, a := range agents {
		emit.WriteReference(sb, a, filepath.Join(commandsDir, a.Name+".toml"))
	}
}

// writeSkillsSection lists skills in the root GEMINI.md. Source pointer
// is the skill spec file by default, or the generated command TOML when
// emit-skills-as-commands is enabled.
func writeSkillsSection(sb *strings.Builder, skills []spec.Entry, commandsDir string, asCommands bool) {
	if len(skills) == 0 {
		return
	}
	sb.WriteString("## Skills\n\n")
	if asCommands {
		sb.WriteString("Gemini CLI slash commands. Definitions live in `" + commandsDir + "/`.\n\n")
	} else {
		sb.WriteString("Reference material. Read the source file to use a skill.\n\n")
	}
	for _, s := range skills {
		var sourcePath string
		if asCommands {
			sourcePath = filepath.Join(commandsDir, skillFilenamePrefix+s.Name+".toml")
		} else {
			sourcePath = s.Path
		}
		emit.WriteReference(sb, s, sourcePath)
	}
}
