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
	// missRefresh is how often the missing marks' last miss is saved. A
	// saved miss can be this much older than the real one, so a pause is
	// measured against missGap plus this.
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
	root = resolvedFolder(root)
	return []inventory{{source: source, tool: tool, root: root, store: storeKey(root), files: files}}
}

// resolvedFolder is root as an absolute path with its links followed,
// or as an absolute path when they cannot be.
func resolvedFolder(root string) string {
	if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}
	if real, err := filepath.EvalSymlinks(root); err == nil {
		root = real
	}
	return filepath.Clean(root)
}

// storeKey names a store's folder in the marks file without its path.
func storeKey(root string) string {
	sum := sha256.Sum256([]byte(resolvedFolder(root)))
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
	p := prunePass{complete: complete, roots: file.Stores, gone: map[string]bool{}, now: now}
	p.due = p.dueStores(file.Marks)
	for key, entry := range file.Marks {
		tool, id, _ := strings.Cut(key, ":")
		next, keep := p.aged(entry, Tool(tool), id)
		switch {
		case !keep:
			delete(file.Marks, key)
			delete(s.appArchives, key)
			changed = true
		case next != entry:
			file.Marks[key] = next
			changed = true
		}
		if keep && recordRoot(file, next.Store, complete) {
			changed = true
		}
	}
	return changed
}

// prunePass is one pass of pruneMarks.
type prunePass struct {
	complete map[string]inventory
	// roots are the folders of the stores marks name, as the file keeps them.
	roots map[string]string
	// gone holds, by store, whether its folder is gone, read once a pass.
	gone map[string]bool
	// due are the stores whose missing marks this pass saves together.
	due map[string]bool
	now time.Time
}

// recordRoot keeps the folder of a complete store a mark names, so a
// later pass can tell when that folder is gone.
func recordRoot(file *marksFile, store string, complete map[string]inventory) bool {
	inv, read := complete[store]
	if !read || file.Stores[store] == inv.root {
		return false
	}
	if file.Stores == nil {
		file.Stores = map[string]string{}
	}
	file.Stores[store] = inv.root
	return true
}

// missing classifies a mark that a complete read of its store missed:
// restart when its clock must begin now, else whether a save is due.
func (p *prunePass) missing(entry markEntry) (restart, due bool) {
	started, last := parseStamp(entry.MissingSince), parseStamp(entry.LastMissAt)
	if started.IsZero() || last.IsZero() || p.now.Sub(last) > missGap+missRefresh {
		return true, true
	}
	return false, p.now.Sub(last) >= missRefresh
}

// dueStores are the stores with a missing mark that this pass writes
// anyway. Each of their missing marks is saved in that same write, so
// marks that went missing at different times of day cost one write a
// day, not one each.
func (p *prunePass) dueStores(marks map[string]markEntry) map[string]bool {
	due := map[string]bool{}
	for key, entry := range marks {
		inv, read := p.complete[entry.Store]
		if !read || due[entry.Store] || inv.source != entry.Source {
			continue
		}
		tool, id, _ := strings.Cut(key, ":")
		if _, listed := inv.files[id]; listed || inv.tool != Tool(tool) {
			continue
		}
		if _, saveDue := p.missing(entry); saveDue {
			due[entry.Store] = true
		}
	}
	return due
}

// moved reports whether a mark's store is not read here, a complete read
// here lists its conversation, and the folder of its store is gone: the
// store moved, as after a home folder was renamed. Two stores that both
// exist and hold the same chat, such as a copied folder, keep the first.
func (p *prunePass) moved(entry markEntry, tool Tool, id string) bool {
	if _, read := p.complete[entry.Store]; read {
		return false
	}
	listed := false
	for _, inv := range p.complete {
		if _, ok := inv.files[id]; ok && inv.tool == tool {
			listed = true
			break
		}
	}
	root, known := p.roots[entry.Store]
	if !listed || !known {
		return false
	}
	gone, checked := p.gone[entry.Store]
	if !checked {
		_, err := os.Lstat(root)
		gone = errors.Is(err, fs.ErrNotExist)
		p.gone[entry.Store] = gone
	}
	return gone
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
func (s *Scanner) attributeMark(file *marksFile, key string, entry *markEntry) {
	if entry.Store != "" {
		return
	}
	s.lastReadMu.Lock()
	results := s.lastRead.results
	s.lastReadMu.Unlock()
	tool, id, _ := strings.Cut(key, ":")
	complete := completeStores(results)
	attribute(entry, Tool(tool), id, complete)
	recordRoot(file, entry.Store, complete)
}

// attribute records the store among complete that lists id, if any.
func attribute(entry *markEntry, tool Tool, id string, complete map[string]inventory) {
	for _, inv := range complete {
		if file, listed := inv.files[id]; listed && inv.tool == tool {
			entry.Source, entry.Store, entry.File = inv.source, inv.store, file
			entry.MissingSince, entry.LastMissAt = "", ""
			return
		}
	}
}

func (p *prunePass) aged(entry markEntry, tool Tool, id string) (markEntry, bool) {
	if entry.Store == "" || p.moved(entry, tool, id) {
		attribute(&entry, tool, id, p.complete)
	}
	inv, read := p.complete[entry.Store]
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
	restart, _ := p.missing(entry)
	switch {
	case restart:
		entry.MissingSince, entry.LastMissAt = stamp(p.now), stamp(p.now)
	case p.now.Sub(parseStamp(entry.MissingSince)) >= marksKept:
		if dataGone(inv.root, entry.File) {
			return entry, false
		}
		entry.MissingSince, entry.LastMissAt = "", ""
	case p.due[entry.Store]:
		entry.LastMissAt = stamp(p.now)
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
		"title": true, "twinShownAt": true, "source": true, "store": true, "file": true, "missingSince": true, "lastMissAt": true, "startedBy": true,
	}
	marksFileKeys = map[string]bool{"schemaVersion": true, "since": true, "marks": true, "stores": true, "historySince": true}
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
