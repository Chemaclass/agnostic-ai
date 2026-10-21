package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/suggest"
)

// unreadOverlay is a file under `.agnostic-ai/overlays/` that no target
// reads, so its settings never reach any tool.
type unreadOverlay struct {
	Path    string
	Message string
}

func overlayKnown(rel string) bool {
	for _, o := range adapters.KnownOverlays() {
		if o.Covers(rel) {
			return true
		}
	}
	return false
}

// unreadOverlays lists the files under root's overlay directory that no
// target reads, whether or not that target is in `targets`. Dot files
// such as `.DS_Store` and `.gitkeep` are skipped.
func unreadOverlays(root string) ([]unreadOverlay, error) {
	dir := filepath.Join(root, agnosticOverlayDir)
	var out []unreadOverlay
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if p == dir && errors.Is(err, fs.ErrNotExist) {
				return fs.SkipAll
			}
			return err
		}
		if d.IsDir() || strings.HasPrefix(d.Name(), ".") {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if overlayKnown(rel) {
			return nil
		}
		out = append(out, unreadOverlay{
			Path:    path.Join(agnosticOverlayDir, rel),
			Message: unreadOverlayMessage(rel),
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("%s: %w", dir, err)
	}
	return out, nil
}

// unreadOverlayMessage says why sync skips rel and where its keys belong.
// A file named for a target with no overlay points to that target's
// `x-<target>` block; anything else lists the overlays sync reads.
func unreadOverlayMessage(rel string) string {
	known := adapters.KnownOverlays()
	prefix := strings.FieldsFunc(rel, func(r rune) bool { return r == '.' || r == '/' })[0]
	_, settings := targetsSupportingKind[spec.KindSettings][prefix]
	hasOverlay := false
	for _, o := range known {
		hasOverlay = hasOverlay || o.Target == prefix
	}
	if settings && !hasOverlay {
		return fmt.Sprintf("sync does not read this file; %s has no overlay, so put its keys under x-%s in a settings/ spec", prefix, prefix)
	}
	var names, files []string
	for _, o := range known {
		if o.Dir() {
			names = append(names, o.Name+"<file>")
			continue
		}
		names = append(names, o.Name)
		files = append(files, o.Name)
	}
	msg := "sync does not read this file; it reads these overlays: " + strings.Join(names, ", ")
	if guess := suggest.Name(rel, files); guess != "" {
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
