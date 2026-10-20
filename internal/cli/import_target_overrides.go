package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// importScopedTargetKeys returns a key the spec at dst sets under
// `x-<target>`, and that sync therefore wrote at the top level of the
// native agent file, to `x-<target>`. Without it, import would make the
// key shared and every other tool would get it on the next sync. The
// spec's shared value for that key, when it has one, comes back in its
// place.
//
// The block is rebuilt from the spec's own, so its order and comments
// stay, and a value the native file did not change keeps the spec's
// node. A scoped key the native file no longer holds was deleted there
// and is dropped, unless it is a delete marker (`null`). Keys in keep,
// which the target never writes or which its import scopes itself, stay
// as the spec has them. A file whose spec scopes none of its keys comes
// back unchanged.
func importScopedTargetKeys(data []byte, dst, target string, keep ...string) ([]byte, error) {
	return importScopedTargetKeysKeeping(data, dst, target, keep, nil, false)
}

// importScopedTargetKeysKeeping is importScopedTargetKeys where a scoped
// key for which unwritten reports true is one the native file has no
// place for: missing there, it was not deleted, so it stays. With
// overwrite, the write replaces the spec instead of merging into it, so
// such a key has to be written back into the block.
func importScopedTargetKeysKeeping(data []byte, dst, target string, keep []string, unwritten func(string) bool, overwrite bool) ([]byte, error) {
	if unwritten == nil {
		unwritten = func(string) bool { return false }
	}
	existing, err := os.ReadFile(dst)
	if errors.Is(err, fs.ErrNotExist) {
		return data, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", dst, err)
	}
	key := "x-" + target
	have, _, ok := frontmatterNode(existing)
	if !ok {
		return data, nil
	}
	scoped := mappingValue(have, key)
	if scoped == nil || scoped.Kind != yaml.MappingNode {
		return data, nil
	}
	doc, rest, ok := frontmatterNode(data)
	if !ok {
		if strings.HasPrefix(string(data), "---") {
			return data, nil
		}
		doc, rest = &yaml.Node{Kind: yaml.MappingNode}, "---\n\n"+string(data)
	}
	mapping := doc
	native := mappingValue(mapping, key)
	if native != nil && native.Kind != yaml.MappingNode {
		return data, nil
	}
	noteUnseenScopedKeys(scoped, mapping, native, dst, key, keep, unwritten)
	changed := false
	for i := 0; i+1 < len(scoped.Content); i += 2 {
		k, was := scoped.Content[i].Value, scoped.Content[i+1]
		if slices.Contains(keep, k) || isNullNode(was) || mappingValue(native, k) != nil {
			continue
		}
		if mappingValue(mapping, k) == nil && unwritten(k) && !overwrite {
			continue
		}
		changed = true
	}
	if !changed {
		return data, nil
	}
	block := &yaml.Node{Kind: yaml.MappingNode, Style: scoped.Style &^ yaml.FlowStyle, HeadComment: scoped.HeadComment, LineComment: scoped.LineComment, FootComment: scoped.FootComment}
	for i := 0; i+1 < len(scoped.Content); i += 2 {
		k, was := scoped.Content[i], scoped.Content[i+1]
		switch top, inBlock := mappingValue(mapping, k.Value), mappingValue(native, k.Value); {
		case slices.Contains(keep, k.Value):
			block.Content = append(block.Content, k, was)
		case inBlock != nil:
			block.Content = append(block.Content, k, specOrNative(was, inBlock, k.Value))
		case top != nil && !isNullNode(was):
			block.Content = append(block.Content, k, specOrNative(was, top, k.Value))
			removeOrShare(mapping, k.Value, have)
		case isNullNode(was) || top == nil && unwritten(k.Value):
			block.Content = append(block.Content, k, was)
		}
	}
	if native != nil {
		for i := 0; i+1 < len(native.Content); i += 2 {
			if mappingValue(block, native.Content[i].Value) == nil {
				block.Content = append(block.Content, native.Content[i], native.Content[i+1])
			}
		}
	}
	if len(block.Content) == 0 {
		block.Style = yaml.FlowStyle
	}
	setMappingNode(mapping, key, block)
	return encodeFrontmatter(mapping, rest, target)
}

// specOrNative returns the spec's node when the native value means the
// same, so its comments, order and style stay, and the native node,
// with the spec's comments, when it changed. A tool list the native
// file writes as one comma-separated string, as Qoder does, matches the
// spec's list.
func specOrNative(was, native *yaml.Node, key string) *yaml.Node {
	var a, b any
	if was.Decode(&a) == nil && native.Decode(&b) == nil {
		if reflect.DeepEqual(a, b) {
			return was
		}
		if text, ok := b.(string); ok && key == "tools" && reflect.DeepEqual(toStringSlice(a), spec.SplitToolList(text)) && len(toStringSlice(a)) > 0 {
			return was
		}
	}
	out := *native
	if out.LineComment == "" {
		out.LineComment = was.LineComment
	}
	if out.HeadComment == "" {
		out.HeadComment = was.HeadComment
	}
	return &out
}

// frontmatterNode returns the frontmatter mapping of a Markdown spec and
// the text from its closing `---` on.
func frontmatterNode(data []byte) (*yaml.Node, string, bool) {
	rest, ok := strings.CutPrefix(string(data), "---")
	end := strings.Index(rest, "\n---")
	if !ok || end < 0 {
		return nil, "", false
	}
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(rest[:end]), &doc); err != nil || doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, "", false
	}
	return doc.Content[0], strings.TrimPrefix(rest[end:], "\n"), true
}

func mappingValue(mapping *yaml.Node, key string) *yaml.Node {
	if mapping == nil {
		return nil
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}

// removeOrShare puts the spec's shared value back for key, or removes
// key when the spec shares none. A spec that shares `can` gets it back
// for a scoped `tools`.
func removeOrShare(mapping *yaml.Node, key string, have *yaml.Node) {
	sharedKey, shared := key, mappingValue(have, key)
	if shared == nil && key == "tools" {
		sharedKey, shared = "can", mappingValue(have, "can")
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value != key {
			continue
		}
		if shared != nil {
			mapping.Content[i].Value = sharedKey
			mapping.Content[i+1] = shared
		} else {
			mapping.Content = append(mapping.Content[:i], mapping.Content[i+2:]...)
		}
		return
	}
}

func isNullNode(n *yaml.Node) bool {
	return n.Kind == yaml.ScalarNode && n.Tag == "!!null"
}

// specScopesKey reports whether the spec at dst sets key under
// `x-<target>`, so the native value under that key came from there and
// a translation meant for a shared value must leave it alone.
func specScopesKey(dst, target, key string) bool {
	data, err := os.ReadFile(dst)
	if err != nil {
		return false
	}
	have, _, ok := frontmatterNode(data)
	return ok && mappingValue(mappingValue(have, "x-"+target), key) != nil
}

// importScopeTarget is the source the running importer reads, such as
// cursor. Set per source by runImportSource. Sequential use only.
var importScopeTarget string

// importScopedSpecKeys applies importScopedTargetKeys for the running
// source to a spec an importer writes at path, so a skill, rule,
// command, or agent keeps the keys its spec scopes under x-<source>.
//
// unwritten reports a key the target's native file cannot hold, from the
// importer's field set, so a scoped one missing there is not taken as
// deleted. Nil treats every key as written.
func importScopedSpecKeys(data []byte, path string, unwritten func(string) bool, overwrite bool) ([]byte, error) {
	if importScopeTarget == "" {
		return data, nil
	}
	return importScopedTargetKeysKeeping(data, path, importScopeTarget, scopedKeysOwned(importScopeTarget, path), unwritten, overwrite)
}

// scopedKeysOwned lists the x-<target> keys of the spec at path that an
// importer handles itself, so the scoped write-back leaves them as the
// spec has them: Claude's readonly, which preserveClaudeReadonly
// reconciles, and Cursor's agent tools, which Cursor never writes.
func scopedKeysOwned(target, path string) []string {
	switch target {
	case "claude":
		return []string{"readonly"}
	case "cursor":
		if filepath.Base(filepath.Dir(path)) == "agents" {
			return []string{"tools"}
		}
	}
	return nil
}

// dropEmptyScopedBlock removes an empty x-<target> block an import left
// in data where the spec at path had one with keys: every key in it was
// deleted from the native file. A merge-based import would otherwise
// carry the spec's block back.
func dropEmptyScopedBlock(data []byte, path, target string) ([]byte, error) {
	if target == "" {
		return data, nil
	}
	mapping, rest, ok := frontmatterNode(data)
	if !ok {
		return data, nil
	}
	key := "x-" + target
	block := mappingValue(mapping, key)
	if block == nil || block.Kind != yaml.MappingNode || len(block.Content) > 0 {
		return data, nil
	}
	existing, err := os.ReadFile(path)
	if err != nil {
		return data, nil
	}
	have, _, ok := frontmatterNode(existing)
	if !ok {
		return data, nil
	}
	if was := mappingValue(have, key); was == nil || was.Kind != yaml.MappingNode || len(was.Content) == 0 {
		return data, nil
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			mapping.Content = append(mapping.Content[:i], mapping.Content[i+2:]...)
			break
		}
	}
	return encodeFrontmatter(mapping, rest, target)
}

// importScopedRename renames the top-level native key, such as Gemini's
// `mcp_servers`, to specKey when the spec at dst scopes specKey under
// x-<target> and not native: sync wrote the scoped value under the
// tool's own name, so the write-back can then return it to x-<target>.
func importScopedRename(data []byte, dst, target, native, specKey string) ([]byte, error) {
	if !specScopesKey(dst, target, specKey) || specScopesKey(dst, target, native) {
		return data, nil
	}
	mapping, rest, ok := frontmatterNode(data)
	if !ok || mappingValue(mapping, specKey) != nil {
		return data, nil
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == native {
			mapping.Content[i].Value = specKey
			return encodeFrontmatter(mapping, rest, target)
		}
	}
	return data, nil
}

// noteUnseenScopedKeys prints one note for a spec whose scoped keys the
// native file has no place for: import keeps them, since it cannot see a
// change to them there.
func noteUnseenScopedKeys(scoped, mapping, native *yaml.Node, dst, key string, keep []string, unwritten func(string) bool) {
	var unseen []string
	for i := 0; i+1 < len(scoped.Content); i += 2 {
		k := scoped.Content[i].Value
		if slices.Contains(keep, k) || isNullNode(scoped.Content[i+1]) || mappingValue(mapping, k) != nil || mappingValue(native, k) != nil || !unwritten(k) {
			continue
		}
		unseen = append(unseen, key+"."+k)
	}
	if len(unseen) == 0 {
		return
	}
	shown := dst
	if wd, err := os.Getwd(); err == nil {
		real := func(p string) string {
			if r, err := filepath.EvalSymlinks(p); err == nil {
				return r
			}
			return p
		}
		if rel, err := filepath.Rel(real(wd), real(dst)); err == nil && !strings.HasPrefix(rel, "..") {
			shown = rel
		}
	}
	pronoun := "it"
	if len(unseen) > 1 {
		pronoun = "them"
	}
	importPendingKeptNote = fmt.Sprintf("note: %s keeps %s, which the %s file does not carry, so import cannot see a change to %s there; edit the spec to change or remove %s\n",
		filepath.ToSlash(shown), strings.Join(unseen, ", "), strings.TrimPrefix(key, "x-"), pronoun, pronoun)
}

// importPendingKeptNote is the note noteUnseenScopedKeys built for the
// spec being written, and importNativeSource the native file the running
// importer read it from, when it says. Sequential use only.
var (
	importPendingKeptNote string
	importNativeSource    string
)

// importReadingNative records path as the native file the next spec
// write comes from.
func importReadingNative(path string) { importNativeSource = path }

// flushKeptNote prints the pending kept-key note when the native file
// changed since the last sync wrote it, and clears both. Repeating the
// note on every import would bury the ones that matter. Without a known
// source, a write that changes the spec at path stands in for it.
func flushKeptNote(path string, data []byte) {
	note, source := importPendingKeptNote, importNativeSource
	importPendingKeptNote, importNativeSource = "", ""
	if note == "" {
		return
	}
	if source != "" {
		if raw, err := os.ReadFile(source); err == nil && unchangedSinceSync(source, string(raw)) {
			return
		}
	} else if existing, err := os.ReadFile(path); err == nil && sameSpecMarkdown(existing, data) {
		return
	}
	keptf("%s", note)
}

// importCarrying returns the unwritten test for a native format that
// holds only the keys in carryable.
func importCarrying(carryable []string) func(string) bool {
	return func(key string) bool { return !slices.Contains(carryable, key) }
}
