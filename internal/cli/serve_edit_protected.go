package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
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
	block := &protectedBlock{Paths: []string{}}
	list, _ := fields["paths"].([]any)
	for _, item := range list {
		block.Paths = append(block.Paths, fmt.Sprint(item))
	}
	block.Decision, _ = fields["decision"].(string)
	block.Reason, _ = fields["reason"].(string)
	return block, nil
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
		return protectedEditView{}, none, fmt.Errorf("%s: no protected paths are set for %s", overviewDisplay(h.root, file), where)
	}
	data := original.data
	if original.info == nil {
		data = []byte(scaffold)
	}
	after, err := setProtectedBlock(data, block, where == audiencePersonal)
	if err != nil {
		return protectedEditView{}, none, fmt.Errorf("%s: %w", overviewDisplay(h.root, file), err)
	}
	after = permissionEditRest(after, data, block == nil)
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

// setProtectedBlock replaces the whole protected block in data, in the
// documented key order. A local file always writes the reason, because a
// local block merges over the shared one key by key.
func setProtectedBlock(data []byte, block *protectedBlock, local bool) ([]byte, error) {
	out, err := removeYAMLPath(data, protectedKey)
	if err != nil {
		return nil, err
	}
	var want any
	if block != nil {
		value := struct {
			Paths    []string `yaml:"paths"`
			Decision string   `yaml:"decision"`
			Reason   *string  `yaml:"reason,omitempty"`
		}{Paths: block.Paths, Decision: block.Decision}
		if block.Reason != "" || local {
			value.Reason = &block.Reason
		}
		_, root, err := yamlDocument(out)
		if err != nil {
			return nil, err
		}
		if root.Anchor != "" || root.Style&yaml.FlowStyle != 0 {
			return nil, errors.New("flow-style or anchored settings are not safely editable")
		}
		if out, err = editYAMLPair(out, root, protectedKey, value); err != nil {
			return nil, err
		}
		fields := map[string]any{"paths": toAnyList(block.Paths), "decision": block.Decision}
		if value.Reason != nil {
			fields["reason"] = block.Reason
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

func toAnyList(items []string) []any {
	out := make([]any, len(items))
	for i, item := range items {
		out[i] = item
	}
	return out
}
