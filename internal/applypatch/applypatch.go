// Package applypatch reads the file operations out of a Codex
// apply_patch body. It reads only the file headers, never the hunks, so
// it tells which files a patch touches, not what it changes in them.
package applypatch

import "strings"

// Kind is what a patch does to one file.
type Kind string

// The operations an apply_patch body can declare.
const (
	Add    Kind = "add"
	Update Kind = "update"
	Delete Kind = "delete"
)

// Op is one file header of a patch. MoveTo is set when an update also
// renames the file.
type Op struct {
	Kind   Kind
	Path   string
	MoveTo string
}

const (
	addHeader    = "*** Add File: "
	updateHeader = "*** Update File: "
	deleteHeader = "*** Delete File: "
	moveHeader   = "*** Move to: "
)

// Parse returns the file operations in patch, in patch order. Hunk and
// added lines start with a space, `+`, `-`, or `@@`, so a header-shaped
// line inside a file body is never read as a header. Text around the
// patch, such as a heredoc wrapper, is ignored.
func Parse(patch string) []Op {
	var ops []Op
	for _, line := range strings.Split(patch, "\n") {
		line = strings.TrimRight(line, " \t\r")
		if path, ok := strings.CutPrefix(line, moveHeader); ok {
			if last := len(ops) - 1; last >= 0 && ops[last].Kind == Update && ops[last].MoveTo == "" && path != "" {
				ops[last].MoveTo = path
			}
			continue
		}
		kind, path, ok := header(line)
		if !ok || path == "" {
			continue
		}
		ops = append(ops, Op{Kind: kind, Path: path})
	}
	return ops
}

func header(line string) (Kind, string, bool) {
	for _, h := range []struct {
		prefix string
		kind   Kind
	}{{addHeader, Add}, {updateHeader, Update}, {deleteHeader, Delete}} {
		if path, ok := strings.CutPrefix(line, h.prefix); ok {
			return h.kind, path, true
		}
	}
	return "", "", false
}
