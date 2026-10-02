package cli

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"reflect"
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
	content     contentMatch
}

// contentMatch says how a clash's two copies compare.
type contentMatch int

const (
	// contentUnknown makes no claim: a file could not be read, or a
	// `models:` tier resolves through a different config on each side.
	contentUnknown contentMatch = iota
	contentDiffers
	contentIdentical
)

func (c globalNameClash) String() string {
	head := fmt.Sprintf("%s: %s %q", filepath.ToSlash(c.project.Path), c.project.Kind, c.project.Name)
	global := homeRelative(c.global.Path)
	if c.content == contentIdentical {
		return fmt.Sprintf("%s is identical in %s, so every target loads the same content; delete one copy to keep them from drifting apart, and run `agnostic-ai sync --global` after deleting the global one", head, global)
	}
	var winners []string
	if len(c.globalWins) > 0 {
		winners = append(winners, loaders(c.globalWins)+" the global one, which exists only on this machine")
	}
	if len(c.projectWins) > 0 {
		loads := loaders(c.projectWins) + " this one"
		if len(c.globalWins) == 0 {
			loads += ", so the global one is unused here"
		}
		winners = append(winners, loads)
	}
	fix := "rename one to load both"
	if len(c.globalWins) > 0 {
		fix = fmt.Sprintf("to load both, rename the global one (such as %s-personal), or delete it to drop it, then run `agnostic-ai sync --global`", c.global.Name)
	}
	differs := ""
	if c.content == contentDiffers {
		differs = " with different content"
	}
	return fmt.Sprintf("%s also exists in %s%s; %s; %s",
		head, global, differs, strings.Join(winners, ", "), fix)
}

// compareContent compares the frontmatter two specs load (the name
// aside, since a target matched them by it), their body, and skill assets.
func compareContent(p, g spec.Entry) contentMatch {
	if strings.TrimSpace(p.Body) != strings.TrimSpace(g.Body) {
		return contentDiffers
	}
	pm, gm := maps.Clone(p.Meta), maps.Clone(g.Meta)
	delete(pm, "name")
	delete(gm, "name")
	// Each side resolves a tier through its own `models:` config, so
	// neither the tier name nor the resolved model proves a match.
	tiered := p.ModelTier != "" || g.ModelTier != ""
	if tiered {
		delete(pm, "model")
		delete(gm, "model")
	}
	if len(pm) != len(gm) || (len(pm) > 0 && !reflect.DeepEqual(pm, gm)) {
		return contentDiffers
	}
	assets := compareSkillAssets(p, g)
	if assets == contentIdentical && tiered {
		return contentUnknown
	}
	return assets
}

// compareSkillAssets compares the files two skills ship beside their
// SKILL.md: names and sizes first, then bytes one pair at a time.
func compareSkillAssets(p, g spec.Entry) contentMatch {
	pa, err := skillAssets(p)
	if err != nil {
		return contentUnknown
	}
	ga, err := skillAssets(g)
	if err != nil {
		return contentUnknown
	}
	if len(pa) != len(ga) {
		return contentDiffers
	}
	for rel, pf := range pa {
		if gf, ok := ga[rel]; !ok || gf.size != pf.size {
			return contentDiffers
		}
	}
	for rel, pf := range pa {
		pb, err := os.ReadFile(pf.path)
		if err != nil {
			return contentUnknown
		}
		gb, err := os.ReadFile(ga[rel].path)
		if err != nil {
			return contentUnknown
		}
		if !bytes.Equal(pb, gb) {
			return contentDiffers
		}
	}
	return contentIdentical
}

type skillAsset struct {
	path string
	size int64
}

// skillAssets maps each file a skill ships beside its SKILL.md, by slash
// path. A flat-file skill or an agent ships none. Like the emitter, it
// skips anything but regular files.
func skillAssets(e spec.Entry) (map[string]skillAsset, error) {
	out := map[string]skillAsset{}
	dir := e.SkillAssetDir()
	if e.Kind != spec.KindSkill || dir == "" {
		return out, nil
	}
	err := spec.WalkSourceRoot(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.Type().IsRegular() {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil || rel == "SKILL.md" {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		out[filepath.ToSlash(rel)] = skillAsset{path: path, size: info.Size()}
		return nil
	})
	return out, err
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
					clash.content = compareContent(p, g)
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
