// Package hookpaths reads the files an agent edit touches out of a
// target's hook payload, so one edit hook can run on every target that
// reports edits.
package hookpaths

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/applypatch"
)

// Action is what an edit did to one path.
type Action string

// A move reads as a delete of its source and a move of its destination,
// so every path the edit leaves on disk is an add, update, or move.
const (
	ActionAdd    Action = "add"
	ActionUpdate Action = "update"
	ActionDelete Action = "delete"
	ActionMove   Action = "move"
)

// Change is one path an edit touched. From is the source of a move.
type Change struct {
	Action Action `json:"action"`
	Path   string `json:"path"`
	From   string `json:"from,omitempty"`
}

// Written reports whether the path exists after the edit.
func (c Change) Written() bool { return c.Action != ActionDelete }

// Payload is what a hook payload says about an edit. Cwd is the
// directory the payload's relative paths start from, when it names one.
type Payload struct {
	Cwd     string
	Changes []Change
}

// ErrUnsupportedTarget means the target reports no edits in a form
// hookpaths reads.
var ErrUnsupportedTarget = errors.New("hook paths does not read this target's payload")

type decoder func(raw []byte) (Payload, error)

// Each decoder follows the payload its vendor documents; the hooks page
// of the docs site cites the sources.
var decoders = map[string]decoder{
	"augment":  readAugment,
	"claude":   readToolInput,
	"codex":    readToolInput,
	"cursor":   readCursor,
	"factory":  readFactory,
	"gemini":   readGemini,
	"windsurf": readWindsurf,
}

// Targets lists the targets Read decodes.
func Targets() []string {
	names := make([]string, 0, len(decoders))
	for name := range decoders {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Read decodes target's hook payload. A payload for a tool call that is
// no edit, or no payload at all, returns no changes and no error.
func Read(target string, raw []byte) (Payload, error) {
	decode, ok := decoders[target]
	if !ok {
		return Payload{}, fmt.Errorf("%s: %w", target, ErrUnsupportedTarget)
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return Payload{}, nil
	}
	p, err := decode(raw)
	if err != nil {
		return Payload{}, fmt.Errorf("read %s hook payload: %w", target, err)
	}
	return p, nil
}

// Relative returns the changes with every path joined to the payload's
// Cwd (or root when it has none) and printed relative to root. A path
// outside root stays absolute.
func (p Payload) Relative(root string) []Change {
	base := p.Cwd
	if base == "" {
		base = root
	}
	out := make([]Change, 0, len(p.Changes))
	for _, c := range p.Changes {
		c.Path = relative(root, base, c.Path)
		if c.From != "" {
			c.From = relative(root, base, c.From)
		}
		out = append(out, c)
	}
	return out
}

func relative(root, base, path string) string {
	if !filepath.IsAbs(path) {
		path = filepath.Join(base, path)
	}
	if rel, ok := within(root, path); ok {
		return rel
	}
	// The tool may name the project through a symlink the working
	// directory resolves, such as macOS /var and /private/var.
	if rel, ok := within(resolveExisting(root), resolveExisting(path)); ok {
		return rel
	}
	return filepath.Clean(path)
}

// resolveExisting resolves symlinks in the longest part of path that
// exists: a deleted file, or one a pre-edit hook sees, is not there yet.
func resolveExisting(path string) string {
	rest := ""
	for dir := path; ; dir = filepath.Dir(dir) {
		if real, err := filepath.EvalSymlinks(dir); err == nil {
			return filepath.Join(real, rest)
		}
		if parent := filepath.Dir(dir); parent == dir {
			return path
		}
		rest = filepath.Join(filepath.Base(dir), rest)
	}
}

func within(root, path string) (string, bool) {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return rel, true
}

// toolPayload is the shape most targets share: the tool name, its
// input, and the session's working directory. Each tool has its own
// input shape, so the input is decoded only for an edit tool.
type toolPayload struct {
	Cwd       string          `json:"cwd"`
	ToolName  string          `json:"tool_name"`
	ToolInput json.RawMessage `json:"tool_input"`
}

type editInput struct {
	FilePath     string `json:"file_path"`
	NotebookPath string `json:"notebook_path"`
	Path         string `json:"path"`
	Command      string `json:"command"`
}

func (t toolPayload) editInput() (editInput, error) {
	var in editInput
	if len(t.ToolInput) == 0 {
		return in, nil
	}
	if err := json.Unmarshal(t.ToolInput, &in); err != nil {
		return in, fmt.Errorf("%s tool_input: %w", t.ToolName, err)
	}
	return in, nil
}

// decodeEdit decodes raw, and the tool's input when the tool is one of
// edits. ok is false for any other tool.
func decodeEdit(raw []byte, edits ...string) (t toolPayload, in editInput, ok bool, err error) {
	if err = json.Unmarshal(raw, &t); err != nil || !slices.Contains(edits, t.ToolName) {
		return t, in, false, err
	}
	in, err = t.editInput()
	return t, in, err == nil, err
}

// readToolInput reads Claude Code's file tools and Codex's apply_patch.
// Codex also accepts Edit and Write as matcher aliases, so a patch body
// is read whatever tool name it arrives under.
func readToolInput(raw []byte) (Payload, error) {
	t, in, ok, err := decodeEdit(raw, "apply_patch", "Edit", "Write", "MultiEdit", "NotebookEdit")
	p := Payload{Cwd: t.Cwd}
	switch {
	case !ok:
	case t.ToolName == "apply_patch" || in.Command != "":
		p.Changes = patchChanges(in.Command)
	default:
		p.Changes = single(ActionUpdate, in.FilePath, in.NotebookPath)
	}
	return p, err
}

func readGemini(raw []byte) (Payload, error) {
	t, in, ok, err := decodeEdit(raw, "write_file", "replace")
	p := Payload{Cwd: t.Cwd}
	if ok {
		p.Changes = single(ActionUpdate, in.FilePath)
	}
	return p, err
}

// readFactory reads Create and Edit. Factory documents no input shape
// for ApplyPatch, so a patch reports nothing.
func readFactory(raw []byte) (Payload, error) {
	t, in, ok, err := decodeEdit(raw, "Create", "Edit")
	p := Payload{Cwd: t.Cwd}
	switch {
	case !ok:
	case t.ToolName == "Create":
		p.Changes = single(ActionAdd, in.FilePath)
	default:
		p.Changes = single(ActionUpdate, in.FilePath)
	}
	return p, err
}

// readCursor reads the edit events. beforeReadFile carries a file_path
// too, but a read is no edit.
func readCursor(raw []byte) (Payload, error) {
	var in struct {
		Event    string `json:"hook_event_name"`
		FilePath string `json:"file_path"`
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return Payload{}, err
	}
	var p Payload
	if in.Event == "afterFileEdit" || in.Event == "afterTabFileEdit" {
		p.Changes = single(ActionUpdate, in.FilePath)
	}
	return p, nil
}

func readWindsurf(raw []byte) (Payload, error) {
	var in struct {
		Action   string          `json:"agent_action_name"`
		ToolInfo json.RawMessage `json:"tool_info"`
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return Payload{}, err
	}
	if in.Action != "pre_write_code" && in.Action != "post_write_code" {
		return Payload{}, nil
	}
	info, err := toolPayload{ToolName: in.Action, ToolInput: in.ToolInfo}.editInput()
	if err != nil {
		return Payload{}, err
	}
	return Payload{Changes: single(ActionUpdate, info.FilePath)}, nil
}

// readAugment prefers file_changes, which says what happened to each
// file. A pre-edit payload has none yet, so it falls back to the path
// the edit tool names. Augment paths are relative to the workspace root.
func readAugment(raw []byte) (Payload, error) {
	var in struct {
		toolPayload
		FileChanges []struct {
			Path       string `json:"path"`
			ChangeType string `json:"changeType"`
		} `json:"file_changes"`
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return Payload{}, err
	}
	var p Payload
	for _, fc := range in.FileChanges {
		if fc.Path == "" {
			continue
		}
		action := ActionUpdate
		switch fc.ChangeType {
		case "create":
			action = ActionAdd
		case "delete":
			action = ActionDelete
		}
		p.Changes = append(p.Changes, Change{Action: action, Path: fc.Path})
	}
	if len(in.FileChanges) == 0 && (in.ToolName == "str-replace-editor" || in.ToolName == "save-file") {
		edit, err := in.editInput()
		if err != nil {
			return Payload{}, err
		}
		p.Changes = single(ActionUpdate, edit.Path)
	}
	return p, nil
}

func single(action Action, paths ...string) []Change {
	for _, path := range paths {
		if path != "" {
			return []Change{{Action: action, Path: path}}
		}
	}
	return nil
}

func patchChanges(patch string) []Change {
	var out []Change
	for _, op := range applypatch.Parse(patch) {
		switch {
		case op.MoveTo != "":
			out = append(out,
				Change{Action: ActionDelete, Path: op.Path},
				Change{Action: ActionMove, Path: op.MoveTo, From: op.Path})
		case op.Kind == applypatch.Add:
			out = append(out, Change{Action: ActionAdd, Path: op.Path})
		case op.Kind == applypatch.Delete:
			out = append(out, Change{Action: ActionDelete, Path: op.Path})
		default:
			out = append(out, Change{Action: ActionUpdate, Path: op.Path})
		}
	}
	return out
}
