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
	"slices"
	"sync"

	"github.com/chemaclass/agnostic-ai/internal/authority"
	"github.com/chemaclass/agnostic-ai/internal/builtins"
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
	UndoName      string              `json:"undoName,omitempty"`
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
			view.UndoID, view.UndoName = h.applied.ID, h.applied.Name
		}
		writeEditJSON(w, http.StatusOK, view)
		return
	}
	if status, message := editRequestGuard(r); status != 0 {
		writeEditJSON(w, status, editError{Error: message})
		return
	}
	if h.global {
		writeEditJSON(w, http.StatusBadRequest, editError{Error: "Built-in preview, apply, and undo are available for projects only"})
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
		if !slices.Contains(builtins.Names(), request.Name) || request.Enabled == nil || request.ID != "" {
			writeEditJSON(w, http.StatusBadRequest, editError{Error: "preview requires a known built-in name and an enabled boolean"})
			return
		}
		var plan *builtinChangePlan
		plan, err = planBuiltinChange(h.root, request.Name, *request.Enabled)
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
		view.UndoAvailable, view.UndoID, view.UndoName = h.after != nil, h.applied.ID, h.applied.Name
	}
	writeEditJSON(w, http.StatusOK, view)
}

type builtinConflict struct{ err error }

func (e *builtinConflict) Error() string { return e.err.Error() }
func (e *builtinConflict) Unwrap() error { return e.err }
func builtinChanged(err error) error     { return &builtinConflict{err: err} }

// Reuse the CLI's OS lock without changing ignore files or lock metadata.
// A project that was never synced gets the lock sync would create; its
// ignore entries are part of the previewed changes.
func lockBuiltinChange(root string) (lock *os.File, created bool, err error) {
	path := filepath.Join(root, ".agnostic-ai", projectLockName)
	if err := builtinSafeParents(root, path); err != nil {
		return nil, false, err
	}
	missing, err := builtinLockMissing(path)
	if err != nil {
		return nil, false, err
	}
	if missing {
		file, err := openProjectLock(root, "ui")
		if err != nil {
			return nil, false, err
		}
		if err := builtinSameLock(root, path, file); err != nil {
			_ = file.Close()
			return nil, false, err
		}
		return file, true, nil
	}
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, false, fmt.Errorf("%s: %w", path, err)
	}
	if err := builtinSameLock(root, path, file); err != nil {
		_ = file.Close()
		return nil, false, err
	}
	if err := tryLockProjectFile(file); err != nil {
		_ = file.Close()
		return nil, false, fmt.Errorf("%s: another command is changing this project; retry when it finishes: %w", path, err)
	}
	return file, false, nil
}

func builtinSameLock(root, path string, file *os.File) error {
	if err := builtinSafeParents(root, path); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("%s: project lock changed", path)
	}
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return fmt.Errorf("%s: project lock changed", path)
	}
	return nil
}

func (h *builtinChangeHandler) apply(id string) (builtinChangeView, error) {
	if h.plan == nil || id == "" || h.plan.view.ID != id {
		return builtinChangeView{}, builtinChanged(fmt.Errorf("proposal is no longer available; preview again"))
	}
	plan := h.plan
	lock, created, err := lockBuiltinChange(h.root)
	if err != nil {
		return builtinChangeView{}, builtinChanged(err)
	}
	defer func() { _ = lock.Close() }()
	// A nested project's CLI sync takes another project lock but the same
	// info/exclude, so hold its lock through revalidation and rollback.
	releaseExclude, err := lockProjectGitExclude(h.root)
	if err != nil {
		return builtinChangeView{}, builtinChanged(err)
	}
	defer releaseExclude()
	if created {
		if err := plan.inputs.adoptLockDir(h.root); err != nil {
			return builtinChangeView{}, builtinChanged(err)
		}
	}
	if err := plan.inputs.check(h.root); err != nil {
		return builtinChangeView{}, builtinChanged(err)
	}
	if _, err := planBuiltinChange(h.root, plan.view.Name, plan.view.Enabled); err != nil {
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
		h.last = h.applied
		restored, rollback := journal.rollback(h.root)
		if rollback != nil {
			return builtinChangeView{}, fmt.Errorf("apply failed: %w; rollback could not restore all files: %v; keep the current files and run agnostic-ai doctor before retrying", err, rollback)
		}
		h.refreshUndoAfterRollback(restored)
		return builtinChangeView{}, fmt.Errorf("apply failed and its file changes were restored: %w", err)
	}

	h.undo, h.after, h.plan = journal, &after, nil
	view := plan.view
	view.Status, view.UndoAvailable, view.UndoID, view.UndoName = "applied", true, view.ID, view.Name
	h.last, h.applied = &view, &view
	return view, nil
}

func (h *builtinChangeHandler) restore(id string) (builtinChangeView, error) {
	if h.after == nil || h.applied == nil || h.applied.ID != id || id == "" {
		return builtinChangeView{}, builtinChanged(fmt.Errorf("undo is no longer available in this server session"))
	}
	lock, _, err := lockBuiltinChange(h.root)
	if err != nil {
		return builtinChangeView{}, builtinChanged(err)
	}
	defer func() { _ = lock.Close() }()
	// A nested project's CLI sync takes another project lock but the same
	// info/exclude, so hold its lock through revalidation and rollback.
	releaseExclude, err := lockProjectGitExclude(h.root)
	if err != nil {
		return builtinChangeView{}, builtinChanged(err)
	}
	defer releaseExclude()
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
	view.UndoID, view.UndoName = "", ""
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
