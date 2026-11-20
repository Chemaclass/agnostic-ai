package cli

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/adapters/claude"
	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// unmanagedConfigGlobs enumerates known agentic config files that
// agnostic-ai can generate, paired with the `import` source that would
// bring each under `.agnostic-ai/`. A matched file that lacks the
// provenance marker is hand-authored and not single-sourced, so doctor
// surfaces it with a concrete next step.
//
// Only header-bearing formats (markdown, TOML) are listed: every
// agnostic-ai-generated file of these kinds carries the marker (enforced
// by the per-adapter header-coverage tests), so a missing marker is a
// reliable "unmanaged" signal. JSON config (settings.json, mcp.json,
// hooks.json) carries no marker and is partially merge-managed, so it is
// covered by the drift section instead, not flagged here.
var unmanagedConfigGlobs = []struct{ glob, target string }{
	{".kimi-code/AGENTS.md", "kimi"},
	{".kimi-code/agents/*.md", "kimi"},
	{".kimi-code/skills/*/SKILL.md", "kimi"},
	{"CLAUDE.md", "claude"},
	{"AGENTS.md", "codex"},
	{"GEMINI.md", "gemini"},
	{"QWEN.md", "qwen"},
	{".qwen/agents/*.md", "qwen"},
	{".qwen/skills/*/SKILL.md", "qwen"},
	{"CONVENTIONS.md", "aider"},
	{".cursor/rules/*.mdc", "cursor"},
	{".cursor/agents/*.md", "cursor"},
	{".cursor/skills/*/SKILL.md", "cursor"},
	{".cursor/commands/*.md", "cursor"},
	{".claude/agents/*.md", "claude"},
	{".claude/commands/*.md", "claude"},
	{".claude/skills/*/SKILL.md", "claude"},
	{".codex/prompts/*.md", "codex"},
	{".codex/agents/*.toml", "codex"},
	{".agents/skills/*/SKILL.md", "codex"},
	{".gemini/skills/*/SKILL.md", "gemini"},
	{".opencode/commands/*.md", "opencode"},
	{".opencode/agents/*.md", "opencode"},
	{".opencode/skills/*/SKILL.md", "opencode"},
	{".github/agents/*.md", "copilot"},
	{".github/skills/*/SKILL.md", "copilot"},
	{".ellipsis/code-review.yaml", "ellipsis"},
	{".ellipsis/environments/*.yaml", "ellipsis"},
	// Amp stopped reading this directory, and sync stopped writing it
	// (#727). Kept so a project that never migrated is told its leftover
	// command files are worth importing rather than losing.
	{".agents/commands/*.md", "amp"},
}

// importedOnceFile is a config file a tool reads that sync never
// writes, so import leaves it in place. dest, with %s for the name, is
// the file sync writes from its spec under outputs dir out, which the
// tool loads as well, and tool names that tool. A file whose dest is
// empty has no such copy.
type importedOnceFile struct{ glob, target, kind, suffix, dest, out, tool string }

// unmanagedImportedOnce lists the imported-once files. One stays
// unmanaged until the spec import writes from it exists, then while it
// duplicates the file sync wrote from that spec.
var unmanagedImportedOnce = []importedOnceFile{
	{cursorLegacyRulesFile, "cursor", "rules", "", "", "", ""},
	{".github/prompts/*" + copilotPromptSuffix, "copilot", "commands", copilotPromptSuffix, ".github/skills/%s/SKILL.md", "skills", "Copilot"},
	{".opencode/agent/*.md", "opencode", "agents", ".md", ".opencode/agents/%s.md", "agents", "OpenCode"},
	{".opencode/command/*.md", "opencode", "commands", ".md", ".opencode/commands/%s.md", "commands", "OpenCode"},
}

// nameOf returns the spec name of the imported-once file at path.
func (f importedOnceFile) nameOf(path string) string {
	return strings.TrimPrefix(strings.TrimSuffix(filepath.Base(path), f.suffix), ".")
}

// destFor returns the file sync writes from the spec named name, for an
// enabled target with the default output folder. A `group:name` command
// lands as `group-name`.
func (f importedOnceFile) destFor(cfg *config.Config, name string) (string, bool) {
	if f.dest == "" || cfg == nil || !slices.Contains(cfg.Targets, f.target) {
		return "", false
	}
	o := cfg.Outputs[f.target]
	if map[string]string{"agents": o.AgentsDir, "commands": o.CommandsDir, "skills": o.SkillsDir}[f.out] != "" {
		return "", false
	}
	return fmt.Sprintf(f.dest, strings.ReplaceAll(name, ":", "-")), true
}

// duplicate returns the generated file under root that repeats the
// imported-once file at path.
func (f importedOnceFile) duplicate(root string, cfg *config.Config, path string) (string, bool) {
	dest, ok := f.destFor(cfg, f.nameOf(path))
	if !ok {
		return "", false
	}
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(dest)))
	return dest, err == nil && header.Has(string(data))
}

// unmanagedMCPFiles lists MCP files sync replaces whole. JSON carries no
// marker, so one is unmanaged when it names a server no MCP spec has.
var unmanagedMCPFiles = []struct{ path, key, target string }{
	{".cursor/mcp.json", "mcpServers", "cursor"},
}

// unmanagedTrees lists config folders a tool reads at every depth, which
// filepath.Glob cannot express: it reads `**` as one folder level.
var unmanagedTrees = []struct{ dir, ext, target string }{
	{qwenRulesDir, ".md", "qwen"},
	{".qwen/commands", ".md", "qwen"},
	{geminiCommandsDir, ".toml", "gemini"},
}

// unmanagedScopedFiles lists config files a tool reads from its
// directory at the root and below any project subdirectory, which a
// static glob cannot reach at every depth. `import cursor` reads each
// BUGBOT.md scope (#1276).
var unmanagedScopedFiles = []struct{ nativeDir, file, target string }{
	{".cursor", cursorReviewFile, "cursor"},
}

// noteImportedOnceDuplicates names each imported-once file of target
// whose spec sync writes for it, to delete after the next sync: the tool
// loads both, and a later spec edit reaches only the copy. Import never
// removes a file it did not write.
func noteImportedOnceDuplicates(root, target string, cfg *config.Config) {
	for _, c := range unmanagedImportedOnce {
		if c.target != target {
			continue
		}
		matches, _ := filepath.Glob(filepath.Join(root, filepath.FromSlash(c.glob)))
		for _, m := range matches {
			name := c.nameOf(m)
			dest, ok := c.destFor(cfg, name)
			if !ok || !c.specEmits(root, cfg, name) {
				continue
			}
			if data, err := os.ReadFile(m); err != nil || header.Has(string(data)) {
				continue
			}
			rel, err := filepath.Rel(root, m)
			if err != nil {
				rel = m
			}
			summaryf("  ! delete %s after the next sync: sync writes it to %s, and %s loads both\n", filepath.ToSlash(rel), dest, c.tool)
		}
	}
}

// specEmits reports whether the spec named name reaches the target of f,
// and as the file dest names: Copilot gets a command as a skill only
// when it writes commands as skills.
func (f importedOnceFile) specEmits(root string, cfg *config.Config, name string) bool {
	kind := map[string]spec.Kind{"agents": spec.KindAgent, "commands": spec.KindCommand}[f.kind]
	data, err := os.ReadFile(filepath.Join(unmanagedSourceDir(root, cfg, f.kind), name+".md"))
	if err != nil {
		return false
	}
	e, err := spec.ParseMarkdownBytes(kind, data)
	if err != nil || !e.EmitsTo(f.target) {
		return false
	}
	return f.out != "skills" || adapters.WritesCommandsAsSkills(cfg, f.target)
}

// unmanagedFinding is one config file present on disk but not generated
// from `.agnostic-ai/`.
type unmanagedFinding struct {
	Path   string
	Target string
	// Duplicate, when set, says the file repeats one sync writes and
	// how to fix it; importing it again changes nothing.
	Duplicate string
}

// findUnmanagedConfig scans root for known agentic config files that
// exist but carry no agnostic-ai provenance marker. Returns findings
// sorted by path. Read errors on individual files are skipped: doctor is
// advisory and a single unreadable file should not abort the scan. A
// failed walk drops only the scoped files it would have found.
func findUnmanagedConfig(root string, cfg *config.Config) ([]unmanagedFinding, error) {
	var out []unmanagedFinding
	headerless := headerlessEntryPoints(cfg)
	sums := readStateFile(root).OutputSums
	add := func(path, target string) *unmanagedFinding {
		data, err := os.ReadFile(path)
		if err != nil || header.Has(string(data)) {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			rel = path
		}
		rel = filepath.ToSlash(rel)
		// A file written without the header (provenance-header: false) is
		// managed when the sync record holds its bytes or sync writes it
		// headerless; a fresh clone has no record.
		if headerless[rel] || sums[rel] != "" && adapters.MatchesSum(data, sums[rel]) {
			return nil
		}
		out = append(out, unmanagedFinding{Path: rel, Target: target})
		return &out[len(out)-1]
	}
	for _, c := range claudeGlobsFor(cfg, unmanagedConfigGlobs) {
		matches, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(c.glob)))
		if err != nil {
			// Only ErrBadPattern, which our static globs never trigger.
			return nil, err
		}
		for _, m := range matches {
			add(m, c.target)
		}
	}
	for _, c := range unmanagedImportedOnce {
		matches, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(c.glob)))
		if err != nil {
			return nil, err
		}
		for _, m := range matches {
			if !fileExists(filepath.Join(unmanagedSourceDir(root, cfg, c.kind), c.nameOf(m)+".md")) {
				add(m, c.target)
				continue
			}
			if dest, ok := c.duplicate(root, cfg, m); ok {
				if f := add(m, c.target); f != nil {
					f.Duplicate = fmt.Sprintf("delete it: sync writes %s from its spec, and %s loads both", dest, c.tool)
				}
			}
		}
	}
	for _, f := range unmanagedMCPFiles {
		path := filepath.Join(root, filepath.FromSlash(f.path))
		if mcpServersWithoutSpecs(path, f.key, unmanagedSourceDir(root, cfg, "mcps")) {
			add(path, f.target)
		}
	}
	for _, tree := range unmanagedTrees {
		_ = filepath.WalkDir(filepath.Join(root, filepath.FromSlash(tree.dir)), func(path string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() && strings.HasSuffix(d.Name(), tree.ext) {
				add(path, tree.target)
			}
			return nil
		})
	}
	for _, f := range unmanagedScopedFiles {
		dirs, err := findScopedSkillDirs(root, f.nativeDir)
		if err != nil {
			continue
		}
		for _, dir := range dirs {
			add(filepath.Join(dir.path, f.file), f.target)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

// withoutLeftOutClaudeAgents moves out of found each Claude agent file
// whose spec sync leaves out for an empty tool list. Import already has
// that spec, so adopting the file changes nothing; the returned lines say
// why it stays unmanaged instead.
func withoutLeftOutClaudeAgents(found []unmanagedFinding, b spec.Bundle, cfg *config.Config) ([]unmanagedFinding, []string) {
	dir := filepath.ToSlash(claudeLayoutFor(cfg).agents)
	leftOut := map[string]string{}
	for _, a := range b.Agents {
		if a.EmitsTo("claude") && claude.LeavesOutAgent(a) {
			leftOut[dir+"/"+a.Name+".md"] = a.Name
		}
	}
	if len(leftOut) == 0 {
		return found, nil
	}
	var kept []unmanagedFinding
	var lines []string
	for _, f := range found {
		name, ok := leftOut[f.Path]
		if f.Target != "claude" || !ok {
			kept = append(kept, f)
			continue
		}
		lines = append(lines, fmt.Sprintf("%s: sync leaves agent %s out of Claude Code because its tool list is empty; give it a tool, or delete the file", f.Path, name))
	}
	return kept, lines
}

// reportUnmanagedConfig prints the doctor section listing agentic config
// files present on disk but not single-sourced from `.agnostic-ai/`,
// each with the `import` command that would adopt it. Paths listed under
// sync.unmanaged are user-owned by choice, so they are left out. Returns
// the number of findings so callers can fold the count into a summary.
func reportUnmanagedConfig(cmd *cobra.Command, root string, cfg *config.Config) int {
	all, err := findUnmanagedConfig(root, cfg)
	return printUnmanagedConfig(cmd, cfg, all, err)
}

// printUnmanagedConfig prints what findUnmanagedConfig found.
func printUnmanagedConfig(cmd *cobra.Command, cfg *config.Config, all []unmanagedFinding, err error) int {
	if err != nil {
		return 0
	}
	var findings []unmanagedFinding
	for _, f := range all {
		if !cfg.IsUnmanaged(f.Path) {
			findings = append(findings, f)
		}
	}
	cmd.Println()
	cmd.Println("Unmanaged config (on disk, not generated from .agnostic-ai/):")
	if len(findings) == 0 {
		cmd.Println("  ✓ none found")
		return 0
	}
	byTarget := map[string][]string{}
	var targetOrder []string
	for _, f := range findings {
		if f.Duplicate != "" {
			cmd.Printf("  ✗ %s\n    → %s\n", f.Path, f.Duplicate)
			continue
		}
		if _, seen := byTarget[f.Target]; !seen {
			targetOrder = append(targetOrder, f.Target)
		}
		byTarget[f.Target] = append(byTarget[f.Target], f.Path)
	}
	sort.Strings(targetOrder)
	for _, target := range targetOrder {
		for _, p := range byTarget[target] {
			cmd.Printf("  ✗ %s\n", p)
		}
		// Import alone leaves a tool outside targets unmanaged, so
		// doctor would keep giving the same advice, unless another
		// enabled target writes the same root AGENTS.md.
		onlyAgentsMd := !slices.ContainsFunc(byTarget[target], func(p string) bool { return p != claudeAgentsMainFile })
		if slices.Contains(cfg.Targets, target) || onlyAgentsMd && slices.ContainsFunc(cfg.Targets, func(t string) bool { return agentsMainFileImporters[t] }) {
			cmd.Printf("    → adopt with: agnostic-ai import %s\n", target)
		} else {
			cmd.Printf("    → adopt with: add %s to targets in agnostic-ai.yaml, then agnostic-ai import %s\n", target, target)
		}
	}
	return len(findings)
}

// reportUserOwned prints the sync.unmanaged entries so a file that sync
// keeps skipping is explained next to the drift report. Silent when the
// list is empty: nothing to explain.
func reportUserOwned(cmd *cobra.Command, cfg *config.Config) {
	if len(cfg.Sync.Unmanaged) == 0 {
		return
	}
	cmd.Println()
	cmd.Println("User-owned (sync.unmanaged):")
	for _, p := range cfg.Sync.Unmanaged {
		cmd.Printf("  ~ %s\n", p)
	}
}

// claudeGlobsFor rewrites the static Claude Code globs onto the
// directory sync actually writes. `outputs.claude.dir` and the per-kind
// keys move the tree, and a glob pinned to `.claude/` then reports
// nothing there, so unmanaged files under a moved directory went
// unseen (#852).
func claudeGlobsFor(cfg *config.Config, globs []struct{ glob, target string }) []struct{ glob, target string } {
	layout := claudeLayoutFor(cfg)
	if layout == defaultClaudeLayout() {
		return globs
	}
	moved := map[string]string{
		".claude/agents/*.md":       filepath.ToSlash(layout.agents) + "/*.md",
		".claude/commands/*.md":     filepath.ToSlash(layout.commands) + "/*.md",
		".claude/skills/*/SKILL.md": filepath.ToSlash(layout.skills) + "/*/SKILL.md",
	}
	out := make([]struct{ glob, target string }, 0, len(globs))
	for _, g := range globs {
		if replacement, ok := moved[g.glob]; ok {
			g.glob = replacement
		}
		out = append(out, g)
	}
	return out
}

// unmanagedSourceDir is the spec folder of kind under root.
func unmanagedSourceDir(root string, cfg *config.Config, kind string) string {
	dirs := map[string]string{"rules": cfg.Sources.Rules, "commands": cfg.Sources.Commands, "agents": cfg.Sources.Agents, "mcps": cfg.Sources.MCPs}
	dir := dirs[kind]
	if dir == "" {
		dir = config.SourceBaseDir + "/" + kind
	}
	return config.ResolveSourcePath(root, dir)
}

// mcpServersWithoutSpecs reports whether the MCP file at path names a
// server under key that has no spec in specDir.
func mcpServersWithoutSpecs(path, key, specDir string) bool {
	servers, err := readJSONCMapAt(path, key)
	if err != nil || len(servers) == 0 {
		return err != nil && fileExists(path)
	}
	for name := range servers {
		if !fileExists(filepath.Join(specDir, spec.MCPFileName(name))) {
			return true
		}
	}
	return false
}
