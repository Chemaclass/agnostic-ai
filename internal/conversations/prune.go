package conversations

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// markSource names the kind of store a complete read listed a marked
// conversation in.
type markSource string

const (
	sourceClaudeCode  markSource = "claude-code"
	sourceCodex       markSource = "codex"
	sourceCursorAgent markSource = "cursor-agent"
)

// provable are the sources whose reader can list every conversation its
// store holds. The Claude app's cached sidebar, the ChatGPT catalog,
// Cursor's filtered editor chat index and the bounded cloud lists
// cannot, so absence from them proves no deletion.
var provable = map[markSource]bool{sourceClaudeCode: true, sourceCodex: true, sourceCursorAgent: true}

const (
	// marksKept is how long complete reads must miss a conversation
	// before its mark goes.
	marksKept = 30 * 24 * time.Hour
	// missGap is the longest pause between two complete misses that still
	// counts as watching; after a longer one, the marksKept span begins
	// again, so a machine that was off for a month prunes nothing on its
	// first read back.
	missGap = 7 * 24 * time.Hour
	// missRefresh is how often a missing mark's last miss is saved.
	missRefresh = 24 * time.Hour
)

// inventory is every conversation one store holds, from a read with no
// error, limit, filter or cancel. Several servers may share the marks
// file while each reads its own store, so a mark ages only on reads of
// the store that listed it.
type inventory struct {
	source markSource
	tool   Tool
	// root is the store's folder, and store a short name for it.
	root  string
	store string
	// files maps each conversation id to its own data: its transcript,
	// rollout or chat folder, relative to root when inside it.
	files map[string]string
}

// provenBy is the inventory of a complete read, or none when files is
// nil. A read that lists nothing proves nothing either: an empty folder
// can be a store that moved or a disk that is not mounted.
func provenBy(source markSource, tool Tool, root string, files map[string]string) []inventory {
	if len(files) == 0 {
		return nil
	}
	return []inventory{{source: source, tool: tool, root: root, store: storeKey(root), files: files}}
}

// storeKey names a store's folder in the marks file without its path.
func storeKey(root string) string {
	if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}
	sum := sha256.Sum256([]byte(filepath.Clean(root)))
	return hex.EncodeToString(sum[:6])
}

// inStore is path relative to root when it lies inside it.
func inStore(root, path string) string {
	if rel, err := filepath.Rel(root, path); err == nil && filepath.IsLocal(rel) {
		return rel
	}
	return path
}

// listTree is filepath.Glob(root/*/.../pattern) with levels folders of
// "*", and says whether every folder could be read: Glob skips one it
// cannot open without an error.
func listTree(root string, levels int, pattern string) (files []string, complete bool) {
	complete = true
	match := nameMatcher(pattern)
	dirs := []string{root}
	for level := 0; level <= levels; level++ {
		var next []string
		for _, dir := range dirs {
			entries, err := os.ReadDir(dir)
			if err != nil {
				complete = false
				continue
			}
			for _, entry := range entries {
				if level == levels {
					if match(entry.Name()) {
						files = append(files, filepath.Join(dir, entry.Name()))
					}
					continue
				}
				path := filepath.Join(dir, entry.Name())
				isDir := entry.IsDir()
				if entry.Type()&os.ModeSymlink != 0 {
					info, err := os.Stat(path)
					if err != nil {
						complete = false
						continue
					}
					isDir = info.IsDir()
				}
				if isDir {
					next = append(next, path)
				}
			}
		}
		dirs = next
	}
	return files, complete
}

// nameMatcher is filepath.Match for one name, with a plain comparison
// for a literal name or a "*" and suffix: Match costs most of a listing
// of thousands of names (see BenchmarkListTranscripts).
func nameMatcher(pattern string) func(name string) bool {
	const meta = `*?[\\`
	if suffix, ok := strings.CutPrefix(pattern, "*"); ok && !strings.ContainsAny(suffix, meta) {
		return func(name string) bool { return strings.HasSuffix(name, suffix) }
	}
	if !strings.ContainsAny(pattern, meta) {
		return func(name string) bool { return name == pattern }
	}
	return func(name string) bool {
		matched, _ := filepath.Match(pattern, name)
		return matched
	}
}

// pruneMarks records, for each mark, the store a complete read listed it
// in and where its data sits, and ages it on complete reads of that
// store that miss it. It drops a mark that such reads missed for
// marksKept, without a pause longer than missGap, once a check shows its
// own data is gone. A read of another store, or one that is not
// complete, changes nothing, and a mark no complete read ever listed is
// kept. It reports whether file changed. The caller holds the marks lock.
func (s *Scanner) pruneMarks(file *marksFile, results []sourceResult, now time.Time) (changed bool) {
	complete := completeStores(results)
	if complete == nil {
		return false
	}
	for key, entry := range file.Marks {
		tool, id, _ := strings.Cut(key, ":")
		next, keep := agedMark(entry, Tool(tool), id, complete, now)
		switch {
		case !keep:
			delete(file.Marks, key)
			delete(s.appArchives, key)
			changed = true
		case next != entry:
			file.Marks[key] = next
			changed = true
		}
	}
	return changed
}

// completeStores indexes the complete inventories of results by store.
func completeStores(results []sourceResult) map[string]inventory {
	var complete map[string]inventory
	for _, result := range results {
		for _, inv := range result.inventories {
			if complete == nil {
				complete = map[string]inventory{}
			}
			complete[inv.store] = inv
		}
	}
	return complete
}

// attributeMark records the store the last complete read listed key in,
// so the scan after a new mark need not write the file for that.
func (s *Scanner) attributeMark(key string, entry *markEntry) {
	if entry.Store != "" {
		return
	}
	s.lastReadMu.Lock()
	results := s.lastRead.results
	s.lastReadMu.Unlock()
	tool, id, _ := strings.Cut(key, ":")
	attribute(entry, Tool(tool), id, completeStores(results))
}

// attribute records the store among complete that lists id, if any.
func attribute(entry *markEntry, tool Tool, id string, complete map[string]inventory) {
	for _, inv := range complete {
		if file, listed := inv.files[id]; listed && inv.tool == tool {
			entry.Source, entry.Store, entry.File = inv.source, inv.store, file
			return
		}
	}
}

func agedMark(entry markEntry, tool Tool, id string, complete map[string]inventory, now time.Time) (markEntry, bool) {
	if entry.Store == "" {
		attribute(&entry, tool, id, complete)
	}
	inv, read := complete[entry.Store]
	if !read || inv.tool != tool || inv.source != entry.Source {
		return entry, true
	}
	if file, listed := inv.files[id]; listed {
		entry.MissingSince, entry.LastMissAt = "", ""
		if file != "" {
			entry.File = file
		}
		return entry, true
	}
	started, last := parseStamp(entry.MissingSince), parseStamp(entry.LastMissAt)
	switch {
	case started.IsZero() || last.IsZero() || now.Sub(last) > missGap:
		entry.MissingSince, entry.LastMissAt = stamp(now), stamp(now)
	case now.Sub(started) >= marksKept:
		if dataGone(inv.root, entry.File) {
			return entry, false
		}
		entry.MissingSince, entry.LastMissAt = "", ""
	case now.Sub(last) >= missRefresh:
		entry.LastMissAt = stamp(now)
	}
	return entry, true
}

// dataGone reports whether a conversation's own data is gone from its
// store: one stat, made only when its mark would go. An unknown path or
// a failed stat proves nothing.
func dataGone(root, file string) bool {
	if file == "" {
		return false
	}
	if !filepath.IsAbs(file) {
		file = filepath.Join(root, file)
	}
	_, err := os.Lstat(file)
	return errors.Is(err, fs.ErrNotExist)
}

// The marks file keeps the fields it does not know, so a version that
// reads a file a newer one wrote does not drop that version's fields.

type markEntryFields markEntry

type marksFileFields marksFile

var (
	// markEntryKeys also holds startedBy, which a development build saved
	// and no release reads, so a write drops it.
	markEntryKeys = map[string]bool{
		"seenAt": true, "completedAt": true, "hiddenAt": true, "closedWrittenAt": true,
		"title": true, "source": true, "store": true, "file": true, "missingSince": true, "lastMissAt": true, "startedBy": true,
	}
	marksFileKeys = map[string]bool{"schemaVersion": true, "since": true, "marks": true}
)

func (e markEntry) MarshalJSON() ([]byte, error) {
	return withFields(markEntryFields(e), e.extra)
}

// decodeMarks reads a file of known fields only in one strict pass, as
// fast as a plain decode; see BenchmarkDecodeMarks. Only a file with
// unknown fields pays for a second pass that keeps them.
func decodeMarks(data []byte, f *marksFile) error {
	*f = marksFile{}
	strict := json.NewDecoder(bytes.NewReader(data))
	strict.DisallowUnknownFields()
	if strict.Decode((*marksFileFields)(f)) == nil {
		if _, err := strict.Token(); err == io.EOF {
			return nil
		}
	}
	*f = marksFile{}
	if err := json.Unmarshal(data, (*marksFileFields)(f)); err != nil {
		return err
	}
	f.extra = unknownFields(data, marksFileKeys)
	var entries struct {
		Marks map[string]json.RawMessage `json:"marks"`
	}
	if err := json.Unmarshal(data, &entries); err != nil {
		return err
	}
	for key, raw := range entries.Marks {
		if extra := unknownFields(raw, markEntryKeys); extra != "" {
			entry := f.Marks[key]
			entry.extra = extra
			f.Marks[key] = entry
		}
	}
	return nil
}

func (f marksFile) MarshalJSON() ([]byte, error) {
	return withFields(marksFileFields(f), f.extra)
}

// unknownJSON is a JSON object of the fields a reader did not know, or
// empty. A string keeps the struct that holds it comparable.
type unknownJSON string

// unknownFields are the fields of the object data whose keys are not
// in known.
func unknownFields(data []byte, known map[string]bool) unknownJSON {
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil {
		return ""
	}
	for key := range fields {
		if known[key] {
			delete(fields, key)
		}
	}
	if len(fields) == 0 {
		return ""
	}
	kept, err := json.Marshal(fields)
	if err != nil {
		return ""
	}
	return unknownJSON(kept)
}

// withFields encodes known with the extra fields beside it. A known
// field wins over an extra one of the same key.
func withFields(known any, extra unknownJSON) ([]byte, error) {
	data, err := json.Marshal(known)
	if err != nil || extra == "" {
		return data, err
	}
	fields := map[string]json.RawMessage{}
	if err := json.Unmarshal([]byte(extra), &fields); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	return json.Marshal(fields)
}
