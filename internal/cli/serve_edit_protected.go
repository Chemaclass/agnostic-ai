package cli

import (
	"bytes"
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/overview"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const protectedKey = overview.ProtectedKey

// protectedBlock is a settings spec's protected block as written. An
// empty decision or reason is one the file leaves out.
type protectedBlock struct {
	Paths    []string `json:"paths"`
	Decision string   `json:"decision"`
	Reason   string   `json:"reason"`
}

type protectedEditRequest struct {
	Action    string          `json:"action"`
	Audience  string          `json:"audience"`
	Path      string          `json:"path"`
	Protected json.RawMessage `json:"protected"`
	Before    json.RawMessage `json:"before"`
	Inherited json.RawMessage `json:"inherited"`
	Expected  string          `json:"expected"`
}

type protectedEditView struct {
	SchemaVersion string          `json:"schemaVersion"`
	File          string          `json:"file"`
	Audience      string          `json:"audience"`
	Before        *protectedBlock `json:"before"`
	Protected     *protectedBlock `json:"protected"`
	Diff          string          `json:"diff"`
	Expected      string          `json:"expected"`
	SyncCommand   string          `json:"syncCommand"`
	OK            bool            `json:"ok,omitempty"`
}

func (h *editHandler) serveProtected(w http.ResponseWriter, body []byte) {
	var req protectedEditRequest
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if dec.Decode(&req) != nil || dec.Decode(&struct{}{}) != io.EOF || (req.Action != "preview" && req.Action != "apply") || len(req.Protected) == 0 {
		writeEditJSON(w, http.StatusBadRequest, editError{Error: "send action preview or apply, audience, and protected as paths, decision, and reason, or null"})
		return
	}
	if h.global {
		writeEditJSON(w, http.StatusBadRequest, editError{Error: "sync --global does not write protected paths; protect files from a project"})
		return
	}
	where, err := (audienceFields{Audience: req.Audience}).audience()
	if err != nil {
		writePermissionEditError(w, err, http.StatusBadRequest, nil)
		return
	}
	h.mu.Lock()
	view, plan, err := h.protectedPlan(req, where)
	h.mu.Unlock()
	if err != nil {
		if req.Action == "apply" && req.Expected != "" {
			err = permissionEditConflict{err}
		}
		writePermissionEditError(w, err, http.StatusBadRequest, nil)
		return
	}
	if req.Action == "preview" {
		writeEditJSON(w, http.StatusOK, view)
		return
	}
	if !h.applyPreviewedEdit(w, where, req.Expected, plan, func() (previewedEdit, error) {
		_, now, err := h.protectedPlan(req, where)
		return now, err
	}) {
		return
	}
	view.OK = true
	writeEditJSON(w, http.StatusOK, view)
}

// decodeProtectedBlock reads the requested block, or nil to remove it,
// and refuses one sync would reject.
func decodeProtectedBlock(raw json.RawMessage) (*protectedBlock, error) {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, nil
	}
	var block protectedBlock
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if dec.Decode(&block) != nil {
		return nil, errors.New("protected must have paths as a list, a decision, and a reason")
	}
	if len(block.Paths) == 0 {
		return nil, errors.New("protected: list at least one path")
	}
	paths := make([]any, len(block.Paths))
	for i, path := range block.Paths {
		block.Paths[i] = strings.TrimSpace(path)
		if strings.ContainsAny(path, "\r\n") {
			return nil, fmt.Errorf("protected: path %d must be on one line", i+1)
		}
		paths[i] = block.Paths[i]
	}
	if block.Decision != spec.ProtectAsk && block.Decision != spec.ProtectDeny {
		return nil, fmt.Errorf("protected: decision must be %s or %s", spec.ProtectAsk, spec.ProtectDeny)
	}
	block.Reason = strings.Join(strings.Fields(block.Reason), " ")
	if slices.ContainsFunc(append(slices.Clone(block.Paths), block.Reason), func(text string) bool { return strings.Contains(text, protectedHiddenMarker) }) {
		return nil, fmt.Errorf("protected: %s stands for a hidden value; edit the file directly", protectedHiddenMarker)
	}
	if _, err := spec.ParseProtectGroup(map[string]any{"paths": paths, "decision": block.Decision, "reason": block.Reason}); err != nil {
		return nil, fmt.Errorf("protected: %w", err)
	}
	return &block, nil
}

// protectedBlockAt is the protected block data itself writes, without
// any value a shared spec under it gives.
func protectedBlockAt(data []byte) (*protectedBlock, error) {
	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse settings: %w", err)
	}
	raw, present := doc[protectedKey]
	if !present || raw == nil {
		return nil, nil
	}
	fields, ok := raw.(map[string]any)
	if !ok {
		return nil, errors.New("protected is not a mapping; edit the file directly")
	}
	return protectedBlockOf(fields), nil
}

func protectedBlockOf(fields map[string]any) *protectedBlock {
	block := &protectedBlock{Paths: []string{}}
	list, _ := fields["paths"].([]any)
	for _, item := range list {
		block.Paths = append(block.Paths, fmt.Sprint(item))
	}
	block.Decision, _ = fields["decision"].(string)
	block.Reason, _ = fields["reason"].(string)
	return block
}

const protectedHiddenMarker = "<redacted>"

// shownProtectedBlock is block as the overview shows it, with values that
// look like credentials hidden and an empty value left empty.
func shownProtectedBlock(block *protectedBlock) *protectedBlock {
	if block == nil {
		return nil
	}
	shown := func(key, text string) string {
		if text == "" {
			return ""
		}
		return overview.ShownText(key, text)
	}
	out := &protectedBlock{Paths: make([]string, len(block.Paths)), Decision: shown("protected.decision", block.Decision), Reason: shown("protected.reason", block.Reason)}
	for i, path := range block.Paths {
		out.Paths[i] = shown("protected.paths", path)
	}
	return out
}

func (h *editHandler) protectedPlan(req protectedEditRequest, where editAudience) (protectedEditView, previewedEdit, error) {
	var none previewedEdit
	block, err := decodeProtectedBlock(req.Protected)
	if err != nil {
		return protectedEditView{}, none, err
	}
	file, scaffold, paths, err := h.permissionSource(where, req.Path, protectedKey)
	if err != nil {
		return protectedEditView{}, none, err
	}
	original, err := snapshotEditFile(file)
	if err != nil {
		return protectedEditView{}, none, err
	}
	before, err := protectedBlockAt(original.data)
	if err != nil {
		return protectedEditView{}, none, fmt.Errorf("%s: %w", overviewDisplay(h.root, file), err)
	}
	if req.Action == "preview" && len(req.Before) > 0 {
		var wanted *protectedBlock
		if json.Unmarshal(req.Before, &wanted) != nil {
			return protectedEditView{}, none, errors.New("before must be the selected scope's own protected block, or null when unset")
		}
		if !reflect.DeepEqual(shownProtectedBlock(before), wanted) {
			return protectedEditView{}, none, permissionEditConflict{fmt.Errorf("%s: protected paths changed after editing began; reload saved paths", overviewDisplay(h.root, file))}
		}
	}
	if shown := shownProtectedBlock(before); !reflect.DeepEqual(shown, before) {
		return protectedEditView{}, none, fmt.Errorf("%s: protected paths hold a value the page hides; edit the file directly", overviewDisplay(h.root, file))
	}
	if block == nil && before == nil {
		scope := h.levelLabel()
		if where == audiencePersonal {
			scope = "Local"
		}
		return protectedEditView{}, none, fmt.Errorf("%s: no protected paths are set in %s", overviewDisplay(h.root, file), scope)
	}
	var inherited *protectedBlock
	if where == audiencePersonal {
		project, source, err := h.projectProtectBlock(file)
		if err != nil {
			return protectedEditView{}, none, err
		}
		if req.Action == "preview" && len(req.Inherited) > 0 {
			var wanted *protectedBlock
			if json.Unmarshal(req.Inherited, &wanted) != nil {
				return protectedEditView{}, none, errors.New("inherited must be the project protected block the page showed, or null when unset")
			}
			if !reflect.DeepEqual(shownProtectedBlock(project), wanted) {
				return protectedEditView{}, none, permissionEditConflict{fmt.Errorf("%s: the protected paths this file inherits changed after editing began; reload saved paths", source)}
			}
		}
		if block != nil && project != nil && project.Decision == spec.ProtectDeny && block.Decision == spec.ProtectAsk {
			return protectedEditView{}, none, fmt.Errorf("%s denies edits to protected paths, and a local block cannot lower that to ask; keep deny or change the project block", source)
		}
		inherited = project
	}
	write := protectedWriteFor(block, inherited)
	if block != nil && write == nil {
		if before == nil {
			return protectedEditView{}, none, errors.New("these values match the protected paths this file inherits; there is nothing to save in Local")
		}
	}
	data := original.data
	if original.info == nil {
		data = []byte(scaffold)
	}
	after, err := setProtectedBlock(data, write)
	if err != nil {
		return protectedEditView{}, none, fmt.Errorf("%s: %w", overviewDisplay(h.root, file), err)
	}
	after = permissionEditRest(after, data, write == nil)
	view := protectedEditView{SchemaVersion: "protected-edit/v1", File: overviewDisplay(h.root, file), Audience: string(where), Before: before, Protected: block, SyncCommand: "agnostic-ai sync"}
	view.Diff = previewDiff(view.File, view.File+" (before)", view.File+" (proposed)", string(original.data), string(after), original.info == nil, 2048)
	sum := sha256.New()
	fingerprint, err := json.Marshal(struct {
		File, Audience string
		Protected      *protectedBlock
	}{file, string(where), block})
	if err != nil {
		return protectedEditView{}, none, fmt.Errorf("prepare protected preview: %w", err)
	}
	_, _ = sum.Write(fingerprint)
	_, _ = fmt.Fprintf(sum, "\x00%d\x00", len(after))
	_, _ = sum.Write(after)
	if err := hashEditSources(sum, append(paths, file)); err != nil {
		return protectedEditView{}, none, err
	}
	view.Expected = hex.EncodeToString(sum.Sum(nil))
	return view, previewedEdit{file: file, before: original, after: after, expected: view.Expected}, nil
}

// projectProtectBlock is the protected block of the project spec under
// the local file, as that spec writes it, and the spec's shown path. It
// returns a nil block when no project spec sits under the file or the
// spec has no block.
func (h *editHandler) projectProtectBlock(file string) (*protectedBlock, string, error) {
	scope, err := loadCheckScope(h.global)
	if err != nil {
		return nil, "", err
	}
	for _, e := range scope.bundle.Settings {
		var project *spec.Entry
		switch {
		case e.Base != nil && resolvedAbs(e.Path) == resolvedAbs(file):
			project = e.Base
		case !overview.LocalLayer(e) && resolvedAbs(filepath.Join(h.root, overview.SettingsOverlayPath(h.global, e.Name))) == resolvedAbs(file):
			project = &e
		default:
			continue
		}
		source := overview.SpecPath(h.root, *project)
		fields, ok := project.Meta[protectedKey].(map[string]any)
		if !ok {
			return nil, source, nil
		}
		return protectedBlockOf(fields), source, nil
	}
	return nil, "", nil
}

// protectedWrite is the protected block a file gets: a nil member is a
// key the file leaves to the block it inherits.
type protectedWrite struct {
	Paths    []string `yaml:"paths,omitempty"`
	Decision *string  `yaml:"decision,omitempty"`
	Reason   *string  `yaml:"reason,omitempty"`
}

// protectedWriteFor is what a save of block writes. Over an inherited
// block it keeps only the keys that differ, so a list the user did not
// change keeps following the project; nil means remove the block.
func protectedWriteFor(block, inherited *protectedBlock) *protectedWrite {
	if block == nil {
		return nil
	}
	if inherited == nil {
		write := &protectedWrite{Paths: block.Paths, Decision: &block.Decision}
		if block.Reason != "" {
			write.Reason = &block.Reason
		}
		return write
	}
	write := &protectedWrite{}
	if !slices.Equal(block.Paths, inherited.Paths) {
		write.Paths = block.Paths
	}
	if block.Decision != cmp.Or(inherited.Decision, spec.ProtectAsk) {
		write.Decision = &block.Decision
	}
	if block.Reason != inherited.Reason {
		write.Reason = &block.Reason
	}
	if write.Paths == nil && write.Decision == nil && write.Reason == nil {
		return nil
	}
	return write
}

// setProtectedBlock replaces the protected block in data with write, in
// the documented key order, or removes it when write is nil.
func setProtectedBlock(data []byte, write *protectedWrite) ([]byte, error) {
	var out []byte
	var want any
	var err error
	if write == nil {
		if out, err = removeYAMLPath(data, protectedKey); err != nil {
			return nil, err
		}
	} else {
		if out, err = replaceYAMLPair(data, protectedKey, write); err != nil {
			return nil, err
		}
		fields := map[string]any{}
		if write.Paths != nil {
			fields["paths"] = toAnyList(write.Paths)
		}
		if write.Decision != nil {
			fields["decision"] = *write.Decision
		}
		if write.Reason != nil {
			fields["reason"] = *write.Reason
		}
		want = fields
	}
	var before, after map[string]any
	if err := yaml.Unmarshal(data, &before); err != nil {
		return nil, fmt.Errorf("parse settings: %w", err)
	}
	if before == nil {
		before = map[string]any{}
	}
	if err := yaml.Unmarshal(out, &after); err != nil {
		return nil, fmt.Errorf("parse proposed settings: %w", err)
	}
	if after == nil {
		after = map[string]any{}
	}
	if want == nil {
		delete(before, protectedKey)
	} else {
		before[protectedKey] = want
	}
	if !reflect.DeepEqual(before, after) {
		return nil, errors.New("protected: the edit changes other YAML values")
	}
	return out, nil
}

// replaceYAMLPair writes key: value at the top level, in place of the
// key's current lines when it is set, so comments around it stay put.
func replaceYAMLPair(data []byte, key string, value any) ([]byte, error) {
	_, root, err := yamlDocument(data)
	if err != nil {
		return nil, err
	}
	if root.Anchor != "" || root.Style&yaml.FlowStyle != 0 {
		return nil, errors.New("flow-style or anchored settings are not safely editable")
	}
	at, node := yamlMapValue(root, key)
	if node == nil {
		return editYAMLPair(data, root, key, value)
	}
	keyNode := root.Content[at]
	if node.Anchor != "" || node.Kind == yaml.AliasNode {
		return nil, fmt.Errorf("%s is anchored and not safely editable", key)
	}
	start, err := yamlNodeOffset(data, keyNode)
	if err != nil {
		return nil, err
	}
	lineStart := bytes.LastIndexByte(data[:start], '\n') + 1
	if len(bytes.TrimSpace(data[lineStart:start])) > 0 {
		return nil, fmt.Errorf("%s is not safely editable", key)
	}
	end, err := yamlCollectionEnd(data, node)
	if err != nil {
		return nil, fmt.Errorf("%s is not safely editable: %w", key, err)
	}
	for end < len(data) && data[end] != '\n' {
		end++
	}
	var fragment yaml.Node
	if err := fragment.Encode(map[string]any{key: value}); err != nil {
		return nil, err
	}
	raw, err := encodeYAML(&fragment)
	if err != nil {
		return nil, err
	}
	newline := "\n"
	if bytes.Contains(data, []byte("\r\n")) {
		newline = "\r\n"
		if end > 0 && data[end-1] == '\r' {
			end--
		}
	}
	lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	indent := strings.Repeat(" ", keyNode.Column-1)
	for i, line := range lines {
		lines[i] = indent + line
	}
	return spliceEditYAML(data, lineStart, end, strings.Join(lines, newline)), nil
}

func toAnyList(items []string) []any {
	out := make([]any, len(items))
	for i, item := range items {
		out[i] = item
	}
	return out
}
