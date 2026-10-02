package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

// importPreviewDirPrefix names the temporary project copy an import
// preview runs in. Cleanup refuses any directory without it.
const importPreviewDirPrefix = "agnostic-ai-import-preview-"

// importPreviewEntry is one destination an import would write.
type importPreviewEntry struct {
	path     string   // slash-form, relative to the project root
	existed  bool     // the destination exists before the import
	before   []byte   // current bytes, nil when !existed
	after    []byte   // bytes the sequential import leaves
	sources  []string // sources that wrote it, in import order
	conflict bool     // two or more sources proposed different bytes
	winner   string   // source whose write a real import keeps
	replaced bool     // a write replaced the file instead of merging into it
	// overwrites marks an existing spec whose replacement stops a real
	// import without --overwrite (see replacesSpec).
	overwrites bool
	// heldBy names, as "<sync|import>:<tools>", where the current bytes
	// came from, when a record still matches them.
	heldBy string
}

// importPreview is the plan an `import --dry-run --diff` run reports.
type importPreview struct {
	order     []string
	entries   []importPreviewEntry // sorted by path
	overwrite bool                 // the import runs with --overwrite
}

// previewImport runs the import preview for args and prints the report.
// An importer failure still prints what the other sources planned, then
// returns the error, as a real multi-source import does.
func previewImport(args []string, overwrite bool) error {
	preview, err := planImportPreview(args)
	preview.overwrite = overwrite
	printImportPreview(os.Stdout, preview)
	if overwrites := preview.overwrites(); len(overwrites) > 0 && !overwrite {
		return errors.Join(err, importOverwriteError(overwrites, func([]string) string { return importOverwriteRemedy(args) }))
	}
	return err
}

// dryRunImport runs the import for args in a copy of the project and
// prints a planning summary instead of file contents: one line per path
// the importer would write, sorted and listed once however many stages
// write it, ending with a count. Equivalent in shape to `sync --plan`.
// The summary prints even when an importer fails. prepare, when set,
// runs in the copy before the import. Existing specs the import would
// replace fail the preview as they fail a real run without overwrite.
func dryRunImport(args []string, prepare func() error, overwrite bool) error {
	var overwrites []importPreviewEntry
	rec, err := runImportInCopy(func() error { return importArgs(args) }, prepare, func(project, shadow string, rec *importRecorder) error {
		preview, err := buildImportPreview(project, shadow, rec)
		overwrites = preview.overwrites()
		return err
	})
	paths := make([]string, 0, len(rec.writes))
	for _, w := range rec.writes {
		paths = append(paths, filepath.FromSlash(w.path))
	}
	sort.Strings(paths)
	paths = slices.Compact(paths)
	for _, p := range paths {
		fmt.Printf("  would write %s\n", p)
	}
	if len(overwrites) > 0 && !overwrite {
		return errors.Join(err, importOverwriteError(overwrites, func([]string) string { return importOverwriteRemedy(args) }))
	}
	printImportOverwrites(os.Stdout, overwrites)
	fmt.Printf("dry-run: %d file(s) would be written\n", len(paths))
	return err
}

// planImportPreview runs the import in a copy of the project and
// compares the result with the project.
func planImportPreview(args []string) (importPreview, error) {
	var preview importPreview
	_, err := runImportInCopy(func() error { return importArgs(args) }, nil, func(project, shadow string, rec *importRecorder) error {
		var err error
		preview, err = buildImportPreview(project, shadow, rec)
		return err
	})
	return preview, err
}

// runImportInCopy copies the working directory (without .git) into a
// temporary directory, runs the import there with every write
// recorded, and calls inspect, when set, before the copy is removed. Running the
// ordinary import is what keeps a dry-run equal to a real one: a later
// stage reads what an earlier one wrote, frontmatter merges and fences
// included. The project itself is never written. An inspect error wins
// over an importer error. prepare, when set, runs in the copy first,
// such as the scaffold `init --from` writes before it imports.
func runImportInCopy(run func() error, prepare func() error, inspect func(project, shadow string, rec *importRecorder) error) (*importRecorder, error) {
	rec := &importRecorder{}
	project, err := os.Getwd()
	if err != nil {
		return rec, fmt.Errorf("getwd: %w", err)
	}
	tmp, err := os.MkdirTemp("", importPreviewDirPrefix)
	if err != nil {
		return rec, fmt.Errorf("create preview dir: %w", err)
	}
	defer removeImportPreviewDir(tmp)
	// The copy keeps the project's directory name: codex names the rule
	// it shreds from a root AGENTS.md after it.
	shadow := filepath.Join(tmp, filepath.Base(project))
	if err := os.Mkdir(shadow, 0o700); err != nil {
		return rec, fmt.Errorf("%s: %w", shadow, err)
	}
	tree := loadImportTree(project)
	copied, err := copyImportPreviewTree(project, shadow, tree, importPreviewKeeps(project))
	if err != nil {
		return rec, fmt.Errorf("copy project for preview: %w", err)
	}

	// Deferred after the cleanup above, so it runs first: the copy is
	// left before it is removed.
	if err := os.Chdir(shadow); err != nil {
		return rec, fmt.Errorf("%s: %w", shadow, err)
	}
	defer func() { _ = os.Chdir(project) }()
	if prepare != nil {
		if err := prepare(); err != nil {
			return rec, err
		}
	}
	// Read back through the working directory, as filepath.Abs does, so
	// the sandbox check compares like with like.
	sandbox, err := os.Getwd()
	if err != nil {
		return rec, fmt.Errorf("getwd: %w", err)
	}
	// The copy has no .git to ask, so the walks there leave out what
	// they would leave out in the project.
	tree.root, tree.nested = sandbox, copied.nested
	importRecording, importSandbox, importSandboxOutsideFiles, importRunTree = rec, sandbox, copied.outsideFiles, &tree
	defer func() {
		importRecording, importSandbox, importSandboxOutsideFiles, importRunTree = nil, "", nil, nil
	}()

	runErr := run()
	if inspect != nil {
		if err := inspect(project, shadow, rec); err != nil {
			return rec, err
		}
	}
	return rec, runErr
}

// buildImportPreview folds the recorded writes into one entry per
// destination. The final bytes come from the preview copy, the current
// bytes from the project.
func buildImportPreview(project, shadow string, rec *importRecorder) (importPreview, error) {
	preview := importPreview{order: rec.order}
	byPath := map[string]*importPreviewEntry{}
	proposals := map[string]map[string][]byte{} // path -> source -> last bytes
	for _, w := range rec.writes {
		e, ok := byPath[w.path]
		if !ok {
			e = &importPreviewEntry{path: w.path}
			byPath[w.path] = e
			proposals[w.path] = map[string][]byte{}
		}
		if _, seen := proposals[w.path][w.source]; !seen {
			e.sources = append(e.sources, w.source)
		}
		proposals[w.path][w.source] = w.data
		e.winner = w.source
		e.replaced = e.replaced || !w.merge
	}
	specDirs := importSpecDirs(shadow)
	sums := readStateFile(project).SpecFileSums
	for path, e := range byPath {
		native := filepath.FromSlash(path)
		after, err := os.ReadFile(filepath.Join(shadow, native))
		if err != nil {
			return importPreview{}, fmt.Errorf("%s: %w", path, err)
		}
		e.after = after
		before, err := os.ReadFile(filepath.Join(project, native))
		switch {
		case err == nil:
			e.existed, e.before = true, before
		case !errors.Is(err, fs.ErrNotExist):
			return importPreview{}, fmt.Errorf("%s: %w", path, err)
		}
		e.conflict = distinctProposals(proposals[path]) > 1
		e.overwrites = replacesSpec(e, specDirs, sums)
		preview.entries = append(preview.entries, *e)
	}
	sort.Slice(preview.entries, func(i, j int) bool {
		return preview.entries[i].path < preview.entries[j].path
	})
	return preview, nil
}

// distinctProposals counts the different byte sequences sources proposed.
func distinctProposals(bySource map[string][]byte) int {
	seen := map[string]bool{}
	for _, data := range bySource {
		seen[string(data)] = true
	}
	return len(seen)
}

// status names what the import does to the destination.
func (e importPreviewEntry) status() string {
	switch {
	case !e.existed:
		return "create"
	case bytes.Equal(e.before, e.after):
		return "unchanged"
	default:
		return "change"
	}
}

// printImportPreview writes the report: import order, one line per
// destination with its writers, the conflicts, a diff per created or
// changed destination, and a closing count.
func printImportPreview(w io.Writer, p importPreview) {
	numbered := make([]string, len(p.order))
	for i, s := range p.order {
		numbered[i] = fmt.Sprintf("%d. %s", i+1, s)
	}
	_, _ = fmt.Fprintf(w, "import order: %s\n", strings.Join(numbered, ", "))

	counts := map[string]int{}
	conflicts := 0
	for _, e := range p.entries {
		counts[e.status()]++
		_, _ = fmt.Fprintf(w, "  %-9s %s  (%s)\n", e.status(), e.path, strings.Join(e.sources, ", "))
	}
	for _, e := range p.entries {
		if !e.conflict {
			continue
		}
		conflicts++
		_, _ = fmt.Fprintf(w, "  ! conflict %s: %s propose different content; %s (last) is kept\n",
			e.path, strings.Join(e.sources, ", "), e.winner)
	}
	if p.overwrite {
		printImportOverwrites(w, p.overwrites())
	}
	for _, e := range p.entries {
		if e.status() != "unchanged" {
			_, _ = fmt.Fprint(w, importPreviewDiff(e))
		}
	}
	_, _ = fmt.Fprintf(w, "dry-run: %d file(s) would be written (%d created, %d changed, %d unchanged), %d conflict(s); nothing was written\n",
		len(p.entries), counts["create"], counts["change"], counts["unchanged"], conflicts)
}

// importPreviewDiff renders one destination's change. Binary content
// gets a size line instead of a diff.
func importPreviewDiff(e importPreviewEntry) string {
	if isBinary(e.before) || isBinary(e.after) {
		return fmt.Sprintf("binary %s: %d -> %d bytes\n", e.path, len(e.before), len(e.after))
	}
	afterLabel := e.path + " (after import)"
	if !e.existed {
		return labeledDiff("/dev/null", afterLabel, nil, splitLines(string(e.after)), diffBodyMax)
	}
	return labeledDiff(e.path+" (current)", afterLabel,
		splitLines(string(e.before)), splitLines(string(e.after)), diffBodyMax)
}

// isBinary reports whether data is not printable as text.
func isBinary(data []byte) bool {
	return bytes.IndexByte(data, 0) >= 0 || !utf8.Valid(data)
}

// previewCopy is what copyImportPreviewTree reports about a copy.
type previewCopy struct {
	// outsideFiles holds the copied files and directories that came from
	// outside the project, relative to the copy.
	outsideFiles map[string]bool
	// nested holds the directories, slash-form and relative to the copy,
	// whose own .git entry the copy dropped.
	nested map[string]bool
}

// copyImportPreviewTree copies the project at src into dst for a preview
// run. It skips .git and the directories leavesOut names, so a large
// node_modules does not slow the preview down. A symlink that
// resolves inside the project is recreated as a relative link, so the copy
// keeps the same shape; one that resolves outside is copied by content, so
// no preview write can reach a file outside the copy. Dangling links,
// sockets, and devices are skipped.
func copyImportPreviewTree(src, dst string, tree importTree, keep previewKeep) (previewCopy, error) {
	root, err := filepath.EvalSymlinks(src)
	if err != nil {
		return previewCopy{}, fmt.Errorf("%s: %w", src, err)
	}
	c := previewCopier{
		root: root, dstRoot: dst, tree: tree, keep: keep,
		visited: map[string]bool{}, outsideFiles: map[string]bool{}, nested: map[string]bool{},
	}
	return previewCopy{outsideFiles: c.outsideFiles, nested: c.nested}, c.copyDir(root, dst)
}

// previewKeep names what the preview copy keeps whole even when git
// ignores it, because an importer reads it by name.
type previewKeep struct {
	// paths are project-relative: the root tool folders, each target's
	// native paths under the project config, and the source directories.
	paths []string
	// toolDirs are tool folder names such as `.cursor`, kept at any depth
	// because sync writes scoped output into them.
	toolDirs map[string]bool
}

// importPreviewKeeps builds the previewKeep of the project.
func importPreviewKeeps(project string) previewKeep {
	cfg, err := config.Load(project)
	if err != nil {
		cfg = &config.Config{}
	}
	paths := map[string]bool{}
	keep := previewKeep{toolDirs: map[string]bool{}}
	add := func(p string) {
		if p == "" || filepath.IsAbs(p) {
			return
		}
		p = strings.TrimSuffix(filepath.ToSlash(filepath.Clean(p)), "/")
		if p == "." || p == ".." || strings.HasPrefix(p, "../") || strings.ContainsAny(p, "<>*?[{~$") {
			return
		}
		paths[p] = true
	}
	tool := func(p string) {
		if dir := firstSegment(p); strings.HasPrefix(dir, ".") {
			keep.toolDirs[dir] = true
			add(dir)
		}
	}
	add(config.SourceBaseDir)
	for _, markers := range targetMarkers {
		for _, m := range markers {
			tool(m)
		}
	}
	s := cfg.Sources
	for _, p := range []string{s.Agents, s.Skills, s.Rules, s.Hooks, s.MCPs, s.Commands, s.Settings, s.Reviews, s.Environments, s.Ignore} {
		add(p)
	}
	for _, t := range allTargets {
		for _, a := range adapters.NativeArtifactsFor(t.Name, cfg) {
			tool(a.Location)
			add(a.Location)
		}
	}
	keep.paths = slices.Sorted(maps.Keys(paths))
	return keep
}

// previewCopier holds the state of one copyImportPreviewTree call.
// visited holds the directories one project link copies by content: the
// link's target, and each directory a link inside that copy reaches,
// followed once so a cycle stops and a web of package links stays small.
// Every project link starts a fresh set.
// detached is set while copying a linked directory by content, whose
// paths are not the project's; outside, while that directory lies
// outside the project.
type previewCopier struct {
	root         string
	dstRoot      string
	tree         importTree
	keep         previewKeep
	visited      map[string]bool
	detached     bool
	outside      bool
	outsideFiles map[string]bool
	nested       map[string]bool
}

func (c previewCopier) copyDir(from, to string) error {
	return filepath.WalkDir(from, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(from, path)
		if err != nil {
			return err
		}
		target := filepath.Join(to, rel)
		if d.Name() == ".git" && path != from {
			c.noteNested(filepath.Dir(target))
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		switch {
		case d.Type()&fs.ModeSymlink != 0:
			return c.copySymlink(path, target)
		case d.IsDir():
			if rel == "." {
				return nil
			}
			if (c.detached && isPackagesDir(d.Name())) || (!c.detached && c.leavesOut(filepath.ToSlash(rel))) {
				return filepath.SkipDir
			}
			return os.Mkdir(target, info.Mode().Perm()|0o700)
		case d.Type().IsRegular():
			if c.outside {
				c.noteOutside(target)
			}
			return copyPreviewFile(path, target, info.Mode().Perm())
		}
		return nil
	})
}

// leavesOut reports whether the copy skips the project directory rel:
// what the import walks skip (see importTree). A kept path, the
// directories on the way to it, and a tool folder at any depth stay
// even when git ignores them; inside one, only node_modules and nested
// repositories are left out.
func (c previewCopier) leavesOut(rel string) bool {
	segs := strings.Split(rel, "/")
	name := segs[len(segs)-1]
	if isPackagesDir(name) {
		return true
	}
	if c.keep.toolDirs[name] {
		return false
	}
	for _, k := range c.keep.paths {
		if rel == k || strings.HasPrefix(k, rel+"/") {
			return false
		}
	}
	for _, k := range c.keep.paths {
		if strings.HasPrefix(rel, k+"/") {
			return c.tree.isRepo(rel)
		}
	}
	if slices.ContainsFunc(segs[:len(segs)-1], func(s string) bool { return c.keep.toolDirs[s] }) {
		return c.tree.isRepo(rel)
	}
	return c.tree.skipsDir(rel)
}

func (c previewCopier) noteNested(dir string) {
	if c.detached {
		return
	}
	if rel, err := filepath.Rel(c.dstRoot, dir); err == nil && rel != "." {
		c.nested[filepath.ToSlash(rel)] = true
	}
}

func (c previewCopier) noteOutside(target string) {
	if rel, err := filepath.Rel(c.dstRoot, target); err == nil {
		c.outsideFiles[rel] = true
	}
}

// copySymlink recreates a link inside the project as a relative link, so
// the copy keeps the same shape. A link that leaves the project, or whose
// target the copy leaves out, is copied by content instead, so an import
// reading through it finds the same files.
func (c previewCopier) copySymlink(link, target string) error {
	resolved, err := filepath.EvalSymlinks(link)
	if err != nil {
		return nil // dangling: an import reading it finds nothing either
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return fmt.Errorf("%s: %w", link, err)
	}
	inside, err := filepath.Rel(c.root, resolved)
	outside := err != nil || inside == ".." || strings.HasPrefix(inside, ".."+string(filepath.Separator))
	if !outside && !c.targetLeftOut(filepath.ToSlash(inside), info.IsDir()) {
		rel, err := filepath.Rel(filepath.Dir(target), filepath.Join(c.dstRoot, inside))
		if err != nil {
			return fmt.Errorf("%s: %w", link, err)
		}
		return os.Symlink(rel, target)
	}
	if !info.IsDir() {
		if outside {
			c.noteOutside(target)
		}
		return copyPreviewFile(resolved, target, info.Mode().Perm())
	}
	if c.detached {
		if c.visited[resolved] {
			return nil
		}
		c.visited[resolved] = true
	} else {
		c.visited = map[string]bool{resolved: true}
	}
	if err := os.Mkdir(target, info.Mode().Perm()|0o700); err != nil {
		return fmt.Errorf("%s: %w", target, err)
	}
	if outside {
		c.noteOutside(target)
	}
	c.detached, c.outside = true, c.outside || outside
	return c.copyDir(resolved, target)
}

// targetLeftOut reports whether the copy leaves out the project path rel
// a link points at, or a directory above it.
func (c previewCopier) targetLeftOut(rel string, isDir bool) bool {
	dir := rel
	if !isDir {
		dir = path.Dir(rel)
	}
	if dir == "." {
		return false
	}
	segs := strings.Split(dir, "/")
	for i := range segs {
		if c.leavesOut(strings.Join(segs[:i+1], "/")) {
			return true
		}
	}
	return false
}

func copyPreviewFile(from, to string, perm fs.FileMode) error {
	data, err := os.ReadFile(from)
	if err != nil {
		return fmt.Errorf("%s: %w", from, err)
	}
	if err := os.WriteFile(to, data, perm|0o200); err != nil {
		return fmt.Errorf("%s: %w", to, err)
	}
	return nil
}

// removeImportPreviewDir deletes a preview copy. It refuses any path that
// is not a prefixed directory directly under the system temp dir, so a
// bad value can never widen the delete.
func removeImportPreviewDir(dir string) {
	tmp := filepath.Clean(os.TempDir())
	if dir == "" || filepath.Dir(filepath.Clean(dir)) != tmp ||
		!strings.HasPrefix(filepath.Base(dir), importPreviewDirPrefix) {
		return
	}
	_ = os.RemoveAll(dir)
}
