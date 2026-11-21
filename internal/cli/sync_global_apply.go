package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
)

// pruneEmptyGlobalDirs removes directories a removal left empty, such as
// a deleted skill folder or an Antigravity agent folder, stopping at the
// managed tree root. Best effort: a directory holding anything stays.
func pruneEmptyGlobalDirs(removals, trees []string) {
	for _, path := range removals {
		for dir := filepath.Dir(path); inManagedTree(dir, trees); dir = filepath.Dir(dir) {
			if os.Remove(dir) != nil {
				break
			}
		}
	}
}

// preflightGlobalWrites stops on an unrecorded file in a managed tree
// before anything is written. A file, or a whole skill folder, holding
// exactly what this run writes there is adopted instead, so a lost state
// does not strand earlier output. It returns the adopted paths.
func preflightGlobalWrites(writes []globalWrite, trees []string, old globalState) ([]string, error) {
	owned := map[string]bool{}
	for _, p := range old.Files {
		owned[p] = true
	}
	planned := map[string][]byte{}
	for _, w := range writes {
		planned[w.path] = w.data
	}
	var adopted, folders []string
	for _, w := range writes {
		if inManagedTree(w.path, trees) && !owned[w.path] {
			if filepath.Base(w.path) == "SKILL.md" {
				dir := filepath.Dir(w.path)
				if _, err := os.Stat(dir); err == nil {
					same, err := globalFolderMatches(dir, planned)
					if err != nil {
						return nil, err
					}
					if !same {
						return nil, fmt.Errorf("%s: unmanaged global skill collision", dir)
					}
					if _, err := os.Stat(w.path); err == nil {
						adopted = append(adopted, dir)
						folders = append(folders, dir)
					}
				}
			}
			if _, err := os.Stat(w.path); err == nil {
				if data, err := os.ReadFile(w.path); err != nil || !bytes.Equal(data, w.data) {
					return nil, fmt.Errorf("%s: unmanaged global spec collision", w.path)
				}
				if !inManagedTree(w.path, folders) {
					adopted = append(adopted, w.path)
				}
			}
		}
		// A single user file, such as CLAUDE.md or settings.json kept in
		// a dotfiles repo, is written through its link. A link inside a
		// tree sync owns file by file is refused.
		if info, err := os.Lstat(w.path); err == nil && info.Mode()&os.ModeSymlink != 0 {
			if data, err := os.ReadFile(w.path); err == nil && bytes.Equal(data, w.data) {
				continue
			}
			if inManagedTree(w.path, trees) {
				return nil, fmt.Errorf("%s: refusing to replace symlink", w.path)
			}
			if _, err := filepath.EvalSymlinks(w.path); err != nil {
				return nil, fmt.Errorf("%s: broken symlink: %w", w.path, err)
			}
		}
	}
	return adopted, nil
}

// globalFolderMatches reports whether every file in dir is one this run
// writes, with the same bytes. Files the run adds are allowed, and so is
// the .bak copy that --backup keeps of one of them; a file of any other
// content, or one the run does not write, is not.
func globalFolderMatches(dir string, planned map[string][]byte) (bool, error) {
	same := true
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if _, ok := planned[strings.TrimSuffix(path, ".bak")]; ok && strings.HasSuffix(path, ".bak") && d.Type().IsRegular() {
			return nil
		}
		want, ok := planned[path]
		if !ok || !d.Type().IsRegular() {
			same = false
			return filepath.SkipAll
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		if !bytes.Equal(data, want) {
			same = false
			return filepath.SkipAll
		}
		return nil
	})
	return same, err
}

// inManagedTree reports whether path sits under one of the directory
// surfaces sync --scope user owns for the targets in this run.
func inManagedTree(path string, trees []string) bool {
	for _, tree := range trees {
		if strings.HasPrefix(path, tree+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func removePaths(paths, drop []string) []string {
	unwanted := map[string]bool{}
	for _, p := range drop {
		unwanted[p] = true
	}
	out := paths[:0]
	for _, p := range paths {
		if !unwanted[p] {
			out = append(out, p)
		}
	}
	return out
}

func removePathPrefix(paths []string, prefix string) []string {
	out := paths[:0]
	for _, p := range paths {
		if !strings.HasPrefix(p, prefix) {
			out = append(out, p)
		}
	}
	return out
}

func removedGlobalFiles(old, next []string, writes ...globalWrite) []string {
	keep := map[string]bool{}
	for _, p := range next {
		keep[p] = true
	}
	for _, write := range writes {
		for _, path := range write.keepAliases {
			keep[path] = true
		}
	}
	var out []string
	for _, p := range old {
		if !keep[p] {
			out = append(out, p)
		}
	}
	return out
}

func removalChangedSincePlan(guards map[string]*diskSnapshot, path string, data []byte) error {
	if guard, ok := guards[path]; ok && (guard.absent || !bytes.Equal(guard.data, data)) {
		return fmt.Errorf("%s changed while sync ran, likely written by the tool itself; nothing was written, so rerun the sync", path)
	}
	return nil
}

// applyGlobalChanges writes and removes files, rolling everything back
// on the first failure. It returns a note for each symlink it wrote
// through or removed.
func applyGlobalChanges(writes []globalWrite, removals []string, backup bool, guards map[string]*diskSnapshot) (linked []string, err error) {
	a := &globalApply{backup: backup, guards: guards}
	for _, w := range writes {
		if err := a.write(w); err != nil {
			a.rollback()
			return nil, err
		}
	}
	for _, path := range removals {
		if err := a.remove(path); err != nil {
			a.rollback()
			return nil, err
		}
	}
	return a.linked, nil
}

// globalApply is one run of applyGlobalChanges: what it changed so far,
// so a failure can put it back.
type globalApply struct {
	backup bool
	guards map[string]*diskSnapshot
	done   []globalPrior
	linked []string
}

// globalPrior is a path as it was before applyGlobalChanges changed it.
type globalPrior struct {
	path   string
	data   []byte
	mode   fs.FileMode
	absent bool
	// link is the text of a symlink removed at path.
	link string
}

func (a *globalApply) rollback() {
	for _, p := range slices.Backward(a.done) {
		if p.link != "" {
			_ = os.Symlink(p.link, p.path)
			continue
		}
		if p.absent {
			_ = os.Remove(p.path)
		} else {
			_ = os.WriteFile(p.path, p.data, p.mode)
			_ = os.Chmod(p.path, p.mode.Perm())
		}
	}
}

func (a *globalApply) keepBackup(path string, data []byte, mode fs.FileMode) error {
	bak := path + ".bak"
	if info, err := os.Lstat(bak); err == nil && !info.Mode().IsRegular() {
		return fmt.Errorf("backup %s: %s is not a regular file; move it aside and rerun", path, bak)
	}
	old, err := os.ReadFile(bak)
	absent := os.IsNotExist(err)
	if err != nil && !absent {
		return fmt.Errorf("backup %s: %w", path, err)
	}
	oldMode := mode
	if info, err := os.Stat(bak); err == nil {
		oldMode = info.Mode()
	}
	a.done = append(a.done, globalPrior{path: bak, data: old, mode: oldMode, absent: absent})
	if err := os.WriteFile(bak, data, mode.Perm()); err != nil {
		return fmt.Errorf("backup %s: %w", path, err)
	}
	// An existing .bak keeps its own mode on write; the copy of a
	// private file must be private too.
	if err := os.Chmod(bak, mode.Perm()); err != nil {
		return fmt.Errorf("backup %s: %w", path, err)
	}
	return nil
}

func (a *globalApply) write(w globalWrite) error {
	old, err := os.ReadFile(w.path)
	absent := os.IsNotExist(err)
	if err != nil && !absent {
		return fmt.Errorf("read %s: %w", w.path, err)
	}
	if !absent && reflect.DeepEqual(old, w.data) {
		return nil
	}
	if w.planned != nil && (w.planned.absent != absent || !bytes.Equal(w.planned.data, old)) {
		return fmt.Errorf("%s changed while sync ran, likely written by the tool itself; nothing was written, so rerun the sync", w.path)
	}
	mode := fs.FileMode(0o644)
	if info, statErr := os.Stat(w.path); statErr == nil {
		mode = info.Mode()
	}
	// Replace the file a symlink points at, so the link stays.
	target := w.path
	if info, err := os.Lstat(w.path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		if target, err = filepath.EvalSymlinks(w.path); err != nil {
			return fmt.Errorf("%s: broken symlink: %w", w.path, err)
		}
		a.linked = append(a.linked, "wrote through symlink "+w.path+" -> "+target)
	}
	a.done = append(a.done, globalPrior{path: target, data: old, mode: mode, absent: absent})
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("create directory for %s: %w", w.path, err)
	}
	if a.backup && !absent {
		if err := a.keepBackup(w.path, old, mode); err != nil {
			return err
		}
	}
	tmp, err := os.CreateTemp(filepath.Dir(target), ".agnostic-ai-global-*")
	if err != nil {
		return fmt.Errorf("create temporary file for %s: %w", w.path, err)
	}
	tmpName := tmp.Name()
	// A user's own file, such as an MCP config holding API keys at
	// 0600, keeps its permission bits.
	perm := w.mode
	if (!w.owned || w.keepMode) && !absent {
		perm = mode.Perm()
	}
	writeErr := func() error {
		defer func() { _ = os.Remove(tmpName) }()
		if _, err := tmp.Write(w.data); err != nil {
			return err
		}
		if err := tmp.Chmod(perm); err != nil {
			return err
		}
		if err := tmp.Close(); err != nil {
			return err
		}
		return os.Rename(tmpName, target)
	}()
	if writeErr != nil {
		_ = tmp.Close()
		return fmt.Errorf("write %s: %w", w.path, writeErr)
	}
	return nil
}

func (a *globalApply) remove(path string) error {
	// Removing a file reached through a symlink removes the file the
	// link points at, then the link, so no stale copy stays behind
	// in a dotfiles repository.
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return a.removeLink(path)
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read managed %s: %w", path, err)
	}
	if err := removalChangedSincePlan(a.guards, path, data); err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat managed %s: %w", path, err)
	}
	if a.backup {
		if err := a.keepBackup(path, data, info.Mode()); err != nil {
			return err
		}
	}
	a.done = append(a.done, globalPrior{path: path, data: data, mode: info.Mode()})
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("remove managed %s: %w", path, err)
	}
	return nil
}

func (a *globalApply) removeLink(path string) error {
	text, err := os.Readlink(path)
	if err != nil {
		return fmt.Errorf("read symlink %s: %w", path, err)
	}
	if target, err := filepath.EvalSymlinks(path); err == nil {
		data, readErr := os.ReadFile(target)
		targetInfo, statErr := os.Stat(target)
		if readErr != nil || statErr != nil {
			return fmt.Errorf("read managed %s: %w", target, errors.Join(readErr, statErr))
		}
		if err := removalChangedSincePlan(a.guards, path, data); err != nil {
			return err
		}
		if a.backup {
			if err := a.keepBackup(path, data, targetInfo.Mode()); err != nil {
				return err
			}
		}
		a.done = append(a.done, globalPrior{path: target, data: data, mode: targetInfo.Mode()})
		if err := os.Remove(target); err != nil {
			return fmt.Errorf("remove managed %s: %w", target, err)
		}
		a.linked = append(a.linked, "removed "+target+" and its symlink "+path)
	}
	a.done = append(a.done, globalPrior{path: path, link: text})
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("remove managed %s: %w", path, err)
	}
	return nil
}
