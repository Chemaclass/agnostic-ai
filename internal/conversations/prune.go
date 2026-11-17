package conversations

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// markSource names the store a complete read listed a marked
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

// marksKept is how long a mark outlives its conversation, counted from
// the first complete read of its source that missed it, or from a mark
// made since.
const marksKept = 30 * 24 * time.Hour

// inventory is every conversation id one source holds, from a read with
// no error, limit, filter or cancel.
type inventory struct {
	source markSource
	tool   Tool
	ids    map[string]bool
}

// provenBy is the inventory of a complete read, or none when ids is nil.
func provenBy(source markSource, tool Tool, ids map[string]bool) []inventory {
	if ids == nil {
		return nil
	}
	return []inventory{{source: source, tool: tool, ids: ids}}
}

// listTree is filepath.Glob(root/*/.../pattern) with levels folders of
// "*", and says whether every folder could be read: Glob skips one it
// cannot open without an error.
func listTree(root string, levels int, pattern string) (files []string, complete bool) {
	complete = true
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
				path := filepath.Join(dir, entry.Name())
				if level == levels {
					if matched, _ := filepath.Match(pattern, entry.Name()); matched {
						files = append(files, path)
					}
					continue
				}
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

// pruneMarks records, for each mark, the provable source a complete read
// listed it in, starts its clock on the first complete read of that
// source that misses it, and drops it once marksKept has passed. A
// source this read did not list completely changes nothing, and a mark
// no complete read ever listed is kept. It reports whether file changed.
// The caller holds the marks lock.
func (s *Scanner) pruneMarks(file *marksFile, results []sourceResult, now time.Time) (changed bool) {
	var complete map[markSource]inventory
	for _, result := range results {
		for _, inv := range result.inventories {
			if complete == nil {
				complete = map[markSource]inventory{}
			}
			complete[inv.source] = inv
		}
	}
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
		case next.Source != entry.Source || next.MissingSince != entry.MissingSince:
			file.Marks[key] = next
			changed = true
		}
	}
	return changed
}

// attributeMark records the source the last complete read listed key
// in, so the scan after a new mark need not write the file for that.
func (s *Scanner) attributeMark(key string, entry *markEntry) {
	if entry.Source != "" {
		return
	}
	s.lastReadMu.Lock()
	results := s.lastRead.results
	s.lastReadMu.Unlock()
	tool, id, _ := strings.Cut(key, ":")
	for _, result := range results {
		for _, inv := range result.inventories {
			if inv.tool == Tool(tool) && inv.ids[id] {
				entry.Source = inv.source
				return
			}
		}
	}
}

func agedMark(entry markEntry, tool Tool, id string, complete map[markSource]inventory, now time.Time) (markEntry, bool) {
	if entry.Source == "" {
		for source, inv := range complete {
			if inv.tool == tool && inv.ids[id] {
				entry.Source = source
				break
			}
		}
	}
	inv, read := complete[entry.Source]
	switch {
	case !read || inv.tool != tool:
		return entry, true
	case inv.ids[id]:
		entry.MissingSince = ""
		return entry, true
	case entry.MissingSince == "":
		entry.MissingSince = stamp(now)
		return entry, true
	}
	return entry, now.Sub(lastMarked(entry)) < marksKept
}

// lastMarked is the later of when the conversation went missing and the
// person's latest mark on it.
func lastMarked(entry markEntry) time.Time {
	latest := parseStamp(entry.MissingSince)
	for _, at := range []string{entry.SeenAt, entry.CompletedAt, entry.HiddenAt} {
		if marked := parseStamp(at); marked.After(latest) {
			latest = marked
		}
	}
	return latest
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
		"title": true, "source": true, "missingSince": true, "startedBy": true,
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
