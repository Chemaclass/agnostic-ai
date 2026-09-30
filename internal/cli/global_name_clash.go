package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// globalNameWinners records, per kind and target, which copy the tool
// loads when a project spec and a user-level one share a name: true
// when the user-level one wins. Only documented precedence is listed.
// Codex shows both skills, and the other targets document none.
var globalNameWinners = map[spec.Kind]map[string]bool{
	// code.claude.com/docs/en/skills: personal over project.
	// ampcode.com/docs/customize/skills: user-level directories mask a
	// project skill. geminicli.com/docs/cli/skills: workspace over user.
	spec.KindSkill: {"amp": true, "claude": true, "gemini": false},
	// code.claude.com/docs/en/sub-agents: .claude/agents/ over
	// ~/.claude/agents/.
	spec.KindAgent: {"claude": false},
}

// globalNameClash is a project spec whose name a global spec also uses,
// with the targets where each copy wins.
type globalNameClash struct {
	project     spec.Entry
	global      spec.Entry
	globalWins  []string
	projectWins []string
}

func (c globalNameClash) String() string {
	var winners []string
	if len(c.globalWins) > 0 {
		winners = append(winners, loaders(c.globalWins)+" the global one")
	}
	if len(c.projectWins) > 0 {
		winners = append(winners, loaders(c.projectWins)+" this one")
	}
	return fmt.Sprintf("%s: %s %q also exists in %s; %s; rename one to load both",
		filepath.ToSlash(c.project.Path), c.project.Kind, c.project.Name,
		filepath.ToSlash(c.global.Path), strings.Join(winners, ", "))
}

func loaders(targets []string) string {
	if len(targets) == 1 {
		return targets[0] + " loads"
	}
	return strings.Join(targets[:len(targets)-1], ", ") + " and " + targets[len(targets)-1] + " load"
}

// globalNameClashes compares the project's skills and agents with the
// global home's, for the targets both write. A missing or broken global
// home reports nothing: project sync never depends on it.
func globalNameClashes(root string, b spec.Bundle, targets []string) []globalNameClash {
	source, err := globalSourceRoot()
	if err != nil {
		return nil
	}
	if info, err := os.Stat(source); err != nil || !info.IsDir() || sameDir(source, root) {
		return nil
	}
	global, err := spec.LoadLayered(globalLayers(source))
	if err != nil {
		return nil
	}
	written, err := loadGlobalTargets(source, io.Discard)
	if err != nil {
		return nil
	}
	if written == nil {
		written = globalTargetNames()
	}
	var shared []string
	for _, t := range slices.Sorted(slices.Values(targets)) {
		if slices.Contains(written, t) {
			shared = append(shared, t)
		}
	}
	var out []globalNameClash
	for _, kind := range []struct {
		kind            spec.Kind
		project, global []spec.Entry
	}{
		{spec.KindAgent, b.Agents, global.Agents},
		{spec.KindSkill, b.Skills, global.Skills},
	} {
		byName := map[string]spec.Entry{}
		for _, e := range kind.global {
			byName[e.Name] = e
		}
		for _, p := range kind.project {
			g, ok := byName[p.Name]
			if !ok {
				continue
			}
			clash := globalNameClash{project: p, global: g}
			for _, t := range shared {
				globalWins, documented := globalNameWinners[kind.kind][t]
				if !documented || !p.EmitsTo(t) || !g.EmitsTo(t) {
					continue
				}
				if globalWins {
					clash.globalWins = append(clash.globalWins, t)
				} else {
					clash.projectWins = append(clash.projectWins, t)
				}
			}
			if len(clash.globalWins)+len(clash.projectWins) > 0 {
				out = append(out, clash)
			}
		}
	}
	return out
}

// reportGlobalNameClashes lists them for doctor. A shared name can be
// on purpose, so it never fails the run.
func reportGlobalNameClashes(cmd *cobra.Command, b spec.Bundle, targets []string) {
	clashes := globalNameClashes(".", b, targets)
	if len(clashes) == 0 {
		return
	}
	cmd.Println()
	cmd.Println("Global names:")
	for _, c := range clashes {
		cmd.Printf("  ! %s\n", c)
	}
}
