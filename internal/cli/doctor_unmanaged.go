package cli

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
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
	{"CLAUDE.md", "claude"},
	{"AGENTS.md", "codex"},
	{"GEMINI.md", "gemini"},
	{"CONVENTIONS.md", "aider"},
	{".cursor/BUGBOT.md", "cursor"},
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
	{".gemini/commands/*.toml", "gemini"},
	{".opencode/commands/*.md", "opencode"},
	{".agents/commands/*.md", "amp"},
}

// unmanagedFinding is one config file present on disk but not generated
// from `.agnostic-ai/`.
type unmanagedFinding struct {
	Path   string
	Target string
}

// findUnmanagedConfig scans root for known agentic config files that
// exist but carry no agnostic-ai provenance marker. Returns findings
// sorted by path. Read errors on individual files are skipped: doctor is
// advisory and a single unreadable file should not abort the scan.
func findUnmanagedConfig(root string) ([]unmanagedFinding, error) {
	var out []unmanagedFinding
	for _, c := range unmanagedConfigGlobs {
		matches, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(c.glob)))
		if err != nil {
			// Only ErrBadPattern, which our static globs never trigger.
			return nil, err
		}
		for _, m := range matches {
			info, err := os.Stat(m)
			if err != nil || info.IsDir() {
				continue
			}
			data, err := os.ReadFile(m)
			if err != nil {
				continue
			}
			if header.Has(string(data)) {
				continue
			}
			rel, err := filepath.Rel(root, m)
			if err != nil {
				rel = m
			}
			out = append(out, unmanagedFinding{Path: filepath.ToSlash(rel), Target: c.target})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

// reportUnmanagedConfig prints the doctor section listing agentic config
// files present on disk but not single-sourced from `.agnostic-ai/`,
// each with the `import` command that would adopt it. Returns the number
// of findings so callers can fold the count into a summary.
func reportUnmanagedConfig(cmd *cobra.Command, root string) int {
	findings, err := findUnmanagedConfig(root)
	if err != nil {
		return 0
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
		cmd.Printf("    → adopt with: agnostic-ai import %s\n", target)
	}
	return len(findings)
}
