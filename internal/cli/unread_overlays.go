package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/suggest"
)

// unreadOverlay is a file under `.agnostic-ai/overlays/` that no target
// reads, so its settings never reach any tool.
type unreadOverlay struct {
	Path    string
	Message string
}

// settingsPassThroughTargets lists the targets that write a settings
// spec's `x-<target>` block into their own files.
var settingsPassThroughTargets = setOf("codebuddy", "pi", "amp", "augment", "copilot", "ellipsis", "factory", "gemini", "junie", "kilo", "opencode", "qoder", "windsurf")

func overlayKnown(rel string) bool {
	for _, o := range adapters.KnownOverlays() {
		if o.Covers(rel) {
			return true
		}
	}
	return false
}

func overlayFolderKnown(name string) bool {
	for _, o := range adapters.KnownOverlays() {
		if o.Dir() && o.Name == name {
			return true
		}
	}
	return false
}

// unreadOverlays lists the files under root's overlay directory that no
// target reads, whether or not that target is in `targets`. A symlinked
// overlay directory is followed, as sync's readers follow it. Dot files,
// dot folders, and README files at the top are skipped, and so is any
// entry the walk cannot read.
func unreadOverlays(root string) ([]unreadOverlay, error) {
	dir := filepath.Join(root, agnosticOverlayDir)
	info, err := os.Stat(dir)
	if errors.Is(err, fs.ErrNotExist) || err == nil && !info.IsDir() {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", dir, err)
	}
	walkRoot, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", dir, err)
	}
	var out []unreadOverlay
	err = filepath.WalkDir(walkRoot, func(p string, d fs.DirEntry, err error) error {
		// The check only adds notes: an entry it cannot read is skipped,
		// never a reason to fail sync or lint.
		if err != nil {
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if p == walkRoot {
			return nil
		}
		rel, err := filepath.Rel(walkRoot, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			if target, err := os.Stat(p); err == nil && target.IsDir() {
				if overlayFolderKnown(rel + "/") {
					return nil
				}
				rel += "/"
			}
		}
		if !strings.Contains(rel, "/") && strings.EqualFold(strings.TrimSuffix(d.Name(), filepath.Ext(d.Name())), "README") {
			return nil
		}
		if overlayKnown(rel) {
			return nil
		}
		out = append(out, unreadOverlay{
			Path:    path.Join(agnosticOverlayDir, rel),
			Message: unreadOverlayMessage(walkRoot, rel),
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("%s: %w", dir, err)
	}
	return out, nil
}

// caseOnlyMatch returns the known overlay name rel matches apart from
// case, when the filesystem resolves both names to the same file.
func caseOnlyMatch(dir, rel string, known []adapters.KnownOverlay) string {
	for _, o := range known {
		var name string
		switch {
		case !o.Dir() && strings.EqualFold(rel, o.Name):
			name = o.Name
		case o.Dir() && strings.EqualFold(rel, o.Name):
			name = o.Name
		case o.Dir():
			first, rest, nested := strings.Cut(rel, "/")
			if nested && rest != "" && !strings.Contains(rest, "/") && strings.EqualFold(first+"/", o.Name) {
				name = o.Name + rest
			}
		}
		if name == "" {
			continue
		}
		a, errA := os.Stat(filepath.Join(dir, filepath.FromSlash(rel)))
		b, errB := os.Stat(filepath.Join(dir, filepath.FromSlash(name)))
		if errA == nil && errB == nil && os.SameFile(a, b) {
			return name
		}
	}
	return ""
}

// unreadOverlayMessage says why sync skips rel and where its keys belong.
// A top-level `<target>.settings.*` file of a target that merges
// `x-<target>` settings points there; anything else lists the overlays
// sync reads.
func unreadOverlayMessage(dir, rel string) string {
	known := adapters.KnownOverlays()
	if name := caseOnlyMatch(dir, rel, known); name != "" {
		return "sync reads this here only because file names ignore case on this filesystem; rename it to " + name + " so every checkout reads it"
	}
	if target, rest, ok := strings.Cut(rel, "."); ok && !strings.Contains(rel, "/") && strings.HasPrefix(rest, "settings.") {
		if _, pass := settingsPassThroughTargets[target]; pass {
			return fmt.Sprintf("sync does not read this file; %s has no overlay, so put its keys under x-%s in a settings/ spec", target, target)
		}
	}
	var names, files, dirs []string
	for _, o := range known {
		names = append(names, o.Name+map[bool]string{true: "<file>"}[o.Dir()])
		if o.Dir() {
			dirs = append(dirs, strings.TrimSuffix(o.Name, "/"))
		} else {
			files = append(files, o.Name)
		}
	}
	msg := "sync does not read this file; it reads these overlays: " + strings.Join(names, ", ")
	guess := suggest.Name(rel, files)
	if first, _, nested := strings.Cut(rel, "/"); nested {
		if folder := suggest.Name(first, dirs); folder != "" {
			guess = folder + "/"
		}
	}
	if guess != "" {
		msg += "; did you mean " + guess + "?"
	}
	return msg
}

// noteUnreadOverlays prints one sync note per overlay file no target reads.
func noteUnreadOverlays(root string) error {
	unread, err := unreadOverlays(root)
	if err != nil {
		return err
	}
	for _, u := range unread {
		adapters.NoteProject(u.Path + ": " + u.Message)
	}
	return nil
}

// lintUnreadOverlays reports each overlay file no target reads (LINT051,
// warning).
func lintUnreadOverlays(root string) ([]lintFinding, error) {
	unread, err := unreadOverlays(root)
	if err != nil {
		return nil, err
	}
	out := make([]lintFinding, 0, len(unread))
	for _, u := range unread {
		out = append(out, lintFinding{Code: "LINT051", Severity: lintWarn, Path: u.Path, Message: u.Message})
	}
	return out, nil
}
