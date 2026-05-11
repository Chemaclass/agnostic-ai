// Package amp emits configs for Sourcegraph Amp.
//
// Amp's owner's manual specifies `AGENTS.md` (plural) at the project
// root plus hierarchical subtree AGENTS.md files that scope to their
// directory. Custom slash commands live under `.agents/commands/<name>.md`.
//
// Previous releases of this adapter wrote `AGENT.md` (singular) which
// Amp no longer reads. The first sync after upgrading detects an old
// agnostic-generated `AGENT.md` and renames it to `AGENT.md.bak` so
// users can verify the new layout before deleting the backup.
package amp

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	target              = "amp"
	defaultOutFile      = "AGENTS.md"
	defaultCommandsDir  = ".agents/commands"
	defaultMCPFile      = ".amp/settings.json"
	skillFilenamePrefix = "skill-"
	legacyOutFile       = "AGENT.md"
	ampMCPKey           = "amp.mcpServers"
)

var caps = emit.Capabilities{
	Target:   target,
	Supports: []spec.Kind{spec.KindAgent, spec.KindSkill, spec.KindRule, spec.KindMCP},
}

// Adapter emits Amp configs.
type Adapter struct{}

// New returns an Amp adapter.
func New() *Adapter { return &Adapter{} }

// Name returns the target identifier.
func (Adapter) Name() string { return target }

// Emit writes hierarchical AGENTS.md files plus one command file per
// agent (and per skill when opted in), and migrates any legacy
// agnostic-generated AGENT.md to AGENT.md.bak.
func (Adapter) Emit(b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if err := emit.ReportUnsupported(caps, b, cfg.OnUnsupported); err != nil {
		return err
	}

	emit.MigrateLegacyFile(cfg, target, legacyOutFile, defaultOutFile, dryRun)

	commandsDir := emit.OutputCommandsDir(cfg, target, defaultCommandsDir)
	if err := emitAgentCommands(b.Agents, commandsDir, dryRun); err != nil {
		return err
	}
	if emit.EmitSkillsAsCommands(cfg, target) {
		if err := emitSkillCommands(b.Skills, commandsDir, dryRun); err != nil {
			return err
		}
	}
	if err := emitAgentsTree(b, cfg, commandsDir, dryRun); err != nil {
		return err
	}
	return emitMCPSettings(b.MCPs, emit.OutputMCPFile(cfg, target, defaultMCPFile), dryRun)
}

func emitAgentCommands(agents []spec.Entry, dir string, dryRun bool) error {
	for _, a := range agents {
		if err := emit.WriteFile(filepath.Join(dir, a.Name+".md"), commandFile(a), dryRun); err != nil {
			return err
		}
	}
	return nil
}

func emitSkillCommands(skills []spec.Entry, dir string, dryRun bool) error {
	for _, s := range skills {
		if err := emit.WriteFile(filepath.Join(dir, skillFilenamePrefix+s.Name+".md"), commandFile(s), dryRun); err != nil {
			return err
		}
	}
	return nil
}

// emitAgentsTree writes one AGENTS.md per scope. The root document
// indexes agents and skills as references; scoped documents only
// contain the rules that route to that subdirectory.
func emitAgentsTree(b spec.Bundle, cfg *config.Config, commandsDir string, dryRun bool) error {
	if len(b.Rules) == 0 && len(b.Agents) == 0 && len(b.Skills) == 0 {
		return nil
	}
	rootFile := emit.OutputFile(cfg, target, defaultOutFile)
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

// emitMCPSettings writes (or merges into) `.amp/settings.json` with the
// `amp.mcpServers` map. User-managed keys in an existing settings file
// survive the sync; only `amp.mcpServers` is overwritten. Capture mode
// and dry-run skip the disk read, which may report drift for unrelated
// user keys - matches the OpenCode adapter's trade-off.
func emitMCPSettings(mcps []spec.Entry, path string, dryRun bool) error {
	if len(mcps) == 0 {
		return nil
	}
	doc := readExistingSettings(path, dryRun)
	doc[ampMCPKey] = buildMCPMap(mcps)

	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal amp settings: %w", err)
	}
	return emit.WriteFile(path, string(raw)+"\n", dryRun)
}

func readExistingSettings(path string, dryRun bool) map[string]any {
	if dryRun || emit.IsCapturing() {
		return map[string]any{}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return map[string]any{}
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil || doc == nil {
		return map[string]any{}
	}
	return doc
}

func buildMCPMap(mcps []spec.Entry) map[string]any {
	out := map[string]any{}
	for _, e := range mcps {
		if e.Name == "" {
			continue
		}
		entry := buildMCPEntry(e)
		if len(entry) == 0 {
			continue
		}
		out[e.Name] = entry
	}
	return out
}

// buildMCPEntry renders a single MCP server in Amp's settings shape.
// Amp accepts the standard `command`/`args`/`env` for stdio and
// `url`/`headers` for HTTP transports (see Amp owner's manual MCP
// guide).
func buildMCPEntry(e spec.Entry) map[string]any {
	transport, _ := e.Meta["type"].(string)
	if transport == "" {
		transport = "stdio"
	}
	entry := map[string]any{}
	switch transport {
	case "stdio":
		if cmd, _ := e.Meta["command"].(string); cmd != "" {
			entry["command"] = cmd
		}
		if args := emit.StringSlice(e.Meta["args"]); len(args) > 0 {
			entry["args"] = args
		}
	case "http", "sse":
		if url, _ := e.Meta["url"].(string); url != "" {
			entry["url"] = url
		}
		if h := emit.StringMap(e.Meta["headers"]); len(h) > 0 {
			entry["headers"] = h
		}
	}
	if env := emit.StringMap(e.Meta["env"]); len(env) > 0 {
		entry["env"] = env
	}
	return entry
}

// commandFile renders one Amp slash command markdown file: description
// frontmatter (when present) followed by the spec body.
func commandFile(e spec.Entry) string {
	meta := emit.ResolveMeta(e.Meta, target)
	front := map[string]any{}
	if d, _ := meta["description"].(string); d != "" {
		front["description"] = d
	}
	var sb strings.Builder
	sb.WriteString(emit.Frontmatter(front))
	sb.WriteString("\n")
	sb.WriteString(e.Body)
	return sb.String()
}

func writeHeader(sb *strings.Builder, scope string) {
	if scope == "" {
		sb.WriteString("# AGENTS.md\n\n")
	} else {
		sb.WriteString("# AGENTS.md (" + scope + ")\n\n")
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

func writeAgentsSection(sb *strings.Builder, agents []spec.Entry, commandsDir string) {
	if len(agents) == 0 {
		return
	}
	sb.WriteString("## Agents\n\n")
	sb.WriteString("Amp slash commands. Definitions live in `" + commandsDir + "/`.\n\n")
	for _, a := range agents {
		emit.WriteReference(sb, a, filepath.Join(commandsDir, a.Name+".md"))
	}
}

func writeSkillsSection(sb *strings.Builder, skills []spec.Entry, commandsDir string, asCommands bool) {
	if len(skills) == 0 {
		return
	}
	sb.WriteString("## Skills\n\n")
	if asCommands {
		sb.WriteString("Amp slash commands. Definitions live in `" + commandsDir + "/`.\n\n")
	} else {
		sb.WriteString("Reference material. Read the source file to use a skill.\n\n")
	}
	for _, s := range skills {
		var sourcePath string
		if asCommands {
			sourcePath = filepath.Join(commandsDir, skillFilenamePrefix+s.Name+".md")
		} else {
			sourcePath = s.Path
		}
		emit.WriteReference(sb, s, sourcePath)
	}
}
