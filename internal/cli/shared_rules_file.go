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

// sharedRulesFile is a target whose merged rules-file is a file sync also
// writes as other targets' instructions file. Sync writes instructions
// files after every adapter, so their version replaces the merged file.
// The merged file holds no AGNOSTIC_AI.md body and only the owner's
// rules, so it cannot win either (#3302).
type sharedRulesFile struct {
	path    string
	owner   string
	readers []string
	// kept is set when the owner still loads the file and the readers'
	// version carries every rule the merged file would hold.
	kept bool
	// missing names the owner's rules the readers' version leaves out.
	missing []string
}

// sharedRulesFiles finds every sharedRulesFile among files, the
// instructions files sync writes for cfg and b.
func sharedRulesFiles(cfg *config.Config, b spec.Bundle, files []entryPointFile) []sharedRulesFile {
	var out []sharedRulesFile
	for _, owner := range slices.Sorted(slices.Values(cfg.Targets)) {
		if !adapters.HasLegacyRulesFile(cfg, owner) {
			continue
		}
		path := cfg.Outputs[owner].RulesFile
		if cfg.IsUnmanaged(path) {
			continue
		}
		at := slices.IndexFunc(files, func(f entryPointFile) bool { return samePath(f.Path, path) })
		if at < 0 {
			continue
		}
		readers := slices.DeleteFunc(slices.Clone(files[at].Readers), func(t string) bool { return t == owner })
		s := sharedRulesFile{path: path, owner: owner, readers: readers}
		if adapters.WritesRulesFile(owner) && loadsRulesFile(cfg, owner) {
			s.missing = rulesLeftOut(cfg, b, owner, files[at].Readers)
			s.kept = len(s.missing) == 0
		}
		out = append(out, s)
	}
	return out
}

// loadsRulesFile reports whether owner reads its own rules-file: as its
// instructions file, or through the import its instructions file gains.
func loadsRulesFile(cfg *config.Config, owner string) bool {
	return adapters.LegacyRulesFileOwnsEntryPoint(cfg, owner) || adapters.RenderLegacyRulesFileImportAppendix(cfg, owner) != ""
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
	detail := ""
	if len(s.missing) > 0 {
		detail = " (" + names + " leave out " + strings.Join(s.missing, ", ") + ")"
	}
	return fmt.Sprintf("%s: outputs.%s.rules-file puts %s's merged rules there, but sync writes that file last as the instructions file of %s, so %s loses those rules%s. Point outputs.%s.rules-file elsewhere, remove it, or set %s",
		s.path, s.owner, s.owner, names, s.owner, detail, s.owner, fileKey)
}

func (s sharedRulesFile) noEffect() string {
	return fmt.Sprintf("outputs.%s.rules-file has no effect on %s: sync writes it as the instructions file of %s, which already carries %s's rules",
		s.owner, s.path, strings.Join(s.readers, ", "), s.owner)
}

// sharedRulesFileConflicts is the message for each sharedRulesFile that
// loses rules. A project whose instructions files cannot be rendered has
// none here; sync reports that error itself.
func sharedRulesFileConflicts(cfg *config.Config, b spec.Bundle, targets []string) []string {
	var out []string
	for _, s := range projectSharedRulesFiles(cfg, b, targets) {
		if !s.kept {
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

// replacedRulesFiles maps each owner whose merged rules-file a run over
// targets replaces with no rule lost to that path. A drift check compares
// the path with the instructions file that run writes there, not the
// owner's render. A run that writes no instructions file there checks
// the owner's render, which the collision winners already give the
// instructions file's bytes.
func replacedRulesFiles(cfg *config.Config, b spec.Bundle, targets []string) map[string]string {
	cfg = cfg.WithAdditionalTargets(targets...)
	runFiles, err := renderEntryPointFiles(cfg, b, targets, "")
	if err != nil {
		return nil
	}
	out := map[string]string{}
	for _, s := range projectSharedRulesFiles(cfg, b, nil) {
		if s.kept && slices.ContainsFunc(runFiles, func(f entryPointFile) bool { return samePath(f.Path, s.path) }) {
			out[s.owner] = filepath.Clean(s.path)
		}
	}
	return out
}
