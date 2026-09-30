package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode"

	"github.com/spf13/cobra"
	"golang.org/x/text/unicode/norm"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// globalNameWinners records, per kind and target, which copy the tool
// loads when a project spec and a user-level one share a name: true
// when the user-level one wins. Only documented precedence is listed.
// Codex shows both skills, and the other targets document none.
var globalNameWinners = map[spec.Kind]map[string]bool{
	// code.claude.com/docs/en/skills: personal over project.
	// geminicli.com/docs/cli/skills: workspace over user.
	// ampcode.com/docs/customize/skills: ~/.agents/skills/ masks
	// .agents/skills/, the amp target's own paths. Amp reads project
	// .claude/skills/ before ~/.claude/skills/, so a clash from the
	// claude target alone loads the project copy there.
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
		homeRelative(c.global.Path), strings.Join(winners, ", "))
}

func loaders(targets []string) string {
	if len(targets) == 1 {
		return targets[0] + " loads"
	}
	return strings.Join(targets[:len(targets)-1], ", ") + " and " + targets[len(targets)-1] + " load"
}

func homeRelative(path string) string {
	if home, err := globalUserHome(); err == nil {
		if rel, err := filepath.Rel(home, path); err == nil && filepath.IsLocal(rel) {
			return "~/" + filepath.ToSlash(rel)
		}
	}
	return filepath.ToSlash(path)
}

// claudeSkillName folds a skill name the way Claude Code matches it:
// compatibility forms, case, spacing, and invisible characters do not
// tell two names apart.
func claudeSkillName(name string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || unicode.Is(unicode.Cf, r) {
			return -1
		}
		return unicode.ToLower(r)
	}, norm.NFKC.String(name))
}

// sameNameIn reports whether target loads p and g under one name.
func sameNameIn(kind spec.Kind, target string, p, g spec.Entry) bool {
	if kind == spec.KindSkill && target == "claude" {
		return claudeSkillName(p.Name) == claudeSkillName(g.Name)
	}
	return p.Name == g.Name
}

func hasUserLevelSurface(kind spec.Kind, target string) bool {
	if kind == spec.KindAgent {
		return globalTargets[target].agents != ""
	}
	return globalTargets[target].skills != ""
}

// sameFile is true when both paths name one file, as they do when the
// project is the global home or the home links to the project's specs.
func sameFile(a, b string) bool {
	infoA, errA := os.Stat(a)
	infoB, errB := os.Stat(b)
	return errA == nil && errB == nil && os.SameFile(infoA, infoB)
}

// globalNameClashes compares the project's skills and agents with the
// global home's, for the targets both write. A missing or broken global
// home reports nothing: project sync never depends on it.
func globalNameClashes(b spec.Bundle, targets []string) []globalNameClash {
	source, err := globalSourceRoot()
	if err != nil {
		return nil
	}
	if info, err := os.Stat(source); err != nil || !info.IsDir() {
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
		for _, p := range kind.project {
			for _, g := range kind.global {
				if claudeSkillName(p.Name) != claudeSkillName(g.Name) || sameFile(p.Path, g.Path) {
					continue
				}
				clash := globalNameClash{project: p, global: g}
				for _, t := range shared {
					globalWins, documented := globalNameWinners[kind.kind][t]
					if !documented || !hasUserLevelSurface(kind.kind, t) || !sameNameIn(kind.kind, t, p, g) || !p.EmitsTo(t) || !g.EmitsTo(t) {
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
	}
	return out
}

// reportGlobalNameClashes lists them for doctor. A shared name can be
// on purpose, so it never fails the run.
func reportGlobalNameClashes(cmd *cobra.Command, b spec.Bundle, targets []string) {
	clashes := globalNameClashes(b, targets)
	if len(clashes) == 0 {
		return
	}
	cmd.Println()
	cmd.Println("Global names:")
	for _, c := range clashes {
		cmd.Printf("  ! %s\n", c)
	}
}
