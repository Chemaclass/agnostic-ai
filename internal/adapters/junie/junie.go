// Package junie emits configs for JetBrains Junie.
//
// Junie reads every Markdown file under `.junie/rules/` and concatenates
// them automatically; there is no single entry file to assemble. Rules
// and agents flatten into that one directory (agents as
// `agent-<name>.md`), matching the cline adapter's shape.
// `.junie/guidelines.md` is the legacy single-file location and is not
// emitted here.
//
// Skills emit into their own native folder tree at
// `.junie/skills/<name>/SKILL.md` (override via outputs.junie.skills-dir),
// not the flattened rules directory: Junie's Native Agent Skills feature
// shipped 2026-07-31 and requires exactly this layout ("Project scope:
// `<projectRoot>/.junie/skills/<skill-name>/`"; "The `SKILL.md` file is
// required. A folder without it is not recognized as a skill",
// junie.jetbrains.com/docs/agent-skills.html, target-audit 2026-08-01).
// A flat `.junie/rules/skill-<name>.md` file never reaches that path and
// drops any bundled asset sitting next to the skill's source SKILL.md;
// the folder writer propagates those siblings byte-for-byte.
//
// The entry-point lookup order is a custom path, then
// `.junie/AGENTS.md` ("the most preferred standard location"), then the
// root `AGENTS.md` "if no file is found in the `.junie` folder"
// (junie.jetbrains.com/docs/junie-ide-plugin.html, target-audit
// 2026-08-01). The root file is written centrally by `sync` as a slim
// pointer to the source specs (one body shared with every other
// target's entry-point file); this adapter never writes it. Whether an
// existing `.junie/rules/*.md` already counts as "a file found in the
// `.junie` folder" (which would make the root file unreachable once
// this adapter has run at all) is unresolved upstream, so this adapter
// mirrors the same pointer body to `.junie/AGENTS.md` too: correct
// under either reading, since Junie then finds the canonical body
// however it resolves the folder. The mirrored body is read from
// `.agnostic-ai/AGNOSTIC_AI.md` when that file already exists (so a
// hand-edited body propagates here exactly like the root file), and
// falls back to the generated template otherwise; this adapter never
// creates AGNOSTIC_AI.md itself, `sync`'s central write owns that
// bootstrap.
//
// MCP servers write to `.junie/mcp/mcp.json` using the standard
// `mcpServers` map schema (the same shape Claude Code and Cursor use).
package junie

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	target           = "junie"
	defaultDir       = ".junie/rules"
	defaultSkillsDir = ".junie/skills"
	defaultMCPFile   = ".junie/mcp/mcp.json"
	// defaultEntryFile is Junie's own preferred entry-point location,
	// checked before it falls back to the root AGENTS.md (see the
	// package doc). Fixed, not user-overridable: it exists purely to
	// make the canonical pointer body reachable regardless of how the
	// `.junie` folder ambiguity resolves.
	defaultEntryFile = ".junie/AGENTS.md"
)

var caps = emit.Capabilities{
	Target:   target,
	Supports: []spec.Kind{spec.KindAgent, spec.KindSkill, spec.KindRule, spec.KindMCP},
}

// Adapter emits Junie configs.
type Adapter struct{}

// New returns a Junie adapter.
func New() *Adapter { return &Adapter{} }

// Name returns the target identifier.
func (Adapter) Name() string { return target }

// Emit writes one .md per rule and per agent into the rules directory,
// one folder per skill under the skills directory (Junie's native
// SKILL.md layout; a flat file there never loads as a skill), the
// `.junie/AGENTS.md` entry-point mirror, then the MCP server file when
// the bundle has any MCP entries.
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
	if err := emitEntryPoint(sess, cfg, dryRun); err != nil {
		return err
	}
	return sess.WriteMCPFile(b.MCPs, emit.MCPSchemaServersMap,
		emit.OutputMCPFile(cfg, target, defaultMCPFile), dryRun)
}

// emitEntryPoint mirrors the canonical pointer body to .junie/AGENTS.md,
// Junie's preferred entry-point location (see the package doc). Uses
// entryPointBody so a hand-edited AGNOSTIC_AI.md propagates here
// identically to the root AGENTS.md sync writes centrally.
func emitEntryPoint(sess *emit.Session, cfg *config.Config, dryRun bool) error {
	body, err := entryPointBody(cfg)
	if err != nil {
		return err
	}
	return sess.WriteFile(defaultEntryFile, emit.WithHeader(body, emit.FormatMarkdown), dryRun)
}

// entryPointBody returns the canonical pointer body: the content of
// .agnostic-ai/AGNOSTIC_AI.md (header stripped) when that file already
// exists, or the generated template otherwise. Mirrors the read side
// of sync's own central entry-point resolution without creating
// AGNOSTIC_AI.md itself; that bootstrap stays sync's responsibility so
// this adapter never races it for the first write.
func entryPointBody(cfg *config.Config) (string, error) {
	data, err := os.ReadFile(emit.AgnosticEntryPointPath)
	if err == nil {
		return emit.StripHeader(string(data)), nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("%s: %w", emit.AgnosticEntryPointPath, err)
	}
	return emit.EntryPointBody(cfg), nil
}
