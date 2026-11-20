package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"reflect"
	"slices"

	"gopkg.in/yaml.v3"
)

// importWriteSpecMarkdown writes an imported markdown spec to path,
// carrying over the frontmatter keys only the spec already on disk
// declares. Most targets emit a subset of a spec's frontmatter, so a
// byte-for-byte write back would delete every key that target cannot
// express (`argument-hint` and `allowed-tools` on a cursor skill, for
// one). A missing destination is a plain write. A destination whose
// frontmatter and body parse the same as the result is left as written.
//
// fields names what the target's native file can hold: a key it can
// hold but the import does not carry was deleted on purpose and is not
// restored. Rules pass through importWriteFile instead, because their
// translators drop a catch-all `globs` and an empty `description` on
// purpose (#429) and the spec has to follow.
func importWriteSpecMarkdown(path string, data []byte, mode fs.FileMode, fields specFields) error {
	return importWriteSpecMarkdownCarrying(path, data, mode, fields, func(key string) bool { return !fields.expresses(key) && !fields.scopedNative[key] })
}

// importWriteSpecMarkdownCarrying is importWriteSpecMarkdown for a
// native format that holds fewer keys than fields expresses: unwritten
// reports the ones it cannot hold, which the scoped write-back keeps.
func importWriteSpecMarkdownCarrying(path string, data []byte, mode fs.FileMode, fields specFields, unwritten func(string) bool) error {
	overwrite := fields.all && len(fields.omitted) == 0 && fields.preserve == nil
	var err error
	if !fields.scopedReconciled {
		data, err = importScopedSpecKeys(data, path, unwritten, overwrite)
		if err != nil {
			return err
		}
	}
	existing, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		importPendingKeptNote, importNativeSource = "", ""
		if data, err = reconcileImportedModel(nil, data, path, importedHasKey(data, "model")); err != nil {
			return fmt.Errorf("merge %s: %w", path, err)
		}
		return importWriteFile(path, data, mode)
	case err != nil:
		return fmt.Errorf("read %s: %w", path, err)
	}
	// An import that reconciles its own x-<target> block, as Claude's
	// does, drops an emptied one before the merge so it can.
	if fields.omitted["x-"+importScopeTarget] {
		if data, err = dropEmptyScopedBlock(data, path, importScopeTarget); err != nil {
			return err
		}
	}
	if !fields.all || len(fields.omitted) > 0 || fields.preserve != nil {
		data, err = mergeSpecFrontmatter(existing, data, fields)
		if err != nil {
			return fmt.Errorf("merge %s: %w", path, err)
		}
	}
	if data, err = reconcileTargetMap(existing, data, "effort", importModelTargets(importScopeTarget), fields.expresses("effort")); err != nil {
		return fmt.Errorf("merge %s: %w", path, err)
	}
	if data, err = reconcileImportedModel(existing, data, path, fields.expresses("model") || importedHasKey(data, "model")); err != nil {
		return fmt.Errorf("merge %s: %w", path, err)
	}
	if data, err = dropEmptyScopedBlock(data, path, importScopeTarget); err != nil {
		return err
	}
	flushKeptNote(path, data)
	data = keepEqualMarkdown(existing, data)
	if addsOnlyOwnModelEntries(existing, data, importModelTargets(importScopeTarget)) {
		return withImportMerge(func() error { return importWriteFile(path, data, mode) })
	}
	return importWriteFile(path, data, mode)
}

// importWriteMarkdown writes an imported markdown spec whose frontmatter
// the translator already settled, so the file is replaced whole. A
// destination that parses the same as data is left as it is.
func importWriteMarkdown(path string, data []byte, mode fs.FileMode) error {
	return importWriteMarkdownCarrying(path, data, mode, nil)
}

// importWriteMarkdownCarrying is importWriteMarkdown for a native format
// that holds only some keys; unwritten reports the others. Nil means it
// holds every key.
func importWriteMarkdownCarrying(path string, data []byte, mode fs.FileMode, unwritten func(string) bool) error {
	data, err := importScopedSpecKeys(data, path, unwritten, true)
	if err != nil {
		return err
	}
	if data, err = dropEmptyScopedBlock(data, path, importScopeTarget); err != nil {
		return err
	}
	flushKeptNote(path, data)
	if existing, err := os.ReadFile(path); err == nil {
		data = keepEqualMarkdown(existing, data)
	}
	return importWriteFile(path, data, mode)
}

// keepEqualMarkdown returns existing when it parses the same as data, so
// the user's formatting survives and the run sees no change (#2182).
func keepEqualMarkdown(existing, data []byte) []byte {
	if sameSpecMarkdown(existing, data) {
		return existing
	}
	return data
}

// sameSpecMarkdown reports whether a and b are markdown with frontmatter
// whose parsed frontmatter and body are equal, so they differ only in how
// the YAML is written: scalar style, quoting, key order, comments.
func sameSpecMarkdown(a, b []byte) bool {
	aYAML, aBody, aHad := splitFrontmatter(a)
	bYAML, bBody, bHad := splitFrontmatter(b)
	if !aHad || !bHad || aBody != bBody {
		return false
	}
	var aMeta, bMeta map[string]any
	if yaml.Unmarshal(aYAML, &aMeta) != nil || yaml.Unmarshal(bYAML, &bMeta) != nil {
		return false
	}
	return reflect.DeepEqual(aMeta, bMeta)
}

// mergeSpecFrontmatter returns imported with the top-level frontmatter
// keys only existing declares, and that fields says the native format
// cannot express, carried over in existing's key order, with
// imported-only keys appended. The body is always the imported one. Frontmatter that is missing or is not a mapping on
// either side returns imported untouched, so a malformed file degrades
// to a plain overwrite rather than failing the import.
func mergeSpecFrontmatter(existing, imported []byte, fields specFields) ([]byte, error) {
	existingYAML, _, hadExisting := splitFrontmatter(existing)
	if !hadExisting || len(bytes.TrimSpace(existingYAML)) == 0 {
		return imported, nil
	}
	existingMap, err := frontmatterMapping(existingYAML)
	if existingMap == nil || err != nil {
		return imported, nil
	}

	importedYAML, importedBody, hadImported := splitFrontmatter(imported)
	importedMap, err := frontmatterMapping(importedYAML)
	if err != nil {
		return imported, nil
	}
	if importedMap == nil {
		importedMap = &yaml.Node{Kind: yaml.MappingNode}
	}
	preserved := false
	if fields.preserve != nil {
		before := len(importedMap.Content)
		fields.preserve(existingMap, importedMap)
		preserved = len(importedMap.Content) != before
	}

	importedKeys := make(map[string]*yaml.Node, len(importedMap.Content)/2)
	for i := 0; i+1 < len(importedMap.Content); i += 2 {
		importedKeys[importedMap.Content[i].Value] = importedMap.Content[i+1]
	}

	merged := &yaml.Node{Kind: yaml.MappingNode}
	carried := preserved
	for i := 0; i+1 < len(existingMap.Content); i += 2 {
		key := existingMap.Content[i]
		if value, ok := importedKeys[key.Value]; ok {
			merged.Content = append(merged.Content, key, value)
			continue
		}
		if (hadImported || fields.all) && fields.expresses(key.Value) {
			continue
		}
		merged.Content = append(merged.Content, key, existingMap.Content[i+1])
		carried = true
	}
	if !carried {
		return imported, nil
	}
	for i := 0; i+1 < len(importedMap.Content); i += 2 {
		if !mappingHasKey(existingMap, importedMap.Content[i].Value) {
			merged.Content = append(merged.Content, importedMap.Content[i], importedMap.Content[i+1])
		}
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(merged); err != nil {
		return nil, fmt.Errorf("encode frontmatter: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}

	var out bytes.Buffer
	out.WriteString("---\n")
	out.Write(buf.Bytes())
	out.WriteString("---\n")
	if importedBody != "" {
		out.WriteString("\n" + importedBody)
	}
	return out.Bytes(), nil
}

// frontmatterMapping decodes a frontmatter block into its mapping node,
// returning nil when the block is empty and an error when it is not a
// YAML mapping.
func frontmatterMapping(yamlBytes []byte) (*yaml.Node, error) {
	if len(bytes.TrimSpace(yamlBytes)) == 0 {
		return nil, nil
	}
	var node yaml.Node
	if err := yaml.Unmarshal(yamlBytes, &node); err != nil {
		return nil, fmt.Errorf("parse frontmatter: %w", err)
	}
	if node.Kind != yaml.DocumentNode || len(node.Content) == 0 || node.Content[0].Kind != yaml.MappingNode {
		return nil, errors.New("frontmatter root must be a mapping")
	}
	return node.Content[0], nil
}

// mappingHasKey reports whether mapping declares key at the top level.
func mappingHasKey(mapping *yaml.Node, key string) bool {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return true
		}
	}
	return false
}

func detachImportOverride(node *yaml.Node) *yaml.Node {
	if node.Kind == yaml.AliasNode {
		return detachImportOverride(node.Alias)
	}
	detached := *node
	detached.Anchor = ""
	detached.Content = make([]*yaml.Node, len(node.Content))
	for i, child := range node.Content {
		detached.Content[i] = detachImportOverride(child)
	}
	return &detached
}

// addsOnlyOwnModelEntries reports whether data differs from existing only
// in the per-target model entries of targets, which layers the import onto
// the spec instead of replacing what it holds for other tools.
func addsOnlyOwnModelEntries(existing, data []byte, targets []string) bool {
	_, oldEnd, oldMeta := claudeFrontmatter(string(existing))
	newRest, newEnd, newMeta := claudeFrontmatter(string(data))
	oldRest, _, _ := claudeFrontmatter(string(existing))
	if oldMeta == nil || newMeta == nil || oldRest[oldEnd:] != newRest[newEnd:] {
		return false
	}
	oldModel, newModel := oldMeta["model"], newMeta["model"]
	oldMap, oldIsMap := oldModel.(map[string]any)
	newMap, newIsMap := newModel.(map[string]any)
	if !newIsMap || oldModel != nil && !oldIsMap {
		return false
	}
	for k, v := range oldMap {
		if !slices.Contains(targets, k) && !reflect.DeepEqual(newMap[k], v) {
			return false
		}
	}
	for k, v := range newMap {
		if !slices.Contains(targets, k) && !reflect.DeepEqual(oldMap[k], v) {
			return false
		}
	}
	oldOther, newOther := maps.Clone(oldMeta), maps.Clone(newMeta)
	delete(oldOther, "model")
	delete(newOther, "model")
	return reflect.DeepEqual(oldOther, newOther)
}
