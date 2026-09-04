// Package trae emits .trae/rules/*.md, .trae/agents/<name>.md,
// .trae/skills/<name>/SKILL.md, .trae/commands/<name>.md, and
// .trae/mcp.json for Trae, ByteDance's AI IDE.
//
// Trae reads project rules from `.trae/rules/*.md` as persistent
// behavioral constraints; it also applies `.trae/rules/` folders found
// in subdirectories, but this adapter targets the project root only.
// `project_rules.md` is the older single-file location and is not
// emitted here. Every rule file carries `description` / `globs` /
// `alwaysApply` YAML frontmatter: docs.trae.ai/ide/rules documents all
// three but never states what activation mode a file with none of them
// gets (#607), so this adapter always emits them rather than leave the
// mode to guesswork. alwaysApply defaults to true; a true rule omits
// globs entirely; alwaysApply:false without an explicit globs falls
// back to the Claude spelling (`paths`, comma-joined). This is the same
// three-field matrix Cursor's `.mdc` files use off the same spec
// metadata (internal/adapters/cursor), ported here rather than shared,
// since the two targets emit different file shapes around it.
//
// Agents emit as native project subagents at `.trae/agents/<name>.md`:
// "Project subagents | ... | `{project_folder}/.trae/agents/{my_agent}.md`"
// (docs.trae.ai/ide/subagents). Frontmatter carries `name` and
// `description` (both required), plus optional `model`, `tools`,
// `disallowedTools`, and `mcpServers`; the body after the closing
// delimiter is the system prompt. This adapter previously flattened
// agents into `.trae/rules/agent-<name>.md`, which reached the rules
// loader instead of the subagent loader and dropped every field rule
// frontmatter has no key for (target-audit 2026-08-27, #638); a stale
// managed copy at the old name is swept for every current agent.
//
// Trae's tool vocabulary is Claude-style and covers agnostic-ai's set
// exactly (Bash, Edit, Glob, Grep, Read, Write, WebFetch, WebSearch,
// plus Skill, LSP, TodoWrite, and `mcp__<server>__<tool>`), so `tools`
// passes through with no translation table, comma-joined into the
// string spelling the vendor documents rather than a YAML list. `model`
// is the opposite case: Trae accepts "Only built-in models provided by
// TraeCode", a table of its own IDs (`gpt-5.4`, `minimax-m3`, ...) with
// zero overlap with a cross-target `model:` value, so a bare generic
// model never reaches the file. Name one for Trae with
// `model: {trae: <id>}` or `x-trae.model`, and the drop surfaces as a
// coverage note either way. `disallowedTools` and `mcpServers` have no
// generic spec field and reach the file through `x-trae`.
//
// Subagents sit behind a toggle: "Go to Settings > Beta > Subagents,
// ensure that the Enable Subagents Directory switch is toggled on."
// The page never states its default, so a project may need that switch
// before the emitted files load. The files are inert until then, which
// is why this ships anyway: the rule-form path it replaces was reaching
// the wrong loader in every case, toggle or not.
//
// Skills emit as one folder per skill under `.trae/skills/<name>/SKILL.md`
// (docs.trae.ai/ide/skills), frontmatter `name` + `description`; sibling
// assets (`examples/`, `templates/`, `resources/`) copy byte-for-byte
// alongside SKILL.md so a skill with bundled files keeps them instead of
// losing them to the flat rule-form Trae never loaded as a skill.
//
// Commands emit as one file per command under `.trae/commands/<name>.md`,
// `name` + `description` frontmatter and the body as the prompt. Trae's
// own docs do not cover the command format; this shape is confirmed from
// real `.trae/commands/*.md` files created through Trae's own chat flow,
// filtered from cross-tool-generated files sharing the same folder
// (identified by Claude Code's `argument-hint` / `$ARGUMENTS`
// convention, which never appears in a native Trae file). Only `name`
// and `description` are confirmed native, so nothing else emits.
// Nesting under `.trae/commands/` up to 3 levels is documented as
// organizational only, with no confirmed functional effect, so this
// adapter writes every command flat.
//
// MCP servers merge into `.trae/mcp.json` (override via
// outputs.trae.mcp-file) under a root `mcpServers` map, keyed by server
// name: docs.trae.ai/ide/add-mcp-servers documents "You can create an
// mcp.json file in the .trae/ directory under the project root and
// declare one or more MCP servers' configurations in it." Stdio carries
// `command` (required) plus optional `args` / `env`; HTTP carries `url`
// (required) plus optional `headers`. Neither transport carries a
// `type` discriminant: the vendor's own examples never key one in, and
// the doc tells stdio and HTTP apart purely by whether `command` or
// `url` is present, unlike claude, cursor, kilo, and qoder, whose
// schemas key an explicit `type` on every remote entry. `disabled` and
// `autoApprove` are equally undocumented (the vendor page covers only a
// project-level MCP toggle under Settings > MCP), so a spec's
// `disabled: true` is stripped with a coverage note rather than written
// dead; see mcp.go for the two vendor quirks (command must not contain
// spaces, and the timeout keys) this adapter deliberately leaves
// unhandled. `docs.trae.ai/ide/mcp`, the URL this repo pointed at
// before, 302s to a marketing page; add-mcp-servers is the live one.
//
// Trae also reads the cross-tool root `AGENTS.md`, which is written
// centrally by `sync`, not by this adapter.
package trae

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
	target             = "trae"
	defaultDir         = ".trae/rules"
	defaultSkillsDir   = ".trae/skills"
	defaultCommandsDir = ".trae/commands"
	defaultMCPFile     = ".trae/mcp.json"
	// defaultAgentsDir is Trae's project subagent path:
	// "`{project_folder}/.trae/agents/{my_agent}.md`"
	// (docs.trae.ai/ide/subagents).
	defaultAgentsDir = ".trae/agents"
	// legacyAgentPrefix is the filename prefix agents carried while they
	// flattened into the rules directory. Emit sweeps a stale managed
	// copy for every current agent name.
	legacyAgentPrefix = "agent-"
)

// commandFrontmatterKeys names the only frontmatter keys confirmed
// native to Trae's command loader. See the package doc for how these
// were confirmed.
var commandFrontmatterKeys = []string{"name", "description"}

var caps = emit.Capabilities{
	Target:   target,
	Supports: []spec.Kind{spec.KindAgent, spec.KindSkill, spec.KindRule, spec.KindCommand, spec.KindMCP},
}

// Adapter emits Trae configs.
type Adapter struct{}

// New returns a Trae adapter.
func New() *Adapter { return &Adapter{} }

// Name returns the target identifier.
func (Adapter) Name() string { return target }

// Emit writes one .md per rule into the rules directory (default
// `.trae/rules`), one .md per agent into the agents directory (default
// `.trae/agents`, Trae's native project-subagent path), one folder per
// skill into the skills directory (default `.trae/skills`), one file
// per command into the commands directory (default `.trae/commands`),
// and the MCP server registry (default `.trae/mcp.json`).
func (Adapter) Emit(sess *emit.Session, b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if err := emit.ReportUnsupported(caps, b, cfg.OnUnsupported); err != nil {
		return err
	}
	rulesDir := emit.OutputRulesDir(cfg, target, defaultDir)
	if err := sess.RulesDirectory(b, emit.RulesDirOpts{
		Dir:        rulesDir,
		SkipAgents: true,
		SkipSkills: true,
		FormatRule: func(e spec.Entry) string { return emit.WithHeader(ruleForm(e), emit.FormatMarkdown) },
	}, dryRun); err != nil {
		return err
	}
	agentsDir := emit.OutputAgentsDir(cfg, target, defaultAgentsDir)
	if err := emitAgents(sess, b.Agents, agentsDir, rulesDir, dryRun); err != nil {
		return err
	}
	skillsDir := emit.OutputSkillsDir(cfg, target, defaultSkillsDir)
	if err := sess.WriteSkillFolders(b.Skills, target, skillsDir, dryRun); err != nil {
		return err
	}
	commandsDir := emit.OutputCommandsDir(cfg, target, defaultCommandsDir)
	if err := emitCommands(sess, b.Commands, commandsDir, dryRun); err != nil {
		return err
	}
	return emitMCP(sess, b.MCPs, emit.OutputMCPFile(cfg, target, defaultMCPFile), dryRun)
}

// emitCommands writes one `<dir>/<name>.md` per command spec: `name` +
// `description` frontmatter, then the body as the prompt.
func emitCommands(sess *emit.Session, commands []spec.Entry, dir string, dryRun bool) error {
	for _, c := range commands {
		path := filepath.Join(dir, c.Name+".md")
		body := emit.WithHeader(commandFile(c), emit.FormatMarkdown)
		if err := sess.WriteFile(path, body, dryRun); err != nil {
			return err
		}
	}
	return nil
}

// commandFile renders a single command markdown file: `name` +
// `description` (description falls back to the command's name when the
// spec has none), plus any x-trae custom keys the author explicitly
// declares on their own spec (an opt-in escape hatch, not an inferred
// vendor key), then the body.
func commandFile(e spec.Entry) string {
	resolved := emit.ResolveMeta(e.Meta, target)
	desc, _ := resolved["description"].(string)
	if desc == "" {
		desc = e.Name
	}
	front := map[string]any{
		"name":        e.Name,
		"description": desc,
	}
	keys := append([]string{}, commandFrontmatterKeys...)
	emit.MergeCustomTargetMeta(front, &keys, e.Meta, target, commandFrontmatterKeys...)
	var sb strings.Builder
	sb.WriteString(emit.FrontmatterOrdered(front, keys))
	sb.WriteString("\n")
	sb.WriteString(e.Body)
	return sb.String()
}

// ruleForm renders one rule as a `.md` file: the
// description/globs/alwaysApply activation frontmatter (see the
// package doc), then a `# <name>` heading and the body.
func ruleForm(e spec.Entry) string {
	var b strings.Builder
	b.WriteString(activationFrontmatter(emit.ResolveMeta(e.Meta, target)))
	b.WriteString("# " + e.Name + "\n\n")
	b.WriteString(e.Body)
	return b.String()
}

// activationFrontmatter renders the description/globs/alwaysApply block
// docs.trae.ai/ide/rules documents, ported from Cursor's identical
// three-field matrix (internal/adapters/cursor's mdc, off the same spec
// metadata): alwaysApply defaults to true; an alwaysApply:true rule
// ignores globs entirely, so none is synthesized; alwaysApply:false
// without an explicit globs falls back to the Claude spelling (`paths`,
// a scalar or list, comma-joined). An empty description still emits a
// bare `description:` key so every file carries all three keys
// regardless of what the spec sets, per the package doc.
func activationFrontmatter(m map[string]any) string {
	desc, _ := m["description"].(string)
	globs, _ := m["globs"].(string)
	always := true
	if v, ok := m["alwaysApply"].(bool); ok {
		always = v
	}
	if globs == "" && !always {
		globs = strings.Join(pathsToGlobs(m["paths"]), ",")
	}
	var b strings.Builder
	b.WriteString("---\n")
	if desc != "" {
		b.WriteString("description: " + desc + "\n")
	} else {
		b.WriteString("description:\n")
	}
	if globs != "" {
		b.WriteString("globs: " + yamlScalar(globs) + "\n")
	}
	fmt.Fprintf(&b, "alwaysApply: %t\n", always)
	b.WriteString("---\n\n")
	return b.String()
}

// pathsToGlobs normalizes a `paths` value (the Claude spelling: a
// scalar string or a list) into a slice of glob strings. Returns nil
// when the key is absent or carries no usable value. Mirrors cursor's
// helper of the same name; kept as its own copy per the
// no-cross-adapter-imports rule rather than shared, since both are
// small and target-specific.
func pathsToGlobs(paths any) []string {
	switch v := paths.(type) {
	case string:
		if v == "" {
			return nil
		}
		return []string{v}
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

// yamlScalar renders s as a YAML scalar with minimal quoting: a plain
// value like `apps/foo/**` stays unquoted, while a value YAML cannot
// represent plainly (a leading `*`, a colon, ...) is quoted just enough
// to stay valid.
func yamlScalar(s string) string {
	out, err := yaml.Marshal(s)
	if err != nil {
		return fmt.Sprintf("%q", s)
	}
	return strings.TrimRight(string(out), "\n")
}
