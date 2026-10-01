package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
)

// importScopedSkillFolders discovers the target's native skill directory at
// the repository root and below every project subdirectory. The prefix before
// the native directory becomes the canonical skill scope.
type scopedSkillDir struct {
	path  string
	scope string
}

func findScopedSkillDirs(root, nativeDir string) ([]scopedSkillDir, error) {
	nativeDir = filepath.ToSlash(filepath.Clean(nativeDir))
	tree := importTreeFor(root)
	var found []scopedSkillDir
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() {
			return nil
		}
		if path != root && (entry.Name() == ".git" || entry.Name() == ".agnostic-ai") {
			return filepath.SkipDir
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if !onNativePath(rel, nativeDir) && tree.skipsDir(rel) {
			return filepath.SkipDir
		}
		if rel != nativeDir && !strings.HasSuffix(rel, "/"+nativeDir) {
			return nil
		}
		scope := strings.TrimSuffix(rel, nativeDir)
		scope = strings.TrimSuffix(scope, "/")
		found = append(found, scopedSkillDir{path: path, scope: scope})
		return filepath.SkipDir
	})
	if err != nil {
		return nil, fmt.Errorf("discover scoped skills under %s: %w", root, err)
	}
	return found, nil
}

func importScopedSkillFolders(root, nativeDir, dstDir string) (int, error) {
	return importScopedSkillFoldersFrom(root, []string{nativeDir}, dstDir)
}

// importScopedSkillFoldersFrom scans every directory in nativeDirs, at
// the repository root and below every project subdirectory. Earlier
// entries win: when the same skill name appears at the same scope under
// two of them, the first is imported and the rest are skipped. Each
// target lists its own emit path first so an emit then import reads back
// the tree the adapter wrote. Precedence is tracked per scope, since the
// same name at two different scopes is two different skills.
func importScopedSkillFoldersFrom(root string, nativeDirs []string, dstDir string) (int, error) {
	return importScopedSkillFoldersWith(root, nativeDirs, dstDir, nil)
}

// importScopedSkillFoldersWith is importScopedSkillFoldersFrom with the
// target's own SKILL.md field set.
func importScopedSkillFoldersWith(root string, nativeDirs []string, dstDir string, fields *specFields) (int, error) {
	var dirs []scopedSkillDir
	for _, nativeDir := range nativeDirs {
		found, err := findScopedSkillDirs(root, nativeDir)
		if err != nil {
			return 0, err
		}
		dirs = append(dirs, found...)
	}
	rootScopeFirst(dirs)
	count := 0
	seen := map[string]map[string]bool{}
	folders := skillFolderClaims{}
	for _, dir := range dirs {
		if seen[dir.scope] == nil {
			seen[dir.scope] = map[string]bool{}
		}
		imported, err := importSkillFoldersWith(
			root,
			dir.path,
			filepath.Join(dstDir, filepath.FromSlash(dir.scope)),
			skillFolderImportOpts{SkipNames: seen[dir.scope], Fields: fields, Folders: folders, Scope: dir.scope},
		)
		if err != nil {
			return count, err
		}
		count += imported
	}
	return count, folders.recordWorkspaces()
}

// rootScopeFirst moves the root-scope skill directories ahead of the
// scoped ones, keeping their order otherwise, so a root link to a
// package's skill folder claims that folder before the package does.
func rootScopeFirst(dirs []scopedSkillDir) {
	sort.SliceStable(dirs, func(i, j int) bool {
		return dirs[i].scope == "" && dirs[j].scope != ""
	})
}

// skillFolderClaims maps the resolved path of each imported skill folder
// to where it was imported from and to, so a folder reached through a
// link and through its own location becomes one spec.
type skillFolderClaims map[string]*skillFolderClaim

// skillFolderClaim is one imported skill folder: the native path kept,
// its scope, the spec folder it became, and the scopes of the other
// native paths that reach the same folder.
type skillFolderClaim struct {
	at, scope, dst string
	workspaces     []string
}

// claim reports whether the skill folder at src, found at entry in scope,
// is new to this import. A folder already imported through another path
// is skipped with a note naming the path that was kept. When a root link
// is kept and the real folder sits in a scope, as when a project links
// `.cursor/skills/<name>` to `<dir>/.cursor/skills/<name>`, the scope is
// recorded so the spec keeps the skill in that workspace too.
func (c skillFolderClaims) claim(root, entry, src, dst, scope string) bool {
	if c == nil {
		return true
	}
	abs, err := filepath.Abs(src)
	if err != nil {
		return true
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return true
	}
	at := entry
	if rel, err := filepath.Rel(root, entry); err == nil {
		at = filepath.ToSlash(rel)
	}
	if kept, ok := c[real]; ok {
		summaryf("  ! skipped %s: kept %s, which is the same skill folder\n", at, kept.at)
		if kept.scope == "" && scope != "" && !slices.Contains(kept.workspaces, scope) {
			kept.workspaces = append(kept.workspaces, scope)
		}
		return false
	}
	c[real] = &skillFolderClaim{at: at, scope: scope, dst: dst}
	return true
}

// recordWorkspaces adds `workspaces:` to each imported root skill spec
// that a scoped native folder also held, so Cursor, which loads skills
// only from the workspace it opens, still finds it there.
func (c skillFolderClaims) recordWorkspaces() error {
	for _, k := range c {
		if len(k.workspaces) == 0 {
			continue
		}
		path := filepath.Join(k.dst, "SKILL.md")
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(data)
		end := strings.Index(text, "\n---")
		if !strings.HasPrefix(text, "---\n") || end < 0 {
			continue
		}
		line := "\nworkspaces: [" + strings.Join(k.workspaces, ", ") + "]"
		if err := importWriteFile(path, []byte(text[:end]+line+text[end:]), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// importSkillFolders copies each `<srcDir>/<name>/` directory tree that
// contains a SKILL.md into `<dstDir>/<name>/`, so a round-trip preserves
// the full payload (scripts, references, assets). Bundled files land
// byte-for-byte; SKILL.md merges onto the spec already there.
// Folders without a SKILL.md are skipped; a missing srcDir imports
// nothing. Shared by every importer whose tool uses the Agent Skills
// folder layout (cursor, gemini, opencode, copilot).
func importSkillFolders(root, srcDir, dstDir string) (int, error) {
	return importSkillFoldersWith(root, srcDir, dstDir, skillFolderImportOpts{})
}

type skillFolderImportOpts struct {
	SkipNames      map[string]bool
	TransformSkill func([]byte) ([]byte, error)
	// Fields overrides what the target's SKILL.md can hold. Nil means
	// the Agent Skills baseline every target but Claude and Cursor writes.
	Fields *specFields
	// Folders dedupes skill folders across directories by resolved path.
	Folders skillFolderClaims
	// Scope is the project directory srcDir's native tree sits in.
	Scope string
}

// importSkillFoldersWith imports a native skill tree with optional
// collision handling and SKILL.md normalization. Candidate lists share a
// SkipNames map so the documented path order becomes explicit precedence
// without treating a pre-existing destination spec as a native collision.
func importSkillFoldersWith(root, srcDir, dstDir string, opts skillFolderImportOpts) (int, error) {
	entries, err := os.ReadDir(srcDir)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", srcDir, err)
	}
	count := 0
	for _, e := range entries {
		if opts.SkipNames[e.Name()] {
			continue
		}
		skillSrc, ok := skillFolderSource(root, srcDir, dstDir, e)
		if !ok {
			continue
		}
		skillDst := filepath.Join(dstDir, e.Name())
		if _, err := os.Stat(filepath.Join(skillSrc, "SKILL.md")); errors.Is(err, fs.ErrNotExist) {
			continue
		} else if err != nil {
			return count, fmt.Errorf("stat skill %s: %w", e.Name(), err)
		}
		if !opts.Folders.claim(root, filepath.Join(srcDir, e.Name()), skillSrc, skillDst, opts.Scope) {
			continue
		}
		if err := copyDirTreeWith(skillSrc, skillDst, opts.TransformSkill, skillFields(opts.Fields)); err != nil {
			return count, fmt.Errorf("copy skill %s: %w", e.Name(), err)
		}
		if opts.SkipNames != nil {
			opts.SkipNames[e.Name()] = true
		}
		count++
	}
	return count, nil
}

// skillFolderSource returns the folder to import for the entry e of the
// native skills directory dir. A symlink to a directory inside root
// imports like a real folder, so a package can keep its own skills and
// the tool folder link to them. A skill linked from outside root is
// skipped with a note naming the link, as is a folder an import preview
// copied from such a link. A link into the skill sources, or to a folder
// holding them, imports nothing: the skill is already a spec, and
// copying a folder into itself never ends.
func skillFolderSource(root, dir, dstDir string, e fs.DirEntry) (string, bool) {
	link := filepath.Join(dir, e.Name())
	abs, err := filepath.Abs(link)
	if err != nil {
		return "", false
	}
	if e.Type()&fs.ModeSymlink == 0 {
		if !e.IsDir() {
			return "", false
		}
		if copiedFromOutside(root, abs) {
			noteOutsideSkill(link, abs)
			return "", false
		}
		return link, true
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", false
	}
	if info, err := os.Stat(resolved); err != nil || !info.IsDir() {
		return "", false
	}
	if _, inside := resolvedInside(root, resolved); !inside {
		noteOutsideSkill(link, resolved)
		return "", false
	}
	if dst, err := filepath.Abs(dstDir); err == nil {
		dst = resolveExisting(dst)
		if _, overlaps := resolvedInside(resolved, dst); overlaps {
			return "", false
		}
		if _, overlaps := resolvedInside(dst, resolved); overlaps {
			return "", false
		}
	}
	return resolved, true
}

// resolveExisting resolves the symlinks of abs, an absolute path whose
// last parts may not exist yet, through its nearest existing parent.
func resolveExisting(abs string) string {
	rest := ""
	for p := abs; ; p = filepath.Dir(p) {
		if real, err := filepath.EvalSymlinks(p); err == nil {
			return filepath.Join(real, rest)
		}
		if filepath.Dir(p) == p {
			return abs
		}
		rest = filepath.Join(filepath.Base(p), rest)
	}
}

// copiedFromOutside reports whether an import preview copied the
// directory abs from a link that leaves the project.
func copiedFromOutside(root, abs string) bool {
	if importSandboxOutsideFiles == nil {
		return false
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(absRoot, abs)
	return err == nil && importSandboxOutsideFiles[rel]
}

// noteOutsideSkill names a skill folder link the import skips because it
// leaves the project. A linked folder without a SKILL.md is no skill.
func noteOutsideSkill(link, target string) {
	if fileExists(filepath.Join(target, "SKILL.md")) {
		summaryf("  ! skipped %s: the skill folder links outside the project\n", link)
	}
}

// copyDirTree walks srcDir recursively and writes every regular file
// into the matching location under dstDir, byte-for-byte but for a
// SKILL.md merged onto an existing spec, recreating
// the directory layout as it goes. File mode bits are preserved so an
// executable script remains executable on the destination. Symlinks
// are not followed; if they appear inside a skill folder they are
// silently skipped (skills are documented to be plain files +
// directories — symlinks would not survive a tar/zip release anyway).
func copyDirTree(srcDir, dstDir string) error {
	return copyDirTreeWith(srcDir, dstDir, nil, allSpecFields)
}

// skillFields resolves a skill tree's field set, defaulting to the
// Agent Skills baseline when the importer names none.
func skillFields(override *specFields) specFields {
	if override == nil {
		return defaultSkillFields
	}
	return *override
}

// isSyncBackup reports whether path is the `<file>.bak` sync keeps of a
// hand edit: file is an output the ledger records, so the copy is sync's,
// not an asset of the skill.
func isSyncBackup(path string) bool {
	file, ok := strings.CutSuffix(path, ".bak")
	if !ok {
		return false
	}
	_, ledgered := readStateFile(".").OutputSums[filepath.ToSlash(filepath.Clean(file))]
	return ledgered
}

func copyDirTreeWith(srcDir, dstDir string, transformSkill func([]byte) ([]byte, error), fields specFields) error {
	return filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dstDir, rel)
		if d.IsDir() {
			return importMkdirAll(target, 0o755)
		}
		if !d.Type().IsRegular() || isSyncBackup(path) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		// Strip the agnostic-ai provenance header from SKILL.md so a
		// roundtrip (claude / codex emit -> import) does not bake the
		// header into the source spec. Sibling assets pass through
		// byte-for-byte because they are user-authored.
		if filepath.Base(path) == "SKILL.md" {
			data = []byte(header.Strip(string(data)))
			if transformSkill != nil {
				data, err = transformSkill(data)
				if err != nil {
					return fmt.Errorf("transform %s: %w", path, err)
				}
			}
		}
		if err := importMkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", filepath.Dir(target), err)
		}
		if filepath.Base(path) == "SKILL.md" {
			if err := importWriteSpecMarkdown(target, data, info.Mode().Perm(), fields); err != nil {
				return fmt.Errorf("write %s: %w", target, err)
			}
			return nil
		}
		if err := importWriteFile(target, data, info.Mode().Perm()); err != nil {
			return fmt.Errorf("write %s: %w", target, err)
		}
		return nil
	})
}
