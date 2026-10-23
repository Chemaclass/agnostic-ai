package emit

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
)

type TOMLValue struct {
	Path      []string
	Value     any
	MergeList bool
}

func (s *Session) MergeTOMLFile(path string, values []TOMLValue, dryRun bool) error {
	values = slices.DeleteFunc(slices.Clone(values), func(value TOMLValue) bool { return value.MergeList && len(tomlStrings(value.Value)) == 0 })
	if len(values) == 0 {
		return nil
	}
	s.WatchPlannedFile(path)
	data, err := os.ReadFile(path)
	if err != nil && !IsAbsent(err) {
		return fmt.Errorf("%s: %w", path, err)
	}
	doc, err := parseTOMLMap(path, data)
	if err != nil {
		return err
	}
	var released [][]string
	prior := priorMergedKeys(path)
	for _, key := range prior {
		releaseTOMLKey(doc, key, false)
		released = append(released, key.Path)
	}
	var keys []MergedKey
	for _, value := range values {
		if len(value.Path) == 0 {
			return fmt.Errorf("%s: empty TOML setting path", path)
		}
		key := MergedKey{Path: slices.Clone(value.Path)}
		if value.MergeList {
			current, _ := tomlMapValue(doc, value.Path)
			list := tomlStrings(current)
			own := tomlStrings(value.Value)
			for _, item := range own {
				if !slices.Contains(list, item) {
					list = append(list, item)
				}
				key.Items = append(key.Items, canonicalValueSum(item, item))
			}
			setTOMLMapValue(doc, value.Path, list)
		} else {
			setTOMLMapValue(doc, value.Path, value.Value)
			key.Sum = canonicalValueSum(value.Value, "")
		}
		if at := slices.IndexFunc(keys, func(prior MergedKey) bool { return slices.Equal(prior.Path, key.Path) }); at >= 0 && key.Items != nil && keys[at].Items != nil {
			for _, sum := range key.Items {
				if !slices.Contains(keys[at].Items, sum) {
					keys[at].Items = append(keys[at].Items, sum)
				}
			}
		} else {
			keys = append(keys, key)
		}
	}
	content, err := renderTOMLMap(path, doc, true)
	if err != nil {
		return err
	}
	return s.writeMerged(path, content, keys, released, dryRun)
}

func parseTOMLMap(path string, data []byte) (map[string]any, error) {
	doc := map[string]any{}
	if _, err := toml.Decode(string(data), &doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return doc, nil
}
func renderTOMLMap(path string, doc map[string]any, provenance bool) (string, error) {
	var out bytes.Buffer
	if err := toml.NewEncoder(&out).Encode(doc); err != nil {
		return "", fmt.Errorf("render %s: %w", path, err)
	}
	if provenance {
		return WithHeader(out.String(), FormatTOML), nil
	}
	return out.String(), nil
}
func tomlStrings(value any) []string {
	if list, ok := value.([]string); ok {
		return slices.Clone(list)
	}
	return StringSlice(value)
}
func tomlMapValue(doc map[string]any, path []string) (any, bool) {
	var current any = doc
	for _, key := range path {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		value, ok := object[key]
		if !ok {
			return nil, false
		}
		current = value
	}
	return current, true
}
func setTOMLMapValue(doc map[string]any, path []string, value any) {
	if len(path) == 1 {
		doc[path[0]] = value
		return
	}
	child, _ := doc[path[0]].(map[string]any)
	if child == nil {
		child = map[string]any{}
		doc[path[0]] = child
	}
	setTOMLMapValue(child, path[1:], value)
}
func deleteTOMLMapValue(doc map[string]any, path []string) {
	if len(path) == 1 {
		delete(doc, path[0])
		return
	}
	child, _ := doc[path[0]].(map[string]any)
	if child == nil {
		return
	}
	deleteTOMLMapValue(child, path[1:])
	if len(child) == 0 {
		delete(doc, path[0])
	}
}
func releaseTOMLKey(doc map[string]any, key MergedKey, force bool) bool {
	value, found := tomlMapValue(doc, key.Path)
	if !found {
		return false
	}
	if key.Items != nil {
		list := tomlStrings(value)
		pending := slices.Clone(key.Items)
		changed := false
		for i := len(list) - 1; i >= 0; i-- {
			sum := canonicalValueSum(list[i], list[i])
			if at := slices.Index(pending, sum); at >= 0 {
				pending = slices.Delete(pending, at, at+1)
				list = slices.Delete(list, i, i+1)
				changed = true
			}
		}
		if changed {
			if len(list) == 0 {
				deleteTOMLMapValue(doc, key.Path)
			} else {
				setTOMLMapValue(doc, key.Path, list)
			}
		}
		return changed
	}
	if !force && key.Sum != "" && canonicalValueSum(value, "") != key.Sum {
		return false
	}
	deleteTOMLMapValue(doc, key.Path)
	return true
}
func (s *Session) ReleaseMergedTOML(path string, keys []MergedKey, created, force, dryRun bool) (MergedRelease, []MergedKey, error) {
	if s.skipUnmanaged(path) {
		return MergedKept, nil, nil
	}
	data, err := os.ReadFile(path)
	if IsAbsent(err) {
		return MergedUnchanged, nil, nil
	}
	if err != nil {
		return MergedKept, nil, fmt.Errorf("%s: %w", path, err)
	}
	doc, err := parseTOMLMap(path, data)
	if err != nil {
		return MergedKept, nil, nil
	}
	var edited []MergedKey
	changed := false
	for _, key := range keys {
		value, found := tomlMapValue(doc, key.Path)
		if !found {
			continue
		}
		if key.Items == nil && !force && key.Sum != "" && canonicalValueSum(value, "") != key.Sum {
			edited = append(edited, key)
			continue
		}
		changed = releaseTOMLKey(doc, key, force) || changed
	}
	if len(doc) == 0 && len(edited) == 0 && created {
		if _, err := s.remove(path, "", data, force, dryRun); err != nil {
			return MergedKept, nil, err
		}
		return MergedRemoved, nil, nil
	}
	if changed || header.Has(string(data)) {
		content, err := renderTOMLMap(path, doc, false)
		if err != nil {
			return MergedKept, nil, err
		}
		if err := s.WriteFile(path, content, dryRun); err != nil {
			return MergedKept, nil, err
		}
		changed = true
	}
	if len(edited) > 0 {
		return MergedEdited, edited, nil
	}
	if changed {
		return MergedStripped, nil, nil
	}
	return MergedUnchanged, nil, nil
}
func isTOMLPath(path string) bool { return strings.EqualFold(filepath.Ext(path), ".toml") }
