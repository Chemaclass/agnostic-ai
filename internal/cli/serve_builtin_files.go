package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

type builtinProjectSnapshot struct {
	roots []string
	files map[string]editFileSnapshot
	dirs  map[string]os.FileInfo
}

func snapshotBuiltinProject(root string, roots []string) (builtinProjectSnapshot, error) {
	tree := builtinProjectSnapshot{roots: slices.Clone(roots), files: map[string]editFileSnapshot{}, dirs: map[string]os.FileInfo{}}
	rootInfo, err := os.Stat(root)
	if err != nil {
		return tree, fmt.Errorf("%s: %w", root, err)
	}
	tree.dirs["."] = rootInfo
	if roots == nil {
		roots = []string{"."}
	}
	add := func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := builtinRelativePath(root, path)
		if err != nil {
			return err
		}
		if rel == ".agnostic-ai/"+projectLockName {
			return nil
		}
		if entry.Name() == ".git" {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		if info.IsDir() {
			tree.dirs[rel] = info
			return nil
		}
		snap, err := snapshotEditFile(path)
		if err != nil {
			return err
		}
		tree.files[rel] = snap
		return nil
	}
	for _, input := range roots {
		if strings.HasPrefix(input, "builtin:") {
			continue
		}
		input = strings.TrimSuffix(input, "/**")
		rel, err := builtinRelativePath(root, input)
		if err != nil {
			return tree, err
		}
		if rel == ".agnostic-ai/"+projectLockName {
			continue
		}
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := builtinSafeParents(root, path); err != nil {
			return tree, err
		}
		for parent := filepath.Dir(path); parent != root && path != root; parent = filepath.Dir(parent) {
			info, err := os.Lstat(parent)
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				return tree, fmt.Errorf("%s: %w", parent, err)
			}
			rel, err := builtinRelativePath(root, parent)
			if err != nil {
				return tree, err
			}
			tree.dirs[rel] = info
		}
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			tree.files[rel] = editFileSnapshot{}
			continue
		}
		if err != nil {
			return tree, fmt.Errorf("%s: %w", path, err)
		}
		if info.IsDir() {
			if err := filepath.WalkDir(path, add); err != nil {
				return tree, fmt.Errorf("snapshot %s: %w", path, err)
			}
		} else {
			snap, err := snapshotEditFile(path)
			if err != nil {
				return tree, err
			}
			tree.files[rel] = snap
		}
	}
	return tree, nil
}

func builtinSafeParents(root, path string) error {
	if filepath.Clean(path) == filepath.Clean(root) {
		return nil
	}
	rel, err := filepath.Rel(root, filepath.Dir(path))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%s: outside the project", path)
	}
	current := root
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("%s: %w", current, err)
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%s: web changes require real project directories; use the CLI", current)
		}
	}
	return nil
}

func (s builtinProjectSnapshot) sameContent(other builtinProjectSnapshot) bool {
	if len(s.files) != len(other.files) || len(s.dirs) != len(other.dirs) {
		return false
	}
	for path, before := range s.files {
		after, ok := other.files[path]
		if !ok || (before.info == nil) != (after.info == nil) || !bytes.Equal(before.data, after.data) {
			return false
		}
		if before.info != nil && before.info.Mode() != after.info.Mode() {
			return false
		}
	}
	for path, before := range s.dirs {
		after, ok := other.dirs[path]
		if !ok || before.Mode() != after.Mode() {
			return false
		}
	}
	return true
}

func (s builtinProjectSnapshot) check(root string) error {
	now, err := snapshotBuiltinProject(root, s.roots)
	if err != nil {
		return err
	}
	if !s.sameContent(now) {
		return fmt.Errorf("project inputs or output files changed; refresh and preview again")
	}
	for path, before := range s.files {
		if before.info != nil && (!os.SameFile(before.info, now.files[path].info) || !before.info.ModTime().Equal(now.files[path].info.ModTime())) {
			return fmt.Errorf("%s: changed since preview; refresh and preview again", path)
		}
	}
	for path, before := range s.dirs {
		if !os.SameFile(before, now.dirs[path]) {
			return fmt.Errorf("%s: directory changed since preview; refresh and preview again", path)
		}
	}
	return nil
}

type builtinWritten struct {
	change builtinFileChange
	after  editFileSnapshot
}

type builtinDirectory struct {
	path string
	info os.FileInfo
}

type builtinJournal struct {
	rootInfo    os.FileInfo
	parents     map[string]os.FileInfo
	writes      []builtinWritten
	createdDirs []builtinDirectory
}

func applyBuiltinFiles(root string, changes []builtinFileChange, beforeWrite func(string) error) (builtinJournal, error) {
	var journal builtinJournal
	anchored, err := os.OpenRoot(root)
	if err != nil {
		return journal, fmt.Errorf("%s: %w", root, err)
	}
	defer func() { _ = anchored.Close() }()
	journal.rootInfo, err = anchored.Stat(".")
	if err != nil {
		return journal, fmt.Errorf("%s: %w", root, err)
	}
	journal.parents, err = captureBuiltinParents(anchored, changes)
	if err != nil {
		return journal, err
	}
	for _, change := range changes {
		path := filepath.Join(root, filepath.FromSlash(change.Path))
		if err := builtinSafeParents(root, path); err != nil {
			return journal, err
		}
		if beforeWrite != nil {
			if err := beforeWrite(change.Path); err != nil {
				return journal, fmt.Errorf("%s: %w", change.Path, err)
			}
		}
		if err := checkBuiltinParents(anchored, change.Path, journal.parents, true); err != nil {
			return journal, err
		}
		if err := checkBuiltinFile(anchored, change.Path, change.before); err != nil {
			return journal, err
		}
		if change.remove {
			if err := checkBuiltinParents(anchored, change.Path, journal.parents, false); err != nil {
				return journal, err
			}
			if err := anchored.Remove(change.Path); err != nil {
				return journal, fmt.Errorf("%s: %w", change.Path, err)
			}
			journal.writes = append(journal.writes, builtinWritten{change: change})
			continue
		}
		var missing []string
		for dir := filepath.Dir(change.Path); dir != "."; dir = filepath.Dir(dir) {
			_, err := anchored.Lstat(dir)
			if err == nil {
				break
			}
			if !errors.Is(err, os.ErrNotExist) {
				return journal, fmt.Errorf("%s: %w", dir, err)
			}
			missing = append(missing, dir)
		}
		slices.Reverse(missing)
		for _, dir := range missing {
			if err := checkBuiltinParents(anchored, dir, journal.parents, false); err != nil {
				return journal, err
			}
			if err := anchored.Mkdir(dir, 0o755); err != nil {
				return journal, fmt.Errorf("%s: %w", dir, err)
			}
			info, err := anchored.Lstat(dir)
			if err != nil {
				return journal, fmt.Errorf("%s: %w", dir, err)
			}
			journal.parents[dir] = info
			journal.createdDirs = append(journal.createdDirs, builtinDirectory{path: dir, info: info})
		}
		mode := os.FileMode(0o644)
		if change.before.info != nil {
			mode = change.before.info.Mode().Perm()
		}
		if change.mode != 0 {
			mode = change.mode
		}
		written, err := replaceBuiltinFile(anchored, journal.parents, change.Path, change.before, change.after, mode)
		if err != nil {
			return journal, err
		}
		journal.writes = append(journal.writes, builtinWritten{change: change, after: written})
	}
	return journal, nil
}

func (j builtinJournal) rollback(root string) (builtinJournal, error) {
	restored := builtinJournal{rootInfo: j.rootInfo, parents: j.parents}
	anchored, err := j.openRoot(root)
	if err != nil {
		return restored, err
	}
	defer func() { _ = anchored.Close() }()
	var failures []error
	for _, written := range slices.Backward(j.writes) {
		path := filepath.Join(root, written.change.Path)
		if err := builtinSafeParents(root, path); err != nil {
			failures = append(failures, err)
			continue
		}
		if err := checkBuiltinFile(anchored, written.change.Path, written.after); err != nil {
			failures = append(failures, err)
			continue
		}
		var after editFileSnapshot
		if written.change.before.info == nil {
			if err := checkBuiltinParents(anchored, written.change.Path, j.parents, false); err != nil {
				failures = append(failures, err)
				continue
			}
			if err := anchored.Remove(written.change.Path); err != nil {
				failures = append(failures, fmt.Errorf("%s: %w", path, err))
				continue
			}
		} else {
			var err error
			after, err = replaceBuiltinFile(anchored, j.parents, written.change.Path, written.after, written.change.before.data, written.change.before.info.Mode().Perm())
			if err != nil {
				failures = append(failures, err)
				continue
			}
		}
		restored.writes = append(restored.writes, builtinWritten{change: builtinFileChange{Path: written.change.Path}, after: after})
	}
	if err := j.removeCreatedDirs(root); err != nil {
		failures = append(failures, err)
	}
	return restored, errors.Join(failures...)
}

func (j builtinJournal) reverse() []builtinFileChange {
	changes := make([]builtinFileChange, 0, len(j.writes))
	for _, written := range slices.Backward(j.writes) {
		change := builtinFileChange{Path: written.change.Path, before: written.after, after: written.change.before.data, remove: written.change.before.info == nil}
		if written.change.before.info != nil {
			change.mode = written.change.before.info.Mode().Perm()
		}
		changes = append(changes, change)
	}
	return changes
}

func (j builtinJournal) check(root string) error {
	for _, written := range j.writes {
		path := filepath.Join(root, written.change.Path)
		if err := builtinSafeParents(root, path); err != nil {
			return err
		}
		if err := written.after.check(path); err != nil {
			return err
		}
	}
	return nil
}

func (j builtinJournal) removeCreatedDirs(root string) error {
	anchored, err := j.openRoot(root)
	if err != nil {
		return err
	}
	defer func() { _ = anchored.Close() }()
	var failures []error
	for _, dir := range slices.Backward(j.createdDirs) {
		info, err := anchored.Lstat(dir.path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", dir.path, err))
			continue
		}
		if !os.SameFile(dir.info, info) || info.Mode() != dir.info.Mode() {
			failures = append(failures, fmt.Errorf("%s: directory changed; kept it", dir.path))
			continue
		}
		if err := checkBuiltinParents(anchored, dir.path, j.parents, false); err != nil {
			failures = append(failures, err)
			continue
		}
		if err := anchored.Remove(dir.path); err != nil {
			failures = append(failures, fmt.Errorf("%s: kept directory: %w", dir.path, err))
		}
	}
	return errors.Join(failures...)
}

func (s builtinProjectSnapshot) afterWrites(root string, journal builtinJournal) (builtinProjectSnapshot, error) {
	now, err := snapshotBuiltinProject(root, s.roots)
	if err != nil {
		return now, err
	}
	expected := make(map[string]editFileSnapshot, len(s.files))
	for path, snap := range s.files {
		expected[path] = snap
	}
	for _, write := range journal.writes {
		expected[write.change.Path] = write.after
	}
	for path, snap := range expected {
		current, ok := now.files[path]
		if !ok && snap.info == nil {
			continue
		}
		if !ok || !sameBuiltinFile(snap, current) {
			return now, fmt.Errorf("%s: changed during the operation", path)
		}
	}
	for path, snap := range now.files {
		if old, ok := expected[path]; !ok || !sameBuiltinFile(old, snap) {
			return now, fmt.Errorf("%s: appeared during the operation", path)
		}
	}
	expectedDirs := make(map[string]os.FileInfo, len(s.dirs))
	for path, info := range s.dirs {
		expectedDirs[path] = info
	}
	for _, dir := range journal.createdDirs {
		rel, err := builtinRelativePath(root, dir.path)
		if err != nil {
			return now, err
		}
		expectedDirs[rel] = dir.info
	}
	for path, info := range now.dirs {
		expected, ok := expectedDirs[path]
		if !ok || !os.SameFile(expected, info) || expected.Mode() != info.Mode() {
			return now, fmt.Errorf("%s: directory changed during the operation", path)
		}
	}
	for path := range s.dirs {
		if _, ok := now.dirs[path]; !ok {
			return now, fmt.Errorf("%s: directory disappeared during the operation", path)
		}
	}
	return now, nil
}

func sameBuiltinFile(a, b editFileSnapshot) bool {
	if a.info == nil || b.info == nil {
		return a.info == nil && b.info == nil
	}
	return bytes.Equal(a.data, b.data) && a.info.Mode() == b.info.Mode() && os.SameFile(a.info, b.info) && a.info.ModTime().Equal(b.info.ModTime())
}

func (j builtinJournal) openRoot(path string) (*os.Root, error) {
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	info, err := root.Stat(".")
	if err != nil || j.rootInfo == nil || !os.SameFile(j.rootInfo, info) {
		_ = root.Close()
		return nil, fmt.Errorf("%s: project directory changed; keep current files", path)
	}
	return root, nil
}
