// Package trae emits .trae/rules/*.md, .trae/skills/<name>/SKILL.md,
// .trae/commands/<name>.md, and .trae/mcp.json for Trae, ByteDance's AI
// IDE.
//
// Trae reads project rules from `.trae/rules/*.md` as persistent
// behavioral constraints; it also applies `.trae/rules/` folders found
// in subdirectories, but this adapter targets the project root only.
// `project_rules.md` is the older single-file location and is not
// emitted here. Trae's custom-agent surface has no documented file
// format yet, so agents flatten to rule-form files alongside plain
// rules (the same approach cline uses).
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
	"path/filepath"
	"strings"

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

// Emit writes one .md per rule and agent into the rules directory
// (default `.trae/rules`), one folder per skill into the skills
// directory (default `.trae/skills`), one file per command into the
// commands directory (default `.trae/commands`), and the MCP server
// registry (default `.trae/mcp.json`).
func (Adapter) Emit(sess *emit.Session, b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if err := emit.ReportUnsupported(caps, b, cfg.OnUnsupported); err != nil {
		return err
	}
	if err := sess.RulesDirectory(b, emit.RulesDirOpts{
		Dir:         emit.OutputRulesDir(cfg, target, defaultDir),
		AgentPrefix: "agent-",
		SkipSkills:  true,
	}, dryRun); err != nil {
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
