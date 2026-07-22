package emit

import (
	"io/fs"
	"path/filepath"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// PropagateSkillAssets mirrors the sibling files of a folder-based skill
// (`<skills-src>/<name>/SKILL.md`) into the emitted skill folder, applying
// skip to exclude entries the adapter re-renders itself (SKILL.md, plus any
// adapter-specific extras such as codex-only assets).
//
// Flat-file skills (`<skills-src>/<name>.md`, the default `agnostic-ai new
// skill` scaffold) share one source directory, so a naive copy would mirror
// every other skill's body into each skill's folder (#387). Detection keys
// on the SKILL.md basename, matching the loader's folder-skill rule, and
// propagation is suppressed for flat-file skills.
//
// No-op when the source path is unknown (empty Path, e.g. in-memory specs
// from the WASM playground) so adapters stay safe for non-disk callers.
func (s *Session) PropagateSkillAssets(sk spec.Entry, dstDir string, skip func(rel string) bool, dryRun bool) error {
	if !FolderBasedSkill(sk) {
		return nil
	}
	return s.CopyTree(filepath.Dir(sk.Path), dstDir, skip, dryRun)
}

// SkipSKILLMd is the common sibling-asset skip predicate: it excludes the
// re-rendered SKILL.md and copies everything else verbatim. Adapters that
// need extra exclusions (e.g. codex-only assets) pass their own predicate.
func SkipSKILLMd(rel string) bool { return rel == "SKILL.md" }

// SkillHasBundledAssets reports whether a folder-based skill carries
// sibling files beyond the ones the adapter re-renders itself (those
// matched by skip, typically SKILL.md). Flat-file skills and in-memory
// specs (empty Path) never have bundled assets.
//
// Adapters that flatten a skill to a single file (e.g. Cursor emits one
// `.mdc` rule per skill) cannot represent attached payloads, so they call
// this to surface a coverage note instead of dropping the files silently.
func SkillHasBundledAssets(s spec.Entry, skip func(rel string) bool) bool {
	if !FolderBasedSkill(s) {
		return false
	}
	root := filepath.Dir(s.Path)
	found := false
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}
		if skip != nil && skip(filepath.ToSlash(rel)) {
			return nil
		}
		found = true
		return filepath.SkipAll
	})
	return found
}

// FolderBasedSkill reports whether the skill spec owns its own directory
// (`<name>/SKILL.md`) rather than living as a flat file (`<name>.md`).
// Folder-based skills own their sibling assets; flat-file skills share the
// parent skills directory and have no per-skill assets to propagate.
func FolderBasedSkill(s spec.Entry) bool {
	return s.Path != "" && filepath.Base(s.Path) == "SKILL.md"
}
