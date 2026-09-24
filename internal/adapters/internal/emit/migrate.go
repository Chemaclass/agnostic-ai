package emit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

// MergeJSONFile reads the existing JSON or JSONC at path (when not in
// dry-run mode), sets every key in `keys` on the decoded document, and
// writes the merged result back to path. A missing or empty file starts
// from an empty document; a non-empty file that will not parse is an
// error, and nothing is written.
//
// Used by adapters that share a project-shared config file with other
// tooling (opencode.json `mcp`, `.amp/settings.json` `amp.mcpServers`)
// and need to overwrite only their managed keys without clobbering
// user-edited siblings. Capture mode (sync --check, doctor, status)
// still reads the existing file: the on-disk document is part input
// (the user's sibling keys), not a pure output, so the captured bytes
// must reflect what sync would write. Skipping the read reported false
// drift and let `doctor --fix` delete the user's keys (#465).
//
// Comments do not survive: the document is re-rendered from parsed
// values, so a JSONC input keeps every key and loses every comment.
// The user hears about that once, on the sync that drops them (#725).
func (s *Session) MergeJSONFile(path string, keys map[string]any, dryRun bool) error {
	return s.mergeJSONFile(path, keys, nil, dryRun)
}

// MergeJSONFileNested merges the named object keys one level deep while
// replacing every other managed key. It is for settings objects where the
// adapter owns a portable child such as model.name but must preserve native
// sibling fields in the same object.
func (s *Session) MergeJSONFileNested(path string, keys map[string]any, nestedKeys []string, dryRun bool) error {
	nested := make(map[string]bool, len(nestedKeys))
	for _, key := range nestedKeys {
		nested[key] = true
	}
	return s.mergeJSONFile(path, keys, nested, dryRun)
}

// ExistingNestedStrings returns the string list at <key>.<child> in the
// JSON or JSONC document at path. A missing file, key, or child yields
// nil. Adapters use it to carry a user's own list entries into a key
// they then rewrite, since the merge replaces a whole array.
func (s *Session) ExistingNestedStrings(path, key, child string, dryRun bool) []string {
	doc, err := s.readExistingJSON(path, dryRun)
	if err != nil {
		return nil
	}
	raw, found := doc.Get(key)
	if !found {
		return nil
	}
	parent := map[string]any{}
	if err := json.Unmarshal(raw, &parent); err != nil {
		return nil
	}
	return StringSlice(parent[child])
}

func (s *Session) mergeJSONFile(path string, keys map[string]any, nested map[string]bool, dryRun bool) error {
	doc, err := s.readExistingJSON(path, dryRun)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(keys))
	for k := range keys {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		value := keys[k]
		if nested[k] {
			value = mergeJSONObject(doc, k, value)
		}
		if err := doc.Set(k, value); err != nil {
			return fmt.Errorf("marshal %s key %s: %w", path, k, err)
		}
	}
	raw, err := MarshalJSONIndent(doc)
	if err != nil {
		return fmt.Errorf("marshal %s: %w", path, err)
	}
	return s.WriteFile(path, string(raw)+"\n", dryRun)
}

func mergeJSONObject(doc *OrderedJSON, key string, value any) any {
	incoming, ok := value.(map[string]any)
	if !ok {
		return value
	}
	existing := map[string]any{}
	if raw, found := doc.Get(key); found {
		_ = json.Unmarshal(raw, &existing)
	}
	for child, childValue := range incoming {
		existing[child] = childValue
	}
	return existing
}

// readExistingJSON parses path as an OrderedJSON, accepting JSONC.
// Source key order is preserved on the round-trip via OrderedJSON.
// dryRun returns empty so `--dry-run` previews stay pure; capture mode
// still reads so drift detection and `doctor --fix` see the user's
// sibling keys (#465).
//
// An unreadable or empty file returns an empty document so callers can
// layer their managed keys unconditionally. Anything else that fails to
// parse returns an error: the caller's next move is to write the file,
// and returning empty there means writing the managed keys over the
// top of content nobody could read. That silent replacement is what
// deleted user keys from every JSONC config (#725).
func (s *Session) readExistingJSON(path string, dryRun bool) (*OrderedJSON, error) {
	if dryRun {
		return NewOrderedJSON(), nil
	}
	data, err := os.ReadFile(path)
	if err != nil || len(bytes.TrimSpace(data)) == 0 {
		return NewOrderedJSON(), nil
	}
	stripped, hadComments := StripJSONC(data)
	doc := NewOrderedJSON()
	if err := json.Unmarshal(stripped, doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w (agnostic-ai will not overwrite a file it cannot read; fix it or move it aside)", path, err)
	}
	if hadComments && !s.IsCapturing() {
		_, _ = fmt.Fprintf(Warner,
			"%s: comments are not preserved across a sync; every key is kept, the comments are dropped\n", path)
	}
	return doc, nil
}

// ProvenanceMarker is the substring agnostic-ai writes into every
// generated document's header (e.g. `Generated by agnostic-ai`).
// Adapters use it to distinguish a previously generated file from a
// user-authored one before performing migration renames. Aliased to
// header.Marker so both packages share one source of truth.
const ProvenanceMarker = header.Marker

// MigrateLegacyFile renames a previously generated legacy file aside
// so the user notices the rename to a new default file name. Used by
// adapters whose default output filename changed across releases
// (`amp`: AGENT.md -> AGENTS.md, `warp`: WARP.md -> AGENTS.md).
//
// Behavior:
//   - Skipped when dryRun is true.
//   - Skipped while capture mode is active so `sync --check` and
//     `revert` do not mutate the working tree.
//   - Skipped when the legacy file is missing or does not carry the
//     agnostic-ai provenance marker (it is treated as user-authored
//     and left untouched).
//   - Skipped when the legacy path is user-owned (sync.unmanaged).
//   - Looks for the legacy file alongside the configured output
//     directory derived from cfg.Outputs[target].File (or the default
//     output path), so a custom output location migrates correctly.
//
// Errors are swallowed (logged nowhere) to keep this call site's
// existing void contract for amp and warp; a target that needs the
// error propagated, or whose legacy and new paths sit in different
// directories, should call MigrateLegacyPath directly instead.
func (s *Session) MigrateLegacyFile(cfg *config.Config, target, legacyName, defaultNewPath string, dryRun bool) {
	legacyPath := legacyFilePath(cfg, target, legacyName, defaultNewPath)
	_ = s.MigrateLegacyPath(target, legacyPath, defaultNewPath, dryRun)
}

// MigrateLegacyPath is MigrateLegacyFile for a legacy path that is
// already fully resolved, rather than a bare filename joined against
// the new default's own directory. Use it when the legacy and current
// paths sit in different directories, which legacyFilePath's
// same-directory join cannot express (antigravity's
// `.agent/AGENTS.md` -> `.agents/AGENTS.md`, #1114).
//
// Both halves of the move go through session-owned writes, not a bare
// os.Rename, so a StartTransaction / Rollback pair spanning the rest of
// the sync pass undoes it cleanly: the backup is a brand-new file
// (removed on rollback) and the legacy file's removal is
// transaction-logged with its prior bytes (restored on rollback). A
// caller with no open transaction sees the same net effect
// MigrateLegacyFile always had.
//
// The backup itself writes through writeUntrackedFile, not WriteFile:
// it carries the legacy file's own provenance marker (it is a byte
// copy), so recording it into the sync ledger the way an ordinary
// generated output is recorded left it looking, to the next sync, like
// output this run stopped writing. Antigravity's own migration no
// longer touches `.agent/AGENTS.md` on that next run (it is already
// gone), so the ledger never re-records the backup, and the orphan
// sweep deleted a user's preserved bytes as if they were a stale
// generated file (#1114 review). The pre-Session `os.Rename` version
// of this migration (amp, warp) never had that failure mode, since a
// bare rename touches no bookkeeping at all; writeUntrackedFile
// restores that guarantee while keeping the write transaction-aware.
//
// Skipped, like MigrateLegacyFile, on dryRun, during capture mode, when
// the legacy file is missing or carries no provenance marker, and when
// sync.unmanaged matches the legacy path. An existing `<legacyPath>.bak`
// is never overwritten and the legacy file is never simply left in
// place either: leaving it in place while the loop above no longer
// declares it a managed output was itself a data-loss bug (a file
// present in an older, already-persisted sync ledger but absent from
// this run's output set reads as an orphan to the next run's sweep,
// which deletes it on the strength of its own provenance marker,
// discarding whatever bytes it held that were not yet in any backup,
// #1114 review). MigrateLegacyPath instead numbers past a taken name
// (`.bak`, `.bak.1`, `.bak.2`, ...) until it finds one that is neither
// on disk nor sync.unmanaged, and always removes the legacy file once
// its current bytes are safely parked there. Exhausting the search
// bound (1000 names) is the one case that still leaves the legacy file
// in place, with a warning; that ceiling is not expected to bite in
// practice.
// A real I/O failure on the backup write or the legacy removal is
// returned, not swallowed, so a caller wired into a transaction can
// roll the whole sync pass back instead of leaving a half-moved file.
func (s *Session) MigrateLegacyPath(target, legacyPath, defaultNewPath string, dryRun bool) error {
	if dryRun || s.IsCapturing() {
		return nil
	}
	data, err := os.ReadFile(legacyPath)
	if err != nil || !bytes.Contains(data, []byte(ProvenanceMarker)) || s.skipUnmanaged(legacyPath) {
		return nil
	}
	backupPath, err := s.uniqueUntrackedBackupPath(legacyPath)
	if err != nil {
		return err
	}
	if backupPath == "" {
		_, _ = fmt.Fprintf(Warner, "%s: could not find a free backup name for %s after %d attempts; leaving it in place\n",
			target, legacyPath, maxUntrackedBackupAttempts)
		return nil
	}
	if err := s.writeUntrackedFile(backupPath, string(data), dryRun); err != nil {
		return fmt.Errorf("backup %s: %w", legacyPath, err)
	}
	if _, err := s.RemoveOwned(legacyPath, "", dryRun); err != nil {
		return fmt.Errorf("remove %s: %w", legacyPath, err)
	}
	newName := filepath.Base(defaultNewPath)
	_, _ = fmt.Fprintf(Warner, "%s: renamed legacy %s to %s; new layout writes %s\n",
		target, legacyPath, filepath.Base(backupPath), newName)
	return nil
}

// maxUntrackedBackupAttempts bounds the numbered-suffix search in
// uniqueUntrackedBackupPath so a pathological pile of prior `.bak.N`
// files (or an unmanaged pattern matching all of them) cannot loop
// forever.
const maxUntrackedBackupAttempts = 1000

// uniqueUntrackedBackupPath returns the first of `<legacyPath>.bak`,
// `<legacyPath>.bak.1`, `<legacyPath>.bak.2`, ... that is neither
// already on disk nor sync.unmanaged, so MigrateLegacyPath can always
// park a legacy file's current bytes somewhere instead of discarding
// them when an earlier backup already claims the conventional name.
// "" (with a nil error) means every candidate within the search bound
// collided.
func (s *Session) uniqueUntrackedBackupPath(legacyPath string) (string, error) {
	for i := 0; i < maxUntrackedBackupAttempts; i++ {
		candidate := legacyPath + ".bak"
		if i > 0 {
			candidate = fmt.Sprintf("%s.%d", candidate, i)
		}
		if s.IsUnmanaged(candidate) {
			continue
		}
		if _, err := os.Stat(candidate); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return "", fmt.Errorf("stat %s: %w", candidate, err)
		}
		return candidate, nil
	}
	return "", nil
}

// legacyFilePath resolves legacyName against the directory holding the
// target's current entry-point output, honoring an outputs.<target>.file
// override the same way MigrateLegacyFile's rename target does.
func legacyFilePath(cfg *config.Config, target, legacyName, defaultNewPath string) string {
	rootDir := filepath.Dir(OutputFile(cfg, target, defaultNewPath))
	if rootDir == "." || rootDir == "" {
		return legacyName
	}
	return filepath.Join(rootDir, legacyName)
}

// WarnIfLegacyFileOutranksEntryPoint fires when legacyName exists next to
// the resolved entry-point file without the agnostic-ai provenance marker.
// MigrateLegacyFile already leaves such a file untouched, since it reads
// as real user content, but for a vendor whose CLI reads the legacy
// filename ahead of the new default that silence hides a total delivery
// failure rather than a harmless leftover: every rule sync just wrote to
// defaultNewPath never reaches the tool.
//
// Warp is the motivating case: "If both WARP.md and AGENTS.md exist in
// the same directory, WARP.md takes priority"
// (docs.warp.dev/agents/capabilities/rules, target-audit 2026-09-08,
// #691). Callers opt in per target; amp's AGENT.md is a documented
// fallback read only when AGENTS.md is absent, so it never calls this.
//
// Skipped while capture mode is active: `sync` itself runs every
// adapter once in capture mode first (collision detection) and once for
// real, and only the real pass should print. `sync --check` / `status`
// / `revert` never leave capture mode, so this stays silent there too,
// the same tradeoff MigrateLegacyFile's own rename already makes.
func (s *Session) WarnIfLegacyFileOutranksEntryPoint(cfg *config.Config, target, legacyName, defaultNewPath string) {
	if s.IsCapturing() {
		return
	}
	legacyPath := legacyFilePath(cfg, target, legacyName, defaultNewPath)
	data, err := os.ReadFile(legacyPath)
	if err != nil || bytes.Contains(data, []byte(ProvenanceMarker)) {
		return
	}
	newName := filepath.Base(defaultNewPath)
	_, _ = fmt.Fprintf(Warner, "%s: %s takes priority over %s; it is not agnostic-ai-generated, so none of the synced rules reach %s until you rename or remove it\n",
		target, legacyPath, newName, target)
}
