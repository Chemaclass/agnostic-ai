package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"
)

type staleToggleError struct{ key string }

func (e staleToggleError) Error() string {
	return fmt.Sprintf("%s changed after this page loaded. Reload saved values and try again", e.key)
}

func writeExpectedError(w http.ResponseWriter, err error) {
	var stale staleToggleError
	status := http.StatusBadRequest
	if errors.As(err, &stale) {
		status = http.StatusConflict
	}
	writeEditJSON(w, status, editError{Error: err.Error()})
}

func expectedBool(raw json.RawMessage) (bool, error) {
	var value bool
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) || json.Unmarshal(raw, &value) != nil {
		return false, errors.New("expected must be the boolean the page showed")
	}
	return value, nil
}

func (h *editHandler) checkBooleanExpected(path, key string, raw json.RawMessage) error {
	expected, err := expectedBool(raw)
	if err != nil {
		return err
	}
	scope, err := loadCheckScope(h.global)
	if err != nil {
		return err
	}
	for _, e := range scope.bundle.Settings {
		if !h.matchesShownSpec(e, path) || e.Layer == "builtin" {
			continue
		}
		var current any = e.Meta
		for _, part := range strings.Split(key, ".") {
			m, ok := current.(map[string]any)
			if !ok {
				return staleToggleError{key}
			}
			current = m[part]
		}
		v, ok := current.(bool)
		if !ok || v != expected {
			return staleToggleError{key}
		}
		return nil
	}
	return staleToggleError{key}
}

func (h *editHandler) checkBuiltinExpected(name string, reset bool, raw json.RawMessage) error {
	list, err := h.teamBuiltins()
	if err != nil {
		return err
	}
	_, local := h.builtinsConfigFiles()
	data, err := os.ReadFile(local)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("%s: %w", local, err)
	}
	if err == nil {
		personal, err := personalBuiltins(data)
		if err != nil {
			return err
		}
		if personal != nil {
			list = *personal
		}
	}
	if reset {
		var expected []string
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) || json.Unmarshal(raw, &expected) != nil {
			return errors.New("expected must be the built-in list the page showed")
		}
		if !sameBuiltins(list, expected) {
			return staleToggleError{"built-ins"}
		}
		return nil
	}
	expected, err := expectedBool(raw)
	if err != nil {
		return err
	}
	if slices.Contains(list, name) != expected {
		return staleToggleError{name}
	}
	return nil
}

func watchEditSources(paths []string, destination, key string) (func() error, error) {
	type watched struct {
		path     string
		resolved string
		snapshot editFileSnapshot
	}
	var sources []watched
	for _, path := range paths {
		if path == "" || path == destination {
			continue
		}
		duplicate := false
		for _, source := range sources {
			if source.path == path {
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}
		resolved := resolvedAbs(path)
		snap, err := snapshotEditFile(resolved)
		if err != nil {
			return nil, err
		}
		sources = append(sources, watched{path, resolved, snap})
	}
	return func() error {
		for _, source := range sources {
			if resolvedAbs(source.path) != source.resolved {
				return staleToggleError{key}
			}
			if err := source.snapshot.check(source.resolved); err != nil {
				return staleToggleError{key}
			}
		}
		return nil
	}, nil
}

func (h *editHandler) booleanSourcePaths(shown string) ([]string, error) {
	scope, err := loadCheckScope(h.global)
	if err != nil {
		return nil, err
	}
	var paths []string
	team, local := h.builtinsConfigFiles()
	paths = append(paths, team, local)
	for _, entry := range scope.bundle.Settings {
		if !h.matchesShownSpec(entry, shown) {
			continue
		}
		for e := &entry; e != nil; e = e.Base {
			paths = append(paths, e.Path)
		}
		break
	}
	return paths, nil
}
