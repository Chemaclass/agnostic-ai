package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
)

// mergedLedgerVersion is the first .sync-state version that records the
// keys of merged JSON files, and mergedYAMLLedgerVersion the first that
// records them for Aider's YAML config. An older ledger cannot tell
// sync's keys from the user's in such a file.
const (
	mergedLedgerVersion     = 6
	mergedYAMLLedgerVersion = 7
)

// The merge writers read what the last sync claimed in a merged file
// from the ledger, so an entry a spec no longer sets leaves and the
// user's own entries stay. Capture reads the same ledger, so check
// renders what sync writes.
func init() {
	adapters.SetPriorMergedKeys(func(path string) []adapters.MergedKey {
		return slices.Clone(priorStateFile().Merged[path].Keys)
	})
	adapters.SetPriorOutputSum(func(path string) string {
		return priorStateFile().OutputSums[path]
	})
}

// mergedOutput is what the ledger keeps about a JSON file sync merges
// into: the values it set there, and whether sync created the file.
// Unrecorded marks a file an older sync wrote before keys were recorded:
// it stays until `doctor --fix` removes it. Released holds this run's
// given-up key paths until ledgerMerged drops them; it is never stored,
// and neither is whether this run wrote the file merged or whole.
type mergedOutput struct {
	Keys       []adapters.MergedKey `json:"keys,omitempty"`
	Created    bool                 `json:"created,omitempty"`
	Unrecorded bool                 `json:"unrecorded,omitempty"`
	Released   [][]string           `json:"-"`
	wroteMerge bool
	wroteWhole bool
}

// recordMergedWrites adds the merged JSON writes in writes to merged.
// Targets that write one shared file each add the keys they set. A hand
// edit --keep-edits left in place was not written, so it keeps the
// record the last sync stored.
func recordMergedWrites(writes []adapters.WrittenFile, merged map[string]mergedOutput) {
	for _, w := range writes {
		switch w.Action {
		case "create", "update", "skip":
		default:
			continue
		}
		m := merged[w.Path]
		if !w.Merged {
			m.wroteWhole = true
			merged[w.Path] = m
			continue
		}
		m.Keys = withMergedKeys(m.Keys, w.Keys)
		m.Released = append(m.Released, w.Released...)
		m.Created = m.Created || w.Action == "create"
		m.wroteMerge = true
		merged[w.Path] = m
	}
}

// withMergedKeys adds keys to base, a later key replacing one with the
// same path, sorted by path.
func withMergedKeys(base, keys []adapters.MergedKey) []adapters.MergedKey {
	out := slices.Clone(base)
	for _, key := range keys {
		i := slices.IndexFunc(out, func(k adapters.MergedKey) bool { return slices.Equal(k.Path, key.Path) })
		if i >= 0 {
			out[i] = key
			continue
		}
		out = append(out, key)
	}
	slices.SortFunc(out, func(a, b adapters.MergedKey) int { return slices.Compare(a.Path, b.Path) })
	return out
}

// ledgerMerged returns the merged-file records to store for ledger. A
// merge sets only the keys the specs produce now and leaves the others
// on disk, so a key an earlier sync set stays claimed until a merge
// removes it or leaves it to the user. A file sync created stays
// created, and so does one an older ledger listed without a record:
// once only sync's keys are left, there is nothing of the user's to
// keep. released overrides the record of a file the sweep kept.
//
// A file an older ledger lists that this run did not write, such as a
// skipped target's on a partial sync, is marked unrecorded, so a later
// sweep keeps it once the ledger no longer reads as older. A file sync
// writes whole drops that mark.
func ledgerMerged(ledger []string, merged map[string]mergedOutput, written map[string]string, prev syncStateFile, released map[string]mergedOutput) map[string]mergedOutput {
	out := map[string]mergedOutput{}
	for _, p := range ledger {
		if m, ok := released[p]; ok {
			out[p] = m
			continue
		}
		current := merged[p]
		last, recorded := prev.Merged[p]
		_, writtenNow := written[p]
		switch {
		case current.wroteMerge:
		case current.wroteWhole:
			// Sync owns the whole file now, so the claims are stale.
			continue
		case recorded && (!writtenNow || !last.Unrecorded):
			out[p] = last
			continue
		case !writtenNow && unrecordedMergedCandidate(prev, p):
			out[p] = mergedOutput{Unrecorded: true}
			continue
		default:
			continue
		}
		var kept []adapters.MergedKey
		for _, key := range last.Keys {
			if !slices.ContainsFunc(current.Released, func(path []string) bool { return isKeyPrefix(path, key.Path) }) {
				kept = append(kept, key)
			}
		}
		created := current.Created || last.Created || !recorded && slices.Contains(prev.Outputs, p)
		out[p] = mergedOutput{Keys: withMergedKeys(kept, followedKeys(current.Keys, last.Keys)), Created: created}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// followedKeys returns the keys this run claims. A key a cleanup
// rewrote (Follows set) takes over the earlier claim at its path only
// when that claim's sum is the one the cleanup started from, so sync
// still owns the whole value. Otherwise the earlier claim stands.
func followedKeys(current, last []adapters.MergedKey) []adapters.MergedKey {
	out := make([]adapters.MergedKey, 0, len(current))
	for _, key := range current {
		if key.Follows == "" {
			out = append(out, key)
			continue
		}
		i := slices.IndexFunc(last, func(k adapters.MergedKey) bool { return slices.Equal(k.Path, key.Path) })
		if i < 0 || last[i].Items != nil || last[i].Sum != key.Follows {
			continue
		}
		key.Follows = ""
		out = append(out, key)
	}
	return out
}

// isKeyPrefix reports whether prefix names key or an object holding it.
func isKeyPrefix(prefix, key []string) bool {
	return len(prefix) <= len(key) && slices.Equal(prefix, key[:len(prefix)])
}

// unrecordedMergedCandidate reports whether path, which an older ledger
// lists without a key record, may hold the user's keys: a JSON file
// with no provenance header, or an Aider config.
func unrecordedMergedCandidate(prev syncStateFile, path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json", ".jsonc":
		if prev.Version >= mergedLedgerVersion {
			return false
		}
		data, err := os.ReadFile(path)
		return err == nil && !header.Has(string(data))
	case ".yml", ".yaml":
		return prev.Version < mergedYAMLLedgerVersion && looksLikeAiderConf(path)
	}
	return false
}

// looksLikeAiderConf reports whether the YAML file at path may be an
// Aider config sync merged into: Aider's file name, or one of the keys
// sync sets there. Keeping a whole-written YAML file by mistake only
// leaves it for `doctor --fix`.
func looksLikeAiderConf(path string) bool {
	switch strings.ToLower(filepath.Base(path)) {
	case ".aider.conf.yml", ".aider.conf.yaml":
		return true
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var doc map[string]any
	if yaml.Unmarshal(data, &doc) != nil {
		return false
	}
	for _, key := range []string{"read", "model", "weak-model"} {
		if _, ok := doc[key]; ok {
			return true
		}
	}
	return false
}

// releaseMergedOrphans hands back the merged files the last sync
// wrote and this one does not. It takes out the keys sync set there and
// keeps the rest, which a whole-file sweep would delete with them
// (#1541). A value the user edited since stays, and so does a file an
// older ledger lists without a key record; both are kept orphans, with
// the record to store for them in records. released lists every path it
// dealt with, so the sweep leaves them alone. A file it cannot read is
// kept, and anything but a regular file is left to the sweep.
func releaseMergedOrphans(sess *adapters.Session, prev syncStateFile, current []string) (released, removed, stripped, kept []string, records map[string]mergedOutput, err error) {
	records = map[string]mergedOutput{}
	pruned := make(map[string]bool)
	for _, p := range prev.Outputs {
		if slices.Contains(current, p) || underSymlinkedDir(p) {
			continue
		}
		if fi, statErr := os.Lstat(p); statErr != nil || !fi.Mode().IsRegular() {
			continue
		}
		m, ok := prev.Merged[p]
		if !ok {
			if unrecordedMergedCandidate(prev, p) && !sess.IsUnmanaged(p) {
				released = append(released, p)
				kept = append(kept, p)
				records[p] = mergedOutput{Unrecorded: true}
			}
			continue
		}
		if sess.KeepsEdits() && editedSince(p, prev.OutputSums[p]) {
			continue
		}
		if m.Unrecorded {
			released = append(released, p)
			kept = append(kept, p)
			continue
		}
		result, edited, err := sess.ReleaseMerged(p, m.Keys, m.Created, false, false)
		if err != nil {
			// Left out of released, so the failed sweep keeps its record.
			return released, removed, stripped, kept, records, err
		}
		released = append(released, p)
		switch result {
		case adapters.MergedRemoved:
			removed = append(removed, p)
			pruneAncestorDirs(p, pruned)
		case adapters.MergedStripped:
			stripped = append(stripped, p)
		case adapters.MergedEdited:
			kept = append(kept, p)
			records[p] = mergedOutput{Keys: edited, Created: m.Created}
		case adapters.MergedKept:
			if !sess.IsUnmanaged(p) {
				kept = append(kept, p)
			}
		}
	}
	return released, removed, stripped, kept, records, nil
}

// keptOrphanReason says why the sweep kept path.
func keptOrphanReason(merged map[string]mergedOutput, priorSums map[string]string, path string) string {
	if m, ok := merged[path]; ok {
		switch {
		case m.Unrecorded:
			return "an older sync wrote it before recording which keys are its own"
		case !adapters.ParsesMerged(path):
			return "it does not parse, so sync's keys cannot be taken out"
		default:
			return "you edited a value sync set; the other keys sync set are gone"
		}
	}
	if priorSums[path] == "" {
		return "the sync that wrote it recorded no checksum"
	}
	return "edited since sync"
}

// recordMergedRelease updates the ledger after `doctor --fix` released
// merged files. A nil record means sync let go of the file: it leaves
// the ledger. Any other record replaces the stored one, and the path
// stays a kept orphan.
func recordMergedRelease(root string, records map[string]*mergedOutput) error {
	if len(records) == 0 {
		return nil
	}
	return updateStateFile(root, func(state *syncStateFile) {
		releaseMergedRecords(state, records)
	})
}

// recordMergedFixes stores the claims of the merged files `doctor --fix`
// wrote, folded into the stored records the way a sync folds them.
func recordMergedFixes(root string, fixes map[string]mergedOutput, written map[string]string) error {
	if len(fixes) == 0 {
		return nil
	}
	return updateStateFile(root, func(state *syncStateFile) {
		paths := make([]string, 0, len(fixes))
		for path := range fixes {
			paths = append(paths, path)
		}
		records := ledgerMerged(paths, fixes, written, *state, nil)
		for _, path := range paths {
			record, ok := records[path]
			if !ok {
				delete(state.Merged, path)
				continue
			}
			if state.Merged == nil {
				state.Merged = map[string]mergedOutput{}
			}
			state.Merged[path] = record
		}
	})
}

// updateStateFile applies update to the stored ledger. With no readable
// ledger there is nothing to update; the next sync writes one.
func updateStateFile(root string, update func(*syncStateFile)) error {
	p := stateFilePath(root)
	data, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	var state syncStateFile
	if err := json.Unmarshal(data, &state); err != nil {
		return fmt.Errorf("parse %s: %w", p, err)
	}
	update(&state)
	out, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("%s: %w", p, err)
	}
	if err := os.WriteFile(p, out, 0o644); err != nil {
		return fmt.Errorf("%s: %w", p, err)
	}
	return nil
}

func releaseMergedRecords(state *syncStateFile, records map[string]*mergedOutput) {
	for path, record := range records {
		if record == nil {
			state.Outputs = removeMatching(state.Outputs, []string{path})
			state.Orphans = removeMatching(state.Orphans, []string{path})
			delete(state.OutputSums, path)
			delete(state.Merged, path)
			continue
		}
		if state.Merged == nil {
			state.Merged = map[string]mergedOutput{}
		}
		state.Merged[path] = *record
		if !slices.Contains(state.Orphans, path) {
			state.Orphans = finalizeLedger(append(state.Orphans, path))
		}
	}
}
