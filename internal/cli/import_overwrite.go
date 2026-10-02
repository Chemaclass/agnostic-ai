package cli

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

// overwrites returns the existing specs the import replaces with
// different content, sorted by path.
func (p importPreview) overwrites() []importPreviewEntry {
	var out []importPreviewEntry
	for _, e := range p.entries {
		if e.overwrites {
			out = append(out, e)
		}
	}
	return out
}

// syncedSpecFiles returns the spec files under project, slash-form and
// relative to it, whose spec still matches the fingerprint the last sync
// recorded: their native files were rendered from them, so an import
// replacing one is the documented re-import of a native edit. A skill
// counts with every file in its folder, since its fingerprint covers
// them all.
func syncedSpecFiles(project string) map[string]bool {
	synced := map[string]bool{}
	sums := readStateFile(project).SpecSums
	if len(sums) == 0 {
		return synced
	}
	_, b, err := loadProject(project)
	if err != nil {
		return synced
	}
	add := func(path string) {
		if !filepath.IsAbs(path) {
			path = filepath.Join(project, path)
		}
		if rel, err := filepath.Rel(project, path); err == nil {
			synced[filepath.ToSlash(rel)] = true
		}
	}
	for _, e := range b.All() {
		if e.Path == "" || sums[specKey(e)] != entrySum(e) {
			continue
		}
		add(e.Path)
		if dir := e.SkillAssetDir(); dir != "" {
			_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
				if err == nil && !d.IsDir() {
					add(p)
				}
				return nil
			})
		}
	}
	return synced
}

// importSpecDirs lists the spec directories of the project at root,
// slash-form and relative to it: the source base and every configured
// source directory.
func importSpecDirs(root string) []string {
	dirs := []string{config.SourceBaseDir}
	cfg, err := config.Load(root)
	if err != nil {
		return dirs
	}
	for _, d := range sourceDirsByKind(cfg.Sources) {
		if d != "" && !filepath.IsAbs(d) {
			dirs = append(dirs, filepath.ToSlash(filepath.Clean(d)))
		}
	}
	return dirs
}

// underAny reports whether the slash-form path lies inside one of dirs.
func underAny(path string, dirs []string) bool {
	return slices.ContainsFunc(dirs, func(d string) bool {
		return strings.HasPrefix(path, d+"/")
	})
}

// printImportOverwrites lists the existing specs an import would replace
// and what a real run does with them.
func printImportOverwrites(w io.Writer, entries []importPreviewEntry, overwrite bool) {
	for _, e := range entries {
		if overwrite {
			_, _ = fmt.Fprintf(w, "  ! replaces %s, which holds different content (%s)\n", e.path, strings.Join(e.sources, ", "))
			continue
		}
		_, _ = fmt.Fprintf(w, "  ! %s already holds different content (%s); the import stops unless --overwrite\n", e.path, strings.Join(e.sources, ", "))
	}
}

// stopOnImportOverwrites runs run in a quiet copy of the project and
// returns an error listing each existing spec it would replace with
// different content, ending with remedy for the sources that wrote them.
// Only files there before the run count, so a later source in the same
// run still replaces what an earlier one wrote.
func stopOnImportOverwrites(run func() error, remedy func(sources []string) string) error {
	if importSandbox != "" || !holdsSpecs(".") {
		return nil
	}
	var overwrites []importPreviewEntry
	err := quietImport(func() error {
		// An importer error is left to the real run, which reports it.
		ignoreRunErr := func() error { _ = run(); return nil }
		_, err := runImportInCopy(ignoreRunErr, nil, func(project, shadow string, rec *importRecorder) error {
			preview, err := buildImportPreview(project, shadow, rec)
			overwrites = preview.overwrites()
			return err
		})
		return err
	})
	if err != nil || len(overwrites) == 0 {
		return err
	}
	var b strings.Builder
	var sources []string
	fmt.Fprintf(&b, "import would replace %d existing spec(s) with different content, so nothing was written:\n", len(overwrites))
	for _, e := range overwrites {
		fmt.Fprintf(&b, "  %s (from %s)\n", e.path, strings.Join(e.sources, ", "))
		for _, s := range e.sources {
			if !slices.Contains(sources, s) {
				sources = append(sources, s)
			}
		}
	}
	b.WriteString(remedy(sources))
	return errors.New(b.String())
}

// importOverwriteRemedy names the two ways past an import that would
// replace existing specs, for the import command args.
func importOverwriteRemedy(args []string) string {
	return fmt.Sprintf("rename a spec to keep both, or run agnostic-ai import %s --overwrite to replace them", strings.Join(args, " "))
}

// holdsSpecs reports whether the spec directories under root hold any
// file besides AGNOSTIC_AI.md, which import merges into rather than
// replaces.
func holdsSpecs(root string) bool {
	main := filepath.Join(root, filepath.FromSlash(agnosticMainFile))
	for _, dir := range importSpecDirs(root) {
		found := errors.New("found")
		err := filepath.WalkDir(filepath.Join(root, filepath.FromSlash(dir)), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if !d.IsDir() && filepath.Clean(path) != filepath.Clean(main) {
				return found
			}
			return nil
		})
		if err == found {
			return true
		}
	}
	return false
}
