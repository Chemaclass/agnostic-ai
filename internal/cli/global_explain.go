package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// runExplainGlobal lists where sync --global places one global spec for
// each target it writes: a whole file, a section of a shared file, or a
// key of a user settings file. A settings key a later spec overrides is
// not this spec's to claim.
func runExplainGlobal(cmd *cobra.Command, input string, jsonOut bool) error {
	scope, err := loadCheckScope(true)
	if err != nil {
		return err
	}
	if !filepath.IsAbs(input) {
		if _, statErr := os.Stat(input); statErr != nil {
			input = filepath.Join(scope.source, input)
		}
	}
	entry, err := findSpecEntry(input, scope.bundle)
	if err != nil {
		return err
	}
	home, err := globalUserHome()
	if err != nil {
		return err
	}
	adapters.SetWarner(io.Discard)
	defer adapters.SetWarner(os.Stderr)
	defer adapters.ResetCoverageNotes()
	contributions := []contribution{}
	for _, target := range slices.Sorted(slices.Values(scope.targets)) {
		found, err := globalContributions(home, target, entry, scope.bundle)
		if err != nil {
			return err
		}
		contributions = append(contributions, found...)
	}
	if jsonOut {
		return emitExplainJSON(cmd, explainOutput{
			Version:            "1",
			Command:            "explain",
			Spec:               explainSpecRef{Kind: string(entry.Kind), Name: entry.Name, Path: filepath.ToSlash(entry.Path)},
			Contributions:      contributions,
			WouldEmitIfEnabled: []contribution{},
		})
	}
	out := cmd.OutOrStdout()
	if _, err := fmt.Fprintf(out, "%s →\n", filepath.ToSlash(entry.Path)); err != nil {
		return fmt.Errorf("write explain output: %w", err)
	}
	if len(contributions) == 0 {
		_, err := fmt.Fprintln(out, "  (no target sync --global writes takes anything from this spec)")
		return err
	}
	for _, c := range contributions {
		if _, err := fmt.Fprintf(out, "  %s\n", formatContribution(c)); err != nil {
			return fmt.Errorf("write explain output: %w", err)
		}
	}
	return nil
}

// globalContributions is where one spec lands for target under sync
// --global, following the same surfaces buildGlobalWrites writes.
func globalContributions(home, target string, entry spec.Entry, b spec.Bundle) ([]contribution, error) {
	g := globalTargets[target]
	var out []contribution
	switch entry.Kind {
	case spec.KindAgent:
		if g.agents == "" {
			return nil, nil
		}
		files, err := adapters.RenderAgents(target, []spec.Entry{entry}, g.agentsPath(home))
		if err != nil {
			return nil, err
		}
		for _, f := range files {
			out = append(out, contribution{Target: target, Path: f.Path, Mode: "full"})
		}
	case spec.KindSkill:
		if g.skills != "" && entry.EmitsTo(target) {
			path := filepath.Join(g.path(home, g.skills), entry.Name, "SKILL.md")
			out = append(out, contribution{Target: target, Path: path, Mode: "full"})
		}
	case spec.KindRule:
		switch {
		case g.instructions != "":
			out = append(out, contribution{Target: target, Path: g.path(home, g.instructions), Section: entry.Name, Mode: "section"})
		case g.rules != "":
			out = append(out, contribution{Target: target, Path: filepath.Join(g.path(home, g.rules), entry.Name+".md"), Mode: "full"})
		}
	case spec.KindHook:
		event, _ := entry.Meta["event"].(string)
		if g.hooks != "" && entry.EmitsTo(target) && event != "" {
			out = append(out, contribution{Target: target, Path: g.path(home, g.hooks), Section: event, Mode: "section"})
		}
	case spec.KindMCP:
		if g.mcp.path == "" || !entry.EmitsTo(target) {
			return nil, nil
		}
		out = append(out, contribution{Target: target, Path: g.path(home, g.mcp.path), Section: g.mcp.key + "." + entry.Name, Mode: "key"})
	case spec.KindSettings:
		if g.settings.path == "" {
			return nil, nil
		}
		for _, s := range globalSettingsFor(target, g, b.Settings) {
			if s.source == entry.Path {
				out = append(out, contribution{Target: target, Path: g.path(home, g.settings.path), Section: s.key, Mode: "key"})
			}
		}
	}
	return out, nil
}
