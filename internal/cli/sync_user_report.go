package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"
)

// userFileRecord is a fileRecord with the key-level edits a user
// settings or MCP file carries.
type userFileRecord struct {
	fileRecord
	Keys []string `json:"keys,omitempty"`
}

func emitUserJSON(cmd *cobra.Command, command string, records []userFileRecord, drops syncDrops) error {
	if records == nil {
		records = []userFileRecord{}
	}
	return writeIndentedJSON(cmd, struct {
		Version string           `json:"version"`
		Command string           `json:"command"`
		Writes  []userFileRecord `json:"writes"`
		Skipped []fileRecord     `json:"skipped"`
		Errors  []errorRecord    `json:"errors"`
		syncDrops
	}{"1", command, records, []fileRecord{}, []errorRecord{}, drops})
}

// userFileRecords describes each file a run would change, in the
// project sync schema: create, update, or delete, or with check the
// drift words missing, stale, and leftover. A user settings or MCP file
// lists its key-level edits, and a conflict is marked in them.
func userFileRecords(writes []userWrite, removals []string, statePath string, next userState, check bool) []userFileRecord {
	create, update, remove := "create", "update", "delete"
	if check {
		create, update, remove = "missing", "stale", "leftover"
	}
	out := []userFileRecord{}
	var staleState *userFileRecord
	for _, w := range writes {
		action := update
		// Check counts the state file as drift only when its ownership
		// differs beside a changed tool file; a plan reports the bytes a
		// sync writes.
		if check && w.path == statePath {
			if userStateMissing(statePath) {
				action = create
			}
			if !userStateCurrent(statePath, next) {
				staleState = &userFileRecord{fileRecord: fileRecord{Path: filepath.ToSlash(w.path), Action: action, Bytes: len(w.data)}}
			}
			continue
		}
		data, err := os.ReadFile(w.path)
		if err == nil && bytes.Equal(data, w.data) {
			continue
		}
		if err != nil {
			action = create
		}
		keys := slices.Clone(w.changes)
		for _, c := range w.conflicts {
			keys = append(keys, "conflict: "+c)
		}
		out = append(out, userFileRecord{fileRecord: fileRecord{Target: strings.Join(w.targets, ","), Path: filepath.ToSlash(w.path), Action: action, Bytes: len(w.data)}, Keys: keys})
	}
	for _, path := range removals {
		out = append(out, userFileRecord{fileRecord: fileRecord{Path: filepath.ToSlash(path), Action: remove}})
	}
	if staleState != nil && len(out) > 0 {
		out = append(out, *staleState)
	}
	return out
}

// printUserPlan prints one line per file a sync would change, with its
// key-level edits under it, and writes nothing.
func printUserPlan(cmd *cobra.Command, records []userFileRecord) error {
	out := cmd.OutOrStdout()
	if len(records) == 0 {
		_, err := fmt.Fprintln(out, "No changes.")
		return err
	}
	for _, r := range records {
		target := ""
		if r.Target != "" {
			target = " [" + r.Target + "]"
		}
		if _, err := fmt.Fprintf(out, "%s %s%s\n", r.Action, r.Path, target); err != nil {
			return fmt.Errorf("write plan: %w", err)
		}
		for _, key := range r.Keys {
			if _, err := fmt.Fprintf(out, "  %s\n", key); err != nil {
				return fmt.Errorf("write plan: %w", err)
			}
		}
	}
	return nil
}

// refuseUserOverwrites stops a sync that would overwrite what the user
// changed: a key conflict, a hand edit, or a file's comments.
func refuseUserOverwrites(writes []userWrite, removals []string, old userState, source string) error {
	var conflicts []string
	for _, w := range writes {
		for _, c := range w.conflicts {
			conflicts = append(conflicts, w.path+": "+c)
		}
	}
	if len(conflicts) > 0 {
		return fmt.Errorf("%s; or rerun with --backup to overwrite and keep a .bak copy", strings.Join(conflicts, "; "))
	}
	edited, err := handEditedUserFiles(writes, removals, old)
	if err != nil {
		return err
	}
	if len(edited) > 0 {
		return fmt.Errorf("edited since the last global sync: %s; move the edit into %s, or rerun with --backup to overwrite and keep a .bak copy", strings.Join(edited, ", "), source)
	}
	var commented []string
	for _, w := range writes {
		if w.dropsComments {
			commented = append(commented, w.path)
		}
	}
	if len(commented) > 0 {
		return fmt.Errorf("rewriting %s would drop its comments; rerun with --backup to rewrite and keep a .bak copy", strings.Join(commented, ", "))
	}
	return nil
}

// printUserAdoptions names each file and settings key a dry run would
// take over because it already holds what sync writes.
func printUserAdoptions(cmd *cobra.Command, adopted []string, writes []userWrite) error {
	for _, line := range append(adopted, settingsAdoptions(writes)...) {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "dry-run: adopt %s\n", line); err != nil {
			return fmt.Errorf("write dry-run output: %w", err)
		}
	}
	return nil
}

// settingsAdoptions names each user settings key that already held the
// value sync writes, as "<path> <key>".
func settingsAdoptions(writes []userWrite) []string {
	var out []string
	for _, w := range writes {
		for _, key := range w.adopted {
			out = append(out, w.path+" "+key)
		}
	}
	return out
}

// checkUserWrites fails when a planned write differs from disk or a
// sync would remove a file, naming each one.
func checkUserWrites(cmd *cobra.Command, writes []userWrite, removals []string, statePath string, next userState, diff bool) error {
	drifted, err := userDrift(cmd, writes, removals, statePath, next, diff)
	if err != nil {
		return err
	}
	if len(drifted) > 0 {
		return fmt.Errorf("global configuration drift: %s", strings.Join(drifted, ", "))
	}
	return nil
}

// userDrift returns every planned write that differs from disk and
// every file a sync would remove. With diff it prints a unified diff per
// file, limited to the managed block when both sides carry one.
func userDrift(cmd *cobra.Command, writes []userWrite, removals []string, statePath string, next userState, diff bool) ([]string, error) {
	var drifted []string
	var stateStale bool
	out := cmd.OutOrStdout()
	for _, w := range writes {
		if w.path == statePath {
			stateStale = !userStateCurrent(statePath, next)
			continue
		}
		data, err := os.ReadFile(w.path)
		if err == nil && bytes.Equal(data, w.data) {
			continue
		}
		drifted = append(drifted, w.path)
		if !diff {
			continue
		}
		if err != nil {
			if _, werr := fmt.Fprintf(out, "would create %s (%d bytes)\n", filepath.ToSlash(w.path), len(w.data)); werr != nil {
				return nil, fmt.Errorf("write global diff: %w", werr)
			}
			continue
		}
		have, want := userBlock(data), userBlock(w.data)
		if have == want {
			have, want = string(data), string(w.data)
		}
		if _, werr := fmt.Fprint(out, unifiedDiff(w.path, have, want)); werr != nil {
			return nil, fmt.Errorf("write global diff: %w", werr)
		}
	}
	for _, path := range removals {
		drifted = append(drifted, path)
		if diff {
			if _, err := fmt.Fprintf(out, "would remove %s\n", filepath.ToSlash(path)); err != nil {
				return nil, fmt.Errorf("write global diff: %w", err)
			}
		}
	}
	if stateStale && len(drifted) == 0 {
		if _, err := fmt.Fprintf(cmd.ErrOrStderr(), "note: %s\n", userOwnershipNote(statePath)); err != nil {
			return nil, fmt.Errorf("write global check: %w", err)
		}
	}
	if stateStale && len(drifted) > 0 {
		drifted = append(drifted, statePath)
		if diff {
			verb := "update"
			if userStateMissing(statePath) {
				verb = "create"
			}
			if _, err := fmt.Fprintf(out, "would %s ownership state %s\n", verb, filepath.ToSlash(statePath)); err != nil {
				return nil, fmt.Errorf("write global diff: %w", err)
			}
		}
	}
	return drifted, nil
}

// existingPaths keeps the paths still on disk, the only ones a removal
// changes.
func existingPaths(paths []string) []string {
	var out []string
	for _, path := range paths {
		if _, err := os.Lstat(path); err == nil {
			out = append(out, path)
		}
	}
	return out
}

// userOwnershipNote explains an ownership record that is missing or
// out of date while every tool file matches. Recording it changes no tool
// file, so check counts it as drift only beside a file that changes.
func userOwnershipNote(path string) string {
	record := "the ownership record at " + filepath.ToSlash(path) + " is out of date"
	if userStateMissing(path) {
		record = "no ownership record at " + filepath.ToSlash(path) + " yet"
	}
	return record + "; until `agnostic-ai sync --scope user` writes it, removing a spec leaves its output in place"
}

// handEditedUserFiles lists owned files whose content no longer
// matches what the last sync wrote and that this run would overwrite or
// remove. A file whose record predates sums is never listed.
func handEditedUserFiles(writes []userWrite, removals []string, old userState) ([]string, error) {
	var edited []string
	check := func(path string, planned []byte) error {
		recorded, ok := old.Sums[path]
		if !ok {
			return nil
		}
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		current := userSum(data)
		if current != recorded && (planned == nil || current != userSum(planned)) {
			edited = append(edited, path)
		}
		return nil
	}
	for _, w := range writes {
		if err := check(w.path, w.data); err != nil {
			return nil, err
		}
	}
	for _, path := range removals {
		if err := check(path, nil); err != nil {
			return nil, err
		}
	}
	return edited, nil
}
