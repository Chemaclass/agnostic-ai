package cli

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/chemaclass/agnostic-ai/internal/authority"
)

const (
	builtinStatusPath  = "/api/changes/builtin"
	builtinPreviewPath = builtinStatusPath + "/preview"
	builtinApplyPath   = builtinStatusPath + "/apply"
	builtinUndoPath    = builtinStatusPath + "/undo"
)

type builtinChangeView struct {
	SchemaVersion string              `json:"schemaVersion"`
	Status        string              `json:"status"`
	ID            string              `json:"id,omitempty"`
	Name          string              `json:"name"`
	Enabled       bool                `json:"enabled"`
	Config        string              `json:"config,omitempty"`
	Targets       []string            `json:"targets"`
	Changes       []builtinFileChange `json:"changes"`
	Findings      []string            `json:"findings"`
	UndoID        string              `json:"undoId,omitempty"`
	UndoAvailable bool                `json:"undoAvailable"`
	Supported     bool                `json:"supported"`
}

type builtinChangeHandler struct {
	root        string
	global      bool
	mu          sync.Mutex
	plan        *builtinChangePlan
	last        *builtinChangeView
	applied     *builtinChangeView
	undo        builtinJournal
	after       *builtinProjectSnapshot
	beforeWrite func(string) error
}

func newBuiltinChangeHandler(root string, global bool) *builtinChangeHandler {
	return &builtinChangeHandler{root: root, global: global}
}

func (h *builtinChangeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == builtinStatusPath && r.Method == http.MethodGet {
		if !authority.RequestAllowed(r) {
			writeEditJSON(w, http.StatusForbidden, editError{Error: "changes are served on loopback only"})
			return
		}
		h.mu.Lock()
		defer h.mu.Unlock()
		view := builtinChangeView{SchemaVersion: "builtin-change/v1", Status: "idle", Name: "caveman", Supported: !h.global, Changes: []builtinFileChange{}, Findings: []string{}, Targets: []string{}}
		if h.last != nil {
			view = *h.last
		}
		view.UndoAvailable = h.after != nil
		if h.applied != nil {
			view.UndoID = h.applied.ID
		}
		writeEditJSON(w, http.StatusOK, view)
		return
	}
	if status, message := editRequestGuard(r); status != 0 {
		writeEditJSON(w, status, editError{Error: message})
		return
	}
	if h.global {
		writeEditJSON(w, http.StatusBadRequest, editError{Error: "Caveman preview, apply, and undo are available for projects only"})
		return
	}
	var request struct {
		Name    string `json:"name"`
		Enabled *bool  `json:"enabled"`
		ID      string `json:"id"`
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, 8193))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeEditJSON(w, http.StatusBadRequest, editError{Error: "send one JSON object with the requested change"})
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		writeEditJSON(w, http.StatusBadRequest, editError{Error: "send exactly one JSON object"})
		return
	}
	if r.URL.Path != builtinPreviewPath && (request.Name != "" || request.Enabled != nil || request.ID == "") {
		writeEditJSON(w, http.StatusBadRequest, editError{Error: "apply and undo require only a proposal id"})
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	var view builtinChangeView
	var err error
	switch r.URL.Path {
	case builtinPreviewPath:
		if request.Name != "caveman" || request.Enabled == nil || request.ID != "" {
			writeEditJSON(w, http.StatusBadRequest, editError{Error: "preview requires name caveman and an enabled boolean"})
			return
		}
		var plan *builtinChangePlan
		plan, err = planBuiltinChange(h.root, *request.Enabled)
		if err == nil {
			var token [24]byte
			if _, err = rand.Read(token[:]); err == nil {
				plan.view.ID = hex.EncodeToString(token[:])
				plan.view.Supported = true
				h.plan = plan
				view = plan.view
				h.last = &view
			}
		}
	case builtinApplyPath:
		view, err = h.apply(request.ID)
	case builtinUndoPath:
		view, err = h.restore(request.ID)
	default:
		writeEditJSON(w, http.StatusNotFound, editError{Error: "unknown change action"})
		return
	}
	if err != nil {
		code := http.StatusUnprocessableEntity
		var conflict *builtinConflict
		if errors.As(err, &conflict) {
			code = http.StatusConflict
			if r.URL.Path == builtinApplyPath && h.plan != nil && h.plan.view.ID == request.ID {
				h.plan, h.last = nil, h.applied
			}
		}
		writeEditJSON(w, code, editError{Error: err.Error()})
		return
	}
	if h.applied != nil {
		view.UndoAvailable, view.UndoID = h.after != nil, h.applied.ID
	}
	writeEditJSON(w, http.StatusOK, view)
}

type builtinConflict struct{ err error }

func (e *builtinConflict) Error() string { return e.err.Error() }
func (e *builtinConflict) Unwrap() error { return e.err }
func builtinChanged(err error) error     { return &builtinConflict{err: err} }

// Reuse the CLI's OS lock without changing ignore files or lock metadata.
func lockBuiltinChange(root string) (*os.File, error) {
	path := filepath.Join(root, ".agnostic-ai", projectLockName)
	if err := builtinSafeParents(root, path); err != nil {
		return nil, err
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s: project lock is unavailable; run agnostic-ai sync first", path)
	}
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		_ = file.Close()
		return nil, fmt.Errorf("%s: project lock changed", path)
	}
	if err := tryLockProjectFile(file); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("%s: another command is changing this project; retry when it finishes: %w", path, err)
	}
	return file, nil
}

func (h *builtinChangeHandler) apply(id string) (builtinChangeView, error) {
	if h.plan == nil || id == "" || h.plan.view.ID != id {
		return builtinChangeView{}, builtinChanged(fmt.Errorf("proposal is no longer available; preview again"))
	}
	plan := h.plan
	lock, err := lockBuiltinChange(h.root)
	if err != nil {
		return builtinChangeView{}, builtinChanged(err)
	}
	defer func() { _ = lock.Close() }()
	if err := plan.inputs.check(h.root); err != nil {
		return builtinChangeView{}, builtinChanged(err)
	}
	if _, err := planBuiltinChange(h.root, plan.view.Enabled); err != nil {
		return builtinChangeView{}, err
	}
	if err := plan.inputs.check(h.root); err != nil {
		return builtinChangeView{}, builtinChanged(err)
	}
	journal, err := applyBuiltinFiles(h.root, plan.changes, h.beforeWrite)
	var after builtinProjectSnapshot
	if err == nil {
		after, err = plan.inputs.afterWrites(h.root, journal)
	}
	if err == nil {
		var reports []driftReport
		reports, err = collectDrift(plan.view.Targets)
		if err == nil {
			for _, report := range reports {
				if report.hasDrift() {
					err = fmt.Errorf("%s: generated files still differ after apply", report.Target)
					break
				}
			}
		}
	}
	if err != nil {
		h.plan = nil
		h.last = nil
		restored, rollback := journal.rollback(h.root)
		if rollback != nil {
			return builtinChangeView{}, fmt.Errorf("apply failed: %w; rollback could not restore all files: %v; keep the current files and run agnostic-ai doctor before retrying", err, rollback)
		}
		h.refreshUndoAfterRollback(restored)
		return builtinChangeView{}, fmt.Errorf("apply failed and its file changes were restored: %w", err)
	}

	h.undo, h.after, h.plan = journal, &after, nil
	view := plan.view
	view.Status, view.UndoAvailable, view.UndoID = "applied", true, view.ID
	h.last, h.applied = &view, &view
	return view, nil
}

func (h *builtinChangeHandler) restore(id string) (builtinChangeView, error) {
	if h.after == nil || h.applied == nil || h.applied.ID != id || id == "" {
		return builtinChangeView{}, builtinChanged(fmt.Errorf("undo is no longer available in this server session"))
	}
	lock, err := lockBuiltinChange(h.root)
	if err != nil {
		return builtinChangeView{}, builtinChanged(err)
	}
	defer func() { _ = lock.Close() }()
	if err := h.after.check(h.root); err != nil {
		return builtinChangeView{}, builtinChanged(err)
	}
	if err := h.undo.check(h.root); err != nil {
		return builtinChangeView{}, builtinChanged(err)
	}
	journal, err := applyBuiltinFiles(h.root, h.undo.reverse(), h.beforeWrite)
	if err == nil {
		_, err = h.after.afterWrites(h.root, journal)
	}
	if err != nil {
		restored, rollback := journal.rollback(h.root)
		if rollback != nil {
			return builtinChangeView{}, fmt.Errorf("undo failed: %w; rollback could not restore all files: %v; run agnostic-ai doctor before retrying", err, rollback)
		}
		h.refreshUndoAfterRollback(restored)
		return builtinChangeView{}, fmt.Errorf("undo failed and the applied files were restored: %w", err)
	}
	cleanup := h.undo.removeCreatedDirs(h.root)
	view := *h.applied
	view.UndoID = ""
	view.Status, view.UndoAvailable = "undone", false
	if cleanup != nil {
		view.Findings = append(view.Findings, "Files restored; directory cleanup: "+cleanup.Error())
	}
	h.last, h.applied, h.after, h.plan, h.undo = &view, nil, nil, nil, builtinJournal{}
	return view, nil
}

func (h *builtinChangeHandler) refreshUndoAfterRollback(restored builtinJournal) {
	if h.after == nil {
		return
	}
	for _, write := range restored.writes {
		before, exists := h.after.files[write.change.Path]
		if !exists || (before.info == nil) != (write.after.info == nil) || !bytes.Equal(before.data, write.after.data) {
			return
		}
		if before.info != nil && before.info.Mode() != write.after.info.Mode() {
			return
		}
	}
	after, err := h.after.afterWrites(h.root, restored)
	if err != nil {
		return
	}
	h.after = &after
	for i := range h.undo.writes {
		path := h.undo.writes[i].change.Path
		h.undo.writes[i].after = after.files[path]
	}
}
