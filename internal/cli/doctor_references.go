package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/mdlink"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// referenceFinding is one relative Markdown link in an emitted skill
// document whose destination is missing on disk. Source is the canonical
// spec file the document came from, empty when it cannot be attributed.
type referenceFinding struct {
	Target      string `json:"target"`
	Source      string `json:"source,omitempty"`
	Path        string `json:"path"`
	Line        int    `json:"line"`
	Destination string `json:"destination"`
}

// collectReferenceFindings checks every local-file Markdown link in the
// skill documents each selected target emits. A document counts as
// skill-owned when it disappears from the capture once the bundle has no
// skills, so the check follows the adapters' real output layout and never
// scans unrelated repository files. Read-only: adapters run in capture
// mode and only emitted documents already on disk are read.
func collectReferenceFindings(targets []string) (findings []referenceFinding, docs int, err error) {
	cfg, b, err := loadProject(".")
	if err != nil {
		return nil, 0, err
	}
	if len(targets) == 0 {
		targets = cfg.Targets
	}
	adapters.SetWarner(io.Discard)
	defer adapters.SetWarner(os.Stderr)

	sess := adapters.NewSession()
	for _, t := range targets {
		adapter, err := adapters.Resolve(t)
		if err != nil {
			continue // collectDrift already warned about the unknown target
		}
		owned, err := skillOwnedDocs(sess, adapter, b, cfg)
		if err != nil {
			return nil, 0, fmt.Errorf("%s: %w", t, err)
		}
		skills := b.For(t).Skills
		for _, p := range owned {
			data, err := os.ReadFile(p)
			if err != nil {
				continue // an absent document is a drift finding, not ours
			}
			docs++
			source, attributed := "", false
			for _, l := range mdlink.Local(string(data)) {
				dest := filepath.Join(filepath.Dir(p), filepath.FromSlash(l.Dest))
				if _, err := os.Stat(dest); err == nil {
					continue
				}
				if !attributed {
					source = skillSourceFor(p, skills)
					if source == "" {
						source = skillSourceByRender(sess, adapter, b, cfg, p)
					}
					attributed = true
				}
				findings = append(findings, referenceFinding{
					Target:      t,
					Source:      source,
					Path:        filepath.ToSlash(p),
					Line:        l.Line,
					Destination: l.Raw,
				})
			}
		}
	}
	return findings, docs, nil
}

// skillOwnedDocs returns the Markdown files a target writes only because
// the bundle has skills, sorted by path. Absolute paths (user-scope
// output) and user-owned paths are left out.
func skillOwnedDocs(sess *adapters.Session, adapter adapters.Adapter, b spec.Bundle, cfg *config.Config) ([]string, error) {
	full, err := captureAdapterFiles(sess, adapter, b, cfg)
	if err != nil {
		return nil, err
	}
	noSkills := b
	noSkills.Skills = nil
	without, err := captureAdapterFiles(sess, adapter, noSkills, cfg)
	if err != nil {
		return nil, err
	}
	kept := make(map[string]bool, len(without))
	for _, f := range without {
		kept[f.Path] = true
	}
	seen := map[string]bool{}
	var out []string
	for _, f := range full {
		if kept[f.Path] || seen[f.Path] || filepath.IsAbs(f.Path) || cfg.IsUnmanaged(f.Path) {
			continue
		}
		switch strings.ToLower(filepath.Ext(f.Path)) {
		case ".md", ".mdc", ".markdown":
		default:
			continue
		}
		seen[f.Path] = true
		out = append(out, f.Path)
	}
	sort.Strings(out)
	return out, nil
}

// skillSourceFor attributes an emitted document to its canonical source.
// The first path segment (directory, or file stem for a flattened skill)
// naming a skill picks it; for a folder skill the rest of the path is
// looked up beside its SKILL.md. Best effort: returns "" on no match.
func skillSourceFor(emitted string, skills []spec.Entry) string {
	segs := strings.Split(filepath.ToSlash(emitted), "/")
	last := len(segs) - 1
	for i, seg := range segs {
		if i == last {
			seg = strings.TrimSuffix(seg, filepath.Ext(seg))
		}
		for _, sk := range skills {
			if sk.Name != seg || sk.Path == "" {
				continue
			}
			rel := strings.Join(segs[i+1:], "/")
			if dir := sk.SkillAssetDir(); rel != "" && dir != "" {
				candidate := filepath.Join(dir, filepath.FromSlash(rel))
				if _, err := os.Stat(candidate); err == nil {
					return filepath.ToSlash(candidate)
				}
			}
			return filepath.ToSlash(sk.Path)
		}
	}
	return ""
}

// skillSourceByRender attributes a document whose path names no skill
// (e.g. Continue's `skill-<name>.md` rule) by re-rendering the target
// without each skill in turn until the document disappears. Runs only for
// a document that has a finding, so a clean project pays nothing.
func skillSourceByRender(sess *adapters.Session, adapter adapters.Adapter, b spec.Bundle, cfg *config.Config, emitted string) string {
	for i, sk := range b.Skills {
		minus := b
		minus.Skills = append(append([]spec.Entry{}, b.Skills[:i]...), b.Skills[i+1:]...)
		files, err := captureAdapterFiles(sess, adapter, minus, cfg)
		if err != nil {
			return ""
		}
		if !slices.ContainsFunc(files, func(f adapters.CapturedFile) bool { return f.Path == emitted }) {
			return filepath.ToSlash(sk.Path)
		}
	}
	return ""
}

// reportBrokenReferences prints the doctor section for
// --check-references and returns the number of broken links.
func reportBrokenReferences(cmd *cobra.Command, targets []string) (int, error) {
	cmd.Println()
	cmd.Println("Skill references:")
	findings, docs, err := collectReferenceFindings(targets)
	if err != nil {
		return 0, err
	}
	if len(findings) == 0 {
		cmd.Printf("  ✓ every local link in %d emitted skill document(s) resolves\n", docs)
		return 0, nil
	}
	for _, f := range findings {
		cmd.Printf("  ✗ %s: %s:%d links to missing %s\n", f.Target, f.Path, f.Line, f.Destination)
		if f.Source != "" {
			cmd.Printf("      source: %s\n", f.Source)
		}
	}
	cmd.Println("    fix: add the file to the skill folder under .agnostic-ai/ and run `agnostic-ai sync`, or correct the link in the source")
	return len(findings), nil
}
