package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/http"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/overview"
	"github.com/chemaclass/agnostic-ai/internal/preview"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

type permissionEditRequest struct {
	Action   string          `json:"action"`
	Audience string          `json:"audience"`
	Path     string          `json:"path"`
	Key      string          `json:"key"`
	Rules    json.RawMessage `json:"rules"`
	Before   json.RawMessage `json:"before"`
	Expected string          `json:"expected"`
}

type permissionEditView struct {
	SchemaVersion string   `json:"schemaVersion"`
	File          string   `json:"file"`
	Key           string   `json:"key"`
	Audience      string   `json:"audience"`
	Before        []string `json:"before"`
	Rules         []string `json:"rules"`
	Diff          string   `json:"diff"`
	Expected      string   `json:"expected"`
	SyncCommand   string   `json:"syncCommand"`
	OK            bool     `json:"ok,omitempty"`
}

type permissionEditPlan struct {
	view   permissionEditView
	file   string
	before editFileSnapshot
	after  []byte
}

type permissionEditConflict struct{ error }

func (e permissionEditConflict) Unwrap() error { return e.error }

func writePermissionEditError(w http.ResponseWriter, err error, status int, problems []string) {
	var conflict permissionEditConflict
	if errors.As(err, &conflict) {
		status = http.StatusConflict
	}
	writeEditJSON(w, status, editError{Error: err.Error(), Problems: problems})
}

func (h *editHandler) servePermissions(w http.ResponseWriter, body []byte) {
	var req permissionEditRequest
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if dec.Decode(&req) != nil || dec.Decode(&struct{}{}) != io.EOF || (req.Action != "preview" && req.Action != "apply") || !overview.PermissionListField(req.Key) || len(req.Rules) == 0 {
		writeEditJSON(w, http.StatusBadRequest, editError{Error: "send action preview or apply, audience, permissions.allow/ask/deny key, and rules as a string list or null"})
		return
	}
	where, err := (audienceFields{Audience: req.Audience}).audience()
	if err != nil {
		writePermissionEditError(w, err, http.StatusBadRequest, nil)
		return
	}
	h.mu.Lock()
	plan, err := h.permissionPlan(req, where)
	h.mu.Unlock()
	if err != nil {
		if req.Action == "apply" && req.Expected != "" {
			err = permissionEditConflict{err}
		}
		writePermissionEditError(w, err, http.StatusBadRequest, nil)
		return
	}
	if req.Action == "preview" {
		writeEditJSON(w, http.StatusOK, plan.view)
		return
	}
	if !h.applyPreviewedEdit(w, where, req.Expected, plan.preview(), func() (previewedEdit, error) {
		now, err := h.permissionPlan(req, where)
		if err != nil {
			return previewedEdit{}, err
		}
		return now.preview(), nil
	}) {
		return
	}
	plan.view.OK = true
	writeEditJSON(w, http.StatusOK, plan.view)
}

func (p *permissionEditPlan) preview() previewedEdit {
	return previewedEdit{file: p.file, before: p.before, after: p.after, expected: p.view.Expected}
}

// previewedEdit is a source change a preview showed: the file, its
// content then, the content to write, and the hash the page sends back.
type previewedEdit struct {
	file     string
	before   editFileSnapshot
	after    []byte
	expected string
}

var errStalePreview = permissionEditConflict{errors.New("the source changed; review a fresh preview before applying")}

// applyPreviewedEdit writes plan when expected still names it and a fresh
// plan agrees, and reports the failure otherwise.
func (h *editHandler) applyPreviewedEdit(w http.ResponseWriter, where editAudience, expected string, plan previewedEdit, replan func() (previewedEdit, error)) bool {
	if expected == "" || plan.expected != expected {
		writePermissionEditError(w, errStalePreview, http.StatusConflict, nil)
		return false
	}
	guard := func() error {
		now, err := replan()
		if err != nil {
			return permissionEditConflict{err}
		}
		if now.file != plan.file || now.expected != expected {
			return errStalePreview
		}
		return nil
	}
	var prepare func() (func(bool) error, error)
	if where == audiencePersonal {
		prepare = func() (func(bool) error, error) { return h.preparePersonalOutOfGit(plan.file) }
	}
	problems, err := h.applyValidatedWithPrepare(plan.file, func(data []byte) ([]byte, error) {
		if !bytes.Equal(data, plan.before.data) {
			return nil, errStalePreview
		}
		return plan.after, nil
	}, prepare, guard)
	if err != nil {
		writePermissionEditError(w, err, http.StatusUnprocessableEntity, problems)
		return false
	}
	return true
}

func permissionRules(raw json.RawMessage, key string) ([]string, error) {
	var rules []string
	if json.Unmarshal(raw, &rules) != nil {
		return nil, fmt.Errorf("%s must be a list of rules or null", key)
	}
	list := strings.TrimPrefix(key, "permissions.")
	for i, rule := range rules {
		if strings.TrimSpace(rule) == "" || strings.ContainsAny(rule, "\r\n\x00") {
			return nil, fmt.Errorf("%s: rule %d must be non-empty and on one line", key, i+1)
		}
		if preview.CredentialInRule(rule) {
			return nil, fmt.Errorf("%s: rule %d contains a credential; use an environment reference", key, i+1)
		}
		if _, problem := spec.PermissionRules(list, rule); problem != "" {
			return nil, fmt.Errorf("%s: rule %d is invalid: %s", key, i+1, problem)
		}
	}
	return rules, nil
}

func (h *editHandler) permissionPlan(req permissionEditRequest, where editAudience) (*permissionEditPlan, error) {
	rules, err := permissionRules(req.Rules, req.Key)
	if err != nil {
		return nil, err
	}
	file, scaffold, paths, err := h.permissionSource(where, req.Path, req.Key)
	if err != nil {
		return nil, err
	}
	original, err := snapshotEditFile(file)
	if err != nil {
		return nil, err
	}
	before, present, err := permissionListAt(original.data, req.Key)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	if req.Action == "preview" && len(req.Before) > 0 {
		var wanted []string
		if json.Unmarshal(req.Before, &wanted) != nil {
			return nil, fmt.Errorf("before must be the selected audience's own list, or null when unset")
		}
		if !slices.Equal(before, wanted) || (before == nil) != (wanted == nil) {
			return nil, permissionEditConflict{fmt.Errorf("%s: %s changed after editing began; reload saved rules", overviewDisplay(h.root, file), req.Key)}
		}
	}
	if rules == nil && !present {
		return nil, fmt.Errorf("%s: no %s list is set for %s", overviewDisplay(h.root, file), req.Key, where)
	}
	data := original.data
	if original.info == nil {
		data = []byte(scaffold)
	}
	after, err := setPermissionList(data, req.Key, rules)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	after = permissionEditRest(after, data, rules == nil)
	if baseline, ok := h.baselines[file]; ok && bytes.Equal(original.data, baseline.last) && sameEditValues(baseline.original, after, true, nil) {
		after = slices.Clone(baseline.original)
	}
	view := permissionEditView{SchemaVersion: "permission-edit/v1", File: overviewDisplay(h.root, file), Key: req.Key, Audience: string(where), Before: before, Rules: rules, SyncCommand: "agnostic-ai sync"}
	if h.global {
		view.SyncCommand += " --global"
	}
	view.Diff = previewDiff(view.File, view.File+" (before)", view.File+" (proposed)", string(original.data), string(after), original.info == nil, 2048)
	hash := sha256.New()
	fingerprint, err := json.Marshal(struct {
		File, Key, Audience string
		Rules               []string
	}{file, req.Key, string(where), rules})
	if err != nil {
		return nil, fmt.Errorf("prepare permission preview: %w", err)
	}
	_, _ = hash.Write(fingerprint)
	_, _ = fmt.Fprintf(hash, "\x00%d\x00", len(after))
	_, _ = hash.Write(after)
	if err := hashEditSources(hash, append(paths, file)); err != nil {
		return nil, err
	}
	view.Expected = hex.EncodeToString(hash.Sum(nil))
	return &permissionEditPlan{view: view, file: file, before: original, after: after}, nil
}

// hashEditSources adds the current content of every file an edit reads
// to sum, so a preview goes stale when any of them changes.
func hashEditSources(sum hash.Hash, paths []string) error {
	for _, path := range slices.Compact(paths) {
		snap, err := snapshotEditFile(path)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(sum, "\x00%s\x00%t\x00%d\x00", resolvedAbs(path), snap.info != nil, len(snap.data))
		_, _ = sum.Write(snap.data)
	}
	return nil
}
