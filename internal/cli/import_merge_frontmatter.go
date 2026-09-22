package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"

	"gopkg.in/yaml.v3"
)

// importWriteSpecMarkdown writes an imported markdown spec to path,
// carrying over the frontmatter keys only the spec already on disk
// declares. Most targets emit a subset of a spec's frontmatter, so a
// byte-for-byte write back would delete every key that target cannot
// express (`argument-hint` and `allowed-tools` on a cursor skill, for
// one). A missing destination is a plain write.
//
// fields names what the target's native file can hold: a key it can
// hold but the import does not carry was deleted on purpose and is not
// restored. Rules pass through importWriteFile instead, because their
// translators drop a catch-all `globs` and an empty `description` on
// purpose (#429) and the spec has to follow.
func importWriteSpecMarkdown(path string, data []byte, mode fs.FileMode, fields specFields) error {
	if fields.all {
		return importWriteFile(path, data, mode)
	}
	existing, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return importWriteFile(path, data, mode)
	case err != nil:
		return fmt.Errorf("read %s: %w", path, err)
	}
	merged, err := mergeSpecFrontmatter(existing, data, fields)
	if err != nil {
		return fmt.Errorf("merge %s: %w", path, err)
	}
	return importWriteFile(path, merged, mode)
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

	importedKeys := make(map[string]*yaml.Node, len(importedMap.Content)/2)
	for i := 0; i+1 < len(importedMap.Content); i += 2 {
		importedKeys[importedMap.Content[i].Value] = importedMap.Content[i+1]
	}

	merged := &yaml.Node{Kind: yaml.MappingNode}
	carried := false
	for i := 0; i+1 < len(existingMap.Content); i += 2 {
		key := existingMap.Content[i]
		if value, ok := importedKeys[key.Value]; ok {
			merged.Content = append(merged.Content, key, value)
			continue
		}
		if hadImported && fields.expresses(key.Value) {
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
