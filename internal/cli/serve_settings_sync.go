package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/codex"
)

// settingsSyncCommand is what a person runs when the sync from a page
// fails.
const settingsSyncCommand = "agnostic-ai sync --scope user"

// settingsCodexNote follows a sync that wrote the Codex hooks file.
const settingsCodexNote = "Codex will show Hooks need review once. Open /hooks in Codex and trust it."

type settingsSyncRequest struct {
	Action string `json:"action"`
	ID     string `json:"id,omitempty"`
}

// settingsSyncFile is one file a sync --scope user would change.
type settingsSyncFile struct {
	Path   string   `json:"path"`
	Change string   `json:"change"`
	Target string   `json:"target,omitempty"`
	Keys   []string `json:"keys,omitempty"`
}

// settingsSyncPlan lists every pending user-level change, not only the
// approvals ones. id names its files and key changes, so apply refuses a
// plan whose list changed since the person saw it.
type settingsSyncPlan struct {
	ID    string             `json:"id"`
	Files []settingsSyncFile `json:"files"`
	Notes []string           `json:"notes"`
}

type settingsSyncReply struct {
	Written  []string     `json:"written"`
	Notes    []string     `json:"notes"`
	Settings settingsView `json:"settings"`
}

type settingsSyncRefusal struct {
	Error   string            `json:"error"`
	Command string            `json:"command,omitempty"`
	Plan    *settingsSyncPlan `json:"plan,omitempty"`
}

// userSyncReport is the part of sync --scope user --json the page shows.
type userSyncReport struct {
	Writes []struct {
		Target string   `json:"target"`
		Path   string   `json:"path"`
		Action string   `json:"action"`
		Keys   []string `json:"keys"`
	} `json:"writes"`
	Warnings []dropRecord `json:"warnings"`
	Notes    []dropRecord `json:"notes"`
}

// sync plans or applies sync --scope user. Both install hooks, so both
// need proof: otherwise a local program could add a hook spec through
// the edit routes and then install it.
func (h *settingsHandler) sync(w http.ResponseWriter, r *http.Request) {
	if status, message := editRequestGuard(r.WithContext(context.Background())); status != 0 {
		writeEditJSON(w, status, editError{Error: message})
		return
	}
	if !h.proven(r) {
		writeEditJSON(w, http.StatusForbidden, editError{Error: "A sync installs hooks, so only the companion can run it here. Run " + settingsSyncCommand + " in a terminal."})
		return
	}
	var req settingsSyncRequest
	if err := decodeOneJSON(w, r, 1<<10, &req); err != nil || req.Action != "plan" && (req.Action != "apply" || req.ID == "") {
		writeEditJSON(w, http.StatusBadRequest, editError{Error: `send {"action": "plan"} or {"action": "apply", "id": "<plan id>"}`})
		return
	}
	plan, err := h.syncPlan(r.Context())
	if err != nil {
		writeEditJSON(w, http.StatusUnprocessableEntity, settingsSyncRefusal{Error: err.Error(), Command: settingsSyncCommand})
		return
	}
	if req.Action == "plan" {
		writeEditJSON(w, http.StatusOK, plan)
		return
	}
	if plan.ID != req.ID {
		writeEditJSON(w, http.StatusConflict, settingsSyncRefusal{Error: "The pending changes changed since you checked them. Check them again.", Plan: &plan})
		return
	}
	out, err := h.runSync(r.Context(), "sync", "--scope", "user", "--json")
	report, parseErr := parseUserSyncReport(out)
	if err != nil || parseErr != nil {
		if err == nil {
			err = parseErr
		}
		writeEditJSON(w, http.StatusUnprocessableEntity, settingsSyncRefusal{Error: err.Error(), Command: settingsSyncCommand})
		return
	}
	source, _ := h.reader.source()
	h.logChange(source, "sync", "sync --scope user")
	reply := settingsSyncReply{Written: []string{}, Notes: report.notes(), Settings: h.document(r)}
	hooks, _ := codex.UserHooksPath()
	for _, write := range report.Writes {
		reply.Written = append(reply.Written, write.Path)
		if hooks != "" && filepath.Clean(filepath.FromSlash(write.Path)) == filepath.Clean(hooks) && !slices.Contains(reply.Notes, settingsCodexNote) {
			// The short note replaces sync's own untrusted-hook warning, which names the full command.
			reply.Notes = slices.DeleteFunc(reply.Notes, func(note string) bool { return strings.Contains(note, "open /hooks in Codex") })
			reply.Notes = append(reply.Notes, settingsCodexNote)
		}
	}
	writeEditJSON(w, http.StatusOK, reply)
}

// syncPlan runs sync --scope user --plan --json.
func (h *settingsHandler) syncPlan(ctx context.Context) (settingsSyncPlan, error) {
	out, err := h.runSync(ctx, "sync", "--scope", "user", "--plan", "--json")
	if err != nil {
		return settingsSyncPlan{}, err
	}
	report, err := parseUserSyncReport(out)
	if err != nil {
		return settingsSyncPlan{}, err
	}
	plan := settingsSyncPlan{Files: []settingsSyncFile{}, Notes: report.notes()}
	for _, write := range report.Writes {
		plan.Files = append(plan.Files, settingsSyncFile{Path: write.Path, Change: write.Action, Target: write.Target, Keys: write.Keys})
	}
	files, err := json.Marshal(plan.Files)
	if err != nil {
		return settingsSyncPlan{}, fmt.Errorf("encode the sync plan: %w", err)
	}
	sum := sha256.Sum256(files)
	plan.ID = hex.EncodeToString(sum[:16])
	return plan, nil
}

func parseUserSyncReport(out []byte) (userSyncReport, error) {
	var report userSyncReport
	if err := json.Unmarshal(out, &report); err != nil {
		return userSyncReport{}, fmt.Errorf("read the sync report: %w", err)
	}
	return report, nil
}

func (r userSyncReport) notes() []string {
	notes := []string{}
	for _, list := range [][]dropRecord{r.Warnings, r.Notes} {
		for _, d := range list {
			if d.Message != "" {
				notes = append(notes, d.Message)
			}
		}
	}
	return notes
}
