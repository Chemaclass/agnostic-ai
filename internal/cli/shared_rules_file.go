package cli

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// sharedRulesFile is a target whose merged rules-file is its own
// instructions file while sync also writes that file for other readers.
// Sync writes instructions files after every adapter, so the readers'
// version replaces the merged file. It holds no AGNOSTIC_AI.md body and
// only the owner's rules, so it cannot win either (#3302).
type sharedRulesFile struct {
	path    string
	owner   string
	readers []string
	// lost names the owner's rules the readers' version leaves out.
	lost []string
}

// sharedRulesFiles finds every sharedRulesFile among files, the
// instructions files sync writes for cfg and b.
func sharedRulesFiles(cfg *config.Config, b spec.Bundle, files []entryPointFile) []sharedRulesFile {
	var out []sharedRulesFile
	for _, owner := range slices.Sorted(slices.Values(cfg.Targets)) {
		if !adapters.LegacyRulesFileOwnsEntryPoint(cfg, owner) {
			continue
		}
		path := adapters.EntryPointPath(cfg, owner)
		if cfg.IsUnmanaged(path) {
			continue
		}
		at := slices.IndexFunc(files, func(f entryPointFile) bool { return filepath.Clean(f.Path) == filepath.Clean(path) })
		if at < 0 {
			continue
		}
		readers := files[at].Readers
		out = append(out, sharedRulesFile{path: path, owner: owner, readers: readers, lost: rulesLeftOut(cfg, b, owner, readers)})
	}
	return out
}

// rulesLeftOut names the rules owner's merged file holds that the rules
// block sync inlines for readers does not.
func rulesLeftOut(cfg *config.Config, b spec.Bundle, owner string, readers []string) []string {
	kept := map[string]bool{}
	if inliners := pathRuleInliners(cfg, readers); len(inliners) > 0 {
		inlined, _ := withoutPartialBuiltinRules(b, readers)
		for _, r := range adapters.EntryPointRules(inlined, inliners[0], cfg).Rules {
			kept[r.Name] = true
		}
	}
	var lost []string
	for _, r := range adapters.EntryPointRules(b, owner, cfg).Rules {
		if !kept[r.Name] {
			lost = append(lost, r.Name)
		}
	}
	return lost
}

func (s sharedRulesFile) conflict() string {
	names := strings.Join(s.readers, ", ")
	fileKey := "outputs.<target>.file"
	if len(s.readers) == 1 {
		fileKey = "outputs." + s.readers[0] + ".file"
	}
	return fmt.Sprintf("%s: outputs.%s.rules-file puts %s's merged rules there, and sync also writes it as the instructions file of %s, which leaves out %s's rules %s. Point outputs.%s.rules-file elsewhere, remove it, or set %s",
		s.path, s.owner, s.owner, names, s.owner, strings.Join(s.lost, ", "), s.owner, fileKey)
}

func (s sharedRulesFile) noEffect() string {
	return fmt.Sprintf("outputs.%s.rules-file has no effect on %s: sync writes it as the instructions file of %s, which already carries %s's rules",
		s.owner, s.path, strings.Join(s.readers, ", "), s.owner)
}

// sharedRulesFileConflicts is the message for each sharedRulesFile that
// loses rules, for a project whose instructions files cannot be rendered
// it is none.
func sharedRulesFileConflicts(cfg *config.Config, b spec.Bundle, targets []string) []string {
	var out []string
	for _, s := range projectSharedRulesFiles(cfg, b, targets) {
		if len(s.lost) > 0 {
			out = append(out, s.conflict())
		}
	}
	return out
}

func projectSharedRulesFiles(cfg *config.Config, b spec.Bundle, targets []string) []sharedRulesFile {
	cfg = cfg.WithAdditionalTargets(targets...)
	files, err := renderEntryPointFiles(cfg, b, cfg.Targets, "")
	if err != nil {
		return nil
	}
	return sharedRulesFiles(cfg, b, files)
}

// replacedRulesFiles maps each owner whose merged rules-file sync
// replaces, with no rule lost, to that path. Drift checks compare the
// path against the instructions file sync leaves there, not the merged
// file the adapter renders.
func replacedRulesFiles(cfg *config.Config, b spec.Bundle, targets []string) map[string]string {
	out := map[string]string{}
	for _, s := range projectSharedRulesFiles(cfg, b, targets) {
		if len(s.lost) == 0 {
			out[s.owner] = filepath.Clean(s.path)
		}
	}
	return out
}
