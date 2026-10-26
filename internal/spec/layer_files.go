package spec

import (
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"
)

var errPlannedRemoval = errors.New("removed by the planned change")

func (l Layer) plannedParse(parse func(string) (Entry, error)) func(string) (Entry, error) {
	return func(path string) (Entry, error) {
		data, planned := l.Files[filepath.Clean(path)]
		switch {
		case !planned:
			return parse(path)
		case data == nil:
			return Entry{}, errPlannedRemoval
		}
		return parseYAMLData(path, data)
	}
}

// addPlannedFiles adds planned files the walk did not find, at the slot a
// walk of the written folder would give them.
func (l Layer) addPlannedFiles(entries []Entry, dir string, kind Kind) ([]Entry, error) {
	clean := filepath.Clean(dir)
	for _, path := range slices.Sorted(maps.Keys(l.Files)) {
		data := l.Files[path]
		rel, err := filepath.Rel(clean, path)
		if data == nil || filepath.Ext(path) != ".yaml" || err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		if slices.ContainsFunc(entries, func(e Entry) bool { return filepath.Clean(e.Path) == path }) {
			continue
		}
		entry, err := parseYAMLData(path, data)
		if err != nil {
			return nil, err
		}
		entry.Kind = kind
		if entry.Name == "" {
			entry.Name = strings.TrimSuffix(filepath.Base(path), ".yaml")
		}
		if err := ValidateName(kind, entry.Name); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		at := slices.IndexFunc(entries, func(e Entry) bool { return filepath.Clean(e.Path) > path })
		if at < 0 {
			at = len(entries)
		}
		entries = slices.Insert(entries, at, entry)
	}
	return entries, nil
}
