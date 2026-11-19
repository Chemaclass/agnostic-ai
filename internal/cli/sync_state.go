package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

// syncStateVersion identifies the on-disk schema of `.agnostic-ai/.sync-state`.
// Bumped to 2 when the per-sync output ledger (Outputs) was added, and to 3
// when OutputSums and Orphans were added, to 4 when SpecSums was added,
// to 5 when Unledgered was added, to 6 when Merged was added, and to 7
// when Merged covered Aider's YAML config.
// Readers tolerate older versions by treating missing fields as zero
// values, and refuse a newer one.
const syncStateVersion = mergedYAMLLedgerVersion

// newerLedgerError is a state file a newer release wrote. Its paths or
// claims may mean something this release misreads, and acting on them
// can strip merged keys or delete a file it believes it created.
type newerLedgerError struct {
	path    string
	version int
}

func (e *newerLedgerError) Error() string {
	return e.reason() + "; upgrade agnostic-ai with `agnostic-ai upgrade`, then run the command again"
}

func (e *newerLedgerError) reason() string {
	return fmt.Sprintf("%s was written by a newer agnostic-ai (ledger version %d, this release reads up to %d)",
		filepath.ToSlash(e.path), e.version, syncStateVersion)
}

type syncStateFile struct {
	Version        int       `json:"version,omitempty"`
	SyncedAt       time.Time `json:"synced_at"`
	FilesChanged   int       `json:"files_changed"`
	WarningsDigest string    `json:"warnings_digest,omitempty"`
	// NotesDigest fingerprints the coverage notes from the previous
	// sync so the next run can sticky-suppress an unchanged set, the
	// same way WarningsDigest gates capability warnings.
	NotesDigest    string   `json:"notes_digest,omitempty"`
	WarningRecords []string `json:"warning_records,omitempty"`
	NoteRecords    []string `json:"note_records,omitempty"`
	// Outputs lists every file path the previous sync wrote (create or
	// update or skip), relative to the project root. The next sync uses
	// it to detect orphans: any path present in the prior ledger but
	// absent from the current write set is swept via the emit
	// session's RemoveGenerated. The header-guarded sweep skips
	// user-authored files. An empty list (older state files, or first
	// sync after upgrade) disables the sweep so projects without a
	// recorded baseline never lose files.
	Outputs []string `json:"outputs,omitempty"`
	// OutputSums maps each ledgered path to the sha256 of the bytes sync
	// wrote. The sweep removes a file without the provenance header only
	// while its bytes still match, so a hand-edited leftover survives
	// (#785), and `sync --check` reports a file whose bytes no longer
	// match as edited rather than out of date (#1270). Older state may
	// lack the sum of a file with the header; check reads such a file
	// as out of date.
	OutputSums map[string]string `json:"output_sums,omitempty"`
	// Orphans lists ledgered paths the last sync no longer emits but
	// could not remove (edited since, or no recorded sum). They stay in
	// Outputs so the next sync retries them, and `sync --check` and
	// `doctor` report them as drift while they remain on disk.
	Orphans []string `json:"orphans,omitempty"`
	// Unledgered lists leftovers a sync run without a prior ledger found
	// (unledgeredReport). No record proves sync wrote them, so they stay
	// out of Outputs and no sync removes them; `sync --check` and
	// `doctor` report them while they remain (#1354).
	Unledgered []string `json:"unledgered,omitempty"`
	// Merged records the ledgered JSON files sync merges into beside
	// keys it did not write. Once sync stops writing one, it takes out
	// only these keys instead of deleting the file (#1541).
	Merged map[string]mergedOutput `json:"merged,omitempty"`
	// SpecSums fingerprints each source spec and the merged config, so
	// the next sync can name which sources changed since this one.
	SpecSums map[string]string `json:"spec_sums,omitempty"`
	// SpecTargets lists the targets that emitted each spec in SpecSums.
	SpecTargets map[string][]string `json:"spec_targets,omitempty"`
	// SpecFileSums maps each spec file, slash-form and relative to the
	// project, to the sha256 of its raw bytes when sync rendered it or
	// import wrote it, so import can tell a spec a tool already reads
	// from one it would replace unseen (#1620).
	SpecFileSums map[string]specFileSum `json:"spec_file_sums,omitempty"`
	// ModelAliases records the id each vendor model alias resolved to,
	// by target, so the next sync can say when an upgrade moved one.
	ModelAliases map[string]map[string]string `json:"model_aliases,omitempty"`
	// Backups maps each `<path>.bak` sync made to the sum of its bytes, so
	// import can tell it from a skill's own asset while it is unchanged.
	Backups map[string]string `json:"backups,omitempty"`
	// Listed names the targets a sync has shown what they read. A project
	// whose first sync covered only some targets lists the others once
	// later; a project synced before this field lists none.
	// Written even when empty, so an empty list stays apart from a
	// ledger older than the field.
	Listed []string `json:"listed"`
	// PendingImports lists the tools `use` added to targets before their
	// own config was imported. A sync waits until `use` finishes them, so
	// it never writes over native config nothing has imported.
	PendingImports []string `json:"pending_imports,omitempty"`
}

// showRepeatedDrops lets -vv print capability warnings and coverage notes
// that match the previous sync. Watch mode clears it after its first pass.
var showRepeatedDrops = true

// syncLedger is the output footprint one sync persists to the state file.
type syncLedger struct {
	outputs []string
	sums    map[string]string
	orphans []string
	// unledgered lists the leftovers no ledger proves sync wrote.
	unledgered []string
	merged     map[string]mergedOutput
	// specSums is not part of the output footprint, but it is written
	// beside it so the next sync can diff sources against this one.
	specSums       map[string]string
	specTargets    map[string][]string
	specFileSums   map[string]specFileSum
	modelAliases   map[string]map[string]string
	backups        map[string]string
	listed         []string
	warningRecords []string
	noteRecords    []string
}

func stateFilePath(projectRoot string) string {
	return filepath.Join(projectRoot, ".agnostic-ai", ".sync-state")
}

// readStateFile loads the sync-state cache. A missing or corrupt file
// yields the zero value on purpose: the state is a self-healing cache the
// next sync rewrites, so an unreadable one means "no prior state" rather
// than a fatal error. Sync warns about a corrupt one (unparsableLedger).
func readStateFile(projectRoot string) syncStateFile {
	return readLedger(projectRoot).state
}

var readStateBytes = os.ReadFile

// emitAdapter is the entry point every render and write here takes, so a
// test can count renders per target.
var emitAdapter = adapters.EmitWithProvenance

// ledgerSnapshot is one read of the state file. The state file lists
// every output, so a sync or check reads it once and asks this copy.
// A file that does not parse keeps what decoded before the error.
type ledgerSnapshot struct {
	path     string
	state    syncStateFile
	absent   bool
	readErr  error
	parseErr error
}

func readLedger(root string) ledgerSnapshot {
	l := ledgerSnapshot{path: stateFilePath(root)}
	if _, err := os.Lstat(l.path); errors.Is(err, fs.ErrNotExist) {
		l.absent = true
		return l
	}
	data, err := readStateBytes(l.path)
	if err != nil {
		l.readErr = err
		return l
	}
	l.state, l.parseErr = decodeLedger(root, l.path, data)
	if newer := new(newerLedgerError); errors.As(l.parseErr, &newer) {
		l.readErr, l.parseErr = l.parseErr, nil
	}
	return l
}

// refuseNewer stops a command that acts on a state file a newer release
// wrote.
func (l ledgerSnapshot) refuseNewer() error {
	if newer := new(newerLedgerError); errors.As(l.readErr, &newer) {
		return newer
	}
	return nil
}

// warnNewer tells a command that only reports that a newer release
// wrote the state file, so the report may be wrong.
func (l ledgerSnapshot) warnNewer() {
	if newer := new(newerLedgerError); errors.As(l.readErr, &newer) {
		_, _ = fmt.Fprintf(ledgerWarnOut, "warning: %s; this report may be wrong until you upgrade agnostic-ai with `agnostic-ai upgrade`\n", newer.reason())
	}
}

// decodeLedger is every read of the state file: entries outside the
// project dropped, each path in the spelling the config's own paths get.
// A file that does not parse keeps what decoded before the error.
func decodeLedger(root, path string, data []byte) (syncStateFile, error) {
	var s syncStateFile
	err := json.Unmarshal(data, &s)
	s.dropEscapingEntries(root, path)
	if s.Version > syncStateVersion {
		return s, &newerLedgerError{path: path, version: s.Version}
	}
	s.mergeSpellings(root, config.CleanOutputPath)
	return s, err
}

// missing is ledgerMissing for this read.
func (l ledgerSnapshot) missing() bool {
	return l.absent || l.readErr == nil && (l.parseErr != nil || l.state.SyncedAt.IsZero())
}

// workdir is the working directory's ledger, which the leftover scan
// and the shared-skills cleanup read whatever root a pass syncs.
func (l ledgerSnapshot) workdir(root string) ledgerSnapshot {
	if filepath.Clean(root) == "." {
		return l
	}
	return readLedger(".")
}

// unparsable is unparsableLedger for this read.
func (l ledgerSnapshot) unparsable() error {
	if l.absent || l.readErr != nil || l.parseErr == nil {
		return nil
	}
	return fmt.Errorf("%s does not parse (%v)", filepath.ToSlash(l.path), l.parseErr)
}

// checkError fails `sync --check` when it cannot read the ledger,
// which would otherwise hide every leftover the ledger listed.
func (l ledgerSnapshot) checkError() error {
	if l.readErr != nil {
		return fmt.Errorf("read sync ledger: %w", l.readErr)
	}
	if err := l.unparsable(); err != nil {
		return fmt.Errorf("%w; run agnostic-ai sync to rebuild the ledger", err)
	}
	return nil
}

// warn tells the user sync treats a ledger that does not parse as missing.
func (l ledgerSnapshot) warn() {
	if err := l.unparsable(); err != nil {
		fmt.Fprintf(os.Stderr, "! %v; treating the ledger as missing until a sync writes a new one\n", err)
	}
}

// A nil frame state masks outer snapshots while history changes directories.
type priorStateFrame struct {
	state    *syncStateFile
	previous *priorStateFrame
	released bool
}

var heldState struct {
	sync.Mutex
	frame *priorStateFrame
}

// holdStateFile reads the state file under root once and serves the
// prior-key hooks from that copy until release runs. The state file
// lists every output, and a command that emits each spec alone asks the
// hooks on every emit. Only a command that never writes the state file
// may hold it; sync holds its own copy through holdPriorState.
func holdStateFile(root string) (release func()) {
	return holdState(readStateFile(root))
}

// holdPriorState serves the prior-key hooks from prev, the state file a
// sync pass read before its first write, until release runs. The pass
// writes the state file only after its last emit, so it must release
// before that write. The hooks read the working directory's state file,
// so a pass on another root holds nothing.
func holdPriorState(root string, prev syncStateFile) (release func()) {
	if filepath.Clean(root) != "." {
		return func() {}
	}
	return holdState(prev)
}

func holdState(s syncStateFile) (release func()) {
	return holdStateFrame(&s, false)
}

func holdCaptureState(s syncStateFile) (release func()) {
	return holdStateFrame(&s, true)
}

func isolatePriorState() (release func()) {
	return holdStateFrame(nil, false)
}

func holdStateFrame(state *syncStateFile, borrow bool) (release func()) {
	heldState.Lock()
	if borrow && heldState.frame != nil && heldState.frame.state != nil {
		heldState.Unlock()
		return func() {}
	}
	frame := &priorStateFrame{state: state, previous: heldState.frame}
	heldState.frame = frame
	heldState.Unlock()
	return func() {
		heldState.Lock()
		frame.released = true
		for heldState.frame != nil && heldState.frame.released {
			heldState.frame = heldState.frame.previous
		}
		heldState.Unlock()
	}
}

// priorStateFile is the held state file, or else the one on disk.
// Callers must not modify the result.
func priorStateFile() syncStateFile {
	heldState.Lock()
	var held *syncStateFile
	if heldState.frame != nil {
		held = heldState.frame.state
	}
	heldState.Unlock()
	if held != nil {
		return *held
	}
	return readStateFile(".")
}

// readStateFileStrict is readStateFile that reports a state file that
// exists but cannot be read or parsed, for a caller about to write it.
func readStateFileStrict(projectRoot string) (syncStateFile, error) {
	p := stateFilePath(projectRoot)
	data, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return syncStateFile{}, nil
	}
	if err != nil {
		return syncStateFile{}, fmt.Errorf("read %s: %w", p, err)
	}
	s, err := decodeLedger(projectRoot, p, data)
	if newer := new(newerLedgerError); errors.As(err, &newer) {
		return s, err
	}
	if err != nil {
		return s, fmt.Errorf("parse %s: %w", p, err)
	}
	return s, nil
}

func writeStateFile(projectRoot string, filesChanged int, warningsDigest, notesDigest string, ledger syncLedger) error {
	return replaceStateFile(projectRoot, syncStateFile{
		Version:        syncStateVersion,
		SyncedAt:       time.Now().UTC(),
		FilesChanged:   filesChanged,
		WarningsDigest: warningsDigest,
		NotesDigest:    notesDigest,
		WarningRecords: ledger.warningRecords,
		NoteRecords:    ledger.noteRecords,
		Outputs:        ledger.outputs,
		OutputSums:     ledger.sums,
		Orphans:        ledger.orphans,
		Unledgered:     ledger.unledgered,
		Merged:         ledger.merged,
		SpecSums:       ledger.specSums,
		SpecTargets:    ledger.specTargets,
		SpecFileSums:   ledger.specFileSums,
		ModelAliases:   ledger.modelAliases,
		Backups:        ledger.backups,
		Listed:         ledger.listed,
	})
}

// recordOutputSums merges sums of files written outside a sync, such as
// by `doctor --fix`, into the state file, so the next check does not
// read them as hand edits. With no readable state file there is no
// ledger to update; the next sync writes one.
func recordOutputSums(projectRoot string, sums map[string]string) error {
	if len(sums) == 0 {
		return nil
	}
	p := stateFilePath(projectRoot)
	data, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	s, err := decodeLedger(projectRoot, p, data)
	if newer := new(newerLedgerError); errors.As(err, &newer) {
		return err
	}
	if err != nil {
		return nil
	}
	if s.OutputSums == nil {
		s.OutputSums = map[string]string{}
	}
	maps.Copy(s.OutputSums, sums)
	return replaceStateFile(projectRoot, s)
}

// unparsableLedger reports a state file that exists but does not parse,
// as a full disk or a stop mid-write leaves it. Such a ledger counts as
// missing (ledgerMissing) and the next sync replaces it.
func unparsableLedger(root string) error {
	return readLedger(root).unparsable()
}
