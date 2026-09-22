package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
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
}

// importPreview is the plan an `import --dry-run --diff` run reports.
type importPreview struct {
	order   []string
	entries []importPreviewEntry // sorted by path
}

// previewImport runs the import preview for args and prints the report.
// An importer failure still prints what the other sources planned, then
// returns the error, as a real multi-source import does.
func previewImport(args []string) error {
	preview, err := planImportPreview(args)
	printImportPreview(os.Stdout, preview)
	return err
}

// planImportPreview copies the working directory (without .git) into a
// temporary directory, runs the real importers there with every write
// recorded, and compares the result with the project. Running the
// ordinary import is what keeps the planned bytes equal to a real one:
// a later source reads what an earlier one wrote, frontmatter merges
// and fences included. The project itself is never written.
func planImportPreview(args []string) (importPreview, error) {
	project, err := os.Getwd()
	if err != nil {
		return importPreview{}, fmt.Errorf("getwd: %w", err)
	}
	tmp, err := os.MkdirTemp("", importPreviewDirPrefix)
	if err != nil {
		return importPreview{}, fmt.Errorf("create preview dir: %w", err)
	}
	defer removeImportPreviewDir(tmp)
	// The copy keeps the project's directory name: codex names the rule
	// it shreds from a root AGENTS.md after it.
	shadow := filepath.Join(tmp, filepath.Base(project))
	if err := os.Mkdir(shadow, 0o700); err != nil {
		return importPreview{}, fmt.Errorf("%s: %w", shadow, err)
	}
	if err := copyImportPreviewTree(project, shadow); err != nil {
		return importPreview{}, fmt.Errorf("copy project for preview: %w", err)
	}

	rec, runErr := recordImportIn(shadow, args)
	if err := os.Chdir(project); err != nil {
		return importPreview{}, fmt.Errorf("%s: %w", project, err)
	}
	preview, err := buildImportPreview(project, shadow, rec)
	if err != nil {
		return importPreview{}, err
	}
	return preview, runErr
}

// recordImportIn runs the import of args with dir as working directory
// and a recorder attached. The caller restores the working directory.
func recordImportIn(dir string, args []string) (*importRecorder, error) {
	if err := os.Chdir(dir); err != nil {
		return nil, fmt.Errorf("%s: %w", dir, err)
	}
	rec := &importRecorder{}
	importRecording = rec
	defer func() { importRecording = nil }()
	return rec, runImportArgs(args)
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
	}
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

// copyImportPreviewTree copies the project at src into dst for a preview
// run. It skips .git, which no importer reads. A symlink that resolves
// inside the project is recreated as a relative link, so the copy keeps
// the same shape; one that resolves outside is copied by content, so no
// preview write can reach a file outside the copy. Dangling links, sockets,
// and devices are skipped.
func copyImportPreviewTree(src, dst string) error {
	root, err := filepath.EvalSymlinks(src)
	if err != nil {
		return fmt.Errorf("%s: %w", src, err)
	}
	c := previewCopier{root: root, dstRoot: dst, visited: map[string]bool{}}
	return c.copyDir(root, dst)
}

// previewCopier holds the state of one copyImportPreviewTree call.
// visited guards against a cycle of symlinks to directories outside the
// project.
type previewCopier struct {
	root    string
	dstRoot string
	visited map[string]bool
}

func (c previewCopier) copyDir(from, to string) error {
	return filepath.WalkDir(from, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Name() == ".git" && path != from {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(from, path)
		if err != nil {
			return err
		}
		target := filepath.Join(to, rel)
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
			return os.Mkdir(target, info.Mode().Perm()|0o700)
		case d.Type().IsRegular():
			return copyPreviewFile(path, target, info.Mode().Perm())
		}
		return nil
	})
}

func (c previewCopier) copySymlink(link, target string) error {
	resolved, err := filepath.EvalSymlinks(link)
	if err != nil {
		return nil // dangling: an import reading it finds nothing either
	}
	if inside, err := filepath.Rel(c.root, resolved); err == nil && inside != ".." &&
		!strings.HasPrefix(inside, ".."+string(filepath.Separator)) {
		rel, err := filepath.Rel(filepath.Dir(target), filepath.Join(c.dstRoot, inside))
		if err != nil {
			return fmt.Errorf("%s: %w", link, err)
		}
		return os.Symlink(rel, target)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return fmt.Errorf("%s: %w", link, err)
	}
	if !info.IsDir() {
		return copyPreviewFile(resolved, target, info.Mode().Perm())
	}
	if c.visited[resolved] {
		return nil
	}
	c.visited[resolved] = true
	if err := os.Mkdir(target, info.Mode().Perm()|0o700); err != nil {
		return fmt.Errorf("%s: %w", target, err)
	}
	return c.copyDir(resolved, target)
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
