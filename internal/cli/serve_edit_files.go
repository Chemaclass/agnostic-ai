package cli

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"

	"gopkg.in/yaml.v3"
)

type editFileSnapshot struct {
	data []byte
	info os.FileInfo
}

type editBaseline struct {
	original, last []byte
	existed        bool
}

func snapshotEditFile(path string) (editFileSnapshot, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return editFileSnapshot{}, nil
	}
	if err != nil {
		return editFileSnapshot{}, fmt.Errorf("%s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return editFileSnapshot{}, fmt.Errorf("%s: is not a regular file", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return editFileSnapshot{}, fmt.Errorf("%s: %w", path, err)
	}
	after, err := os.Lstat(path)
	if err != nil {
		return editFileSnapshot{}, fmt.Errorf("%s: reread metadata: %w", path, err)
	}
	if !os.SameFile(info, after) || info.Size() != after.Size() || !info.ModTime().Equal(after.ModTime()) {
		return editFileSnapshot{}, fmt.Errorf("%s: changed while reading", path)
	}
	return editFileSnapshot{data: data, info: after}, nil
}

func (s editFileSnapshot) check(path string) error {
	now, err := snapshotEditFile(path)
	if err != nil {
		return err
	}
	same := s.info == nil && now.info == nil
	if s.info != nil && now.info != nil {
		same = os.SameFile(s.info, now.info) && s.info.Mode() == now.info.Mode() && s.info.ModTime().Equal(now.info.ModTime()) && bytes.Equal(s.data, now.data)
	}
	if !same {
		return fmt.Errorf("%s: changed since it was read; keeping the current file", path)
	}
	return nil
}

func (s editFileSnapshot) replace(path string, data []byte, modes ...os.FileMode) (editFileSnapshot, error) {
	if err := s.check(path); err != nil {
		return editFileSnapshot{}, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return editFileSnapshot{}, fmt.Errorf("%s: %w", path, err)
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".agnostic-ai-edit-*")
	if err != nil {
		return editFileSnapshot{}, fmt.Errorf("%s: %w", path, err)
	}
	defer func() { _ = os.Remove(f.Name()) }()
	mode := os.FileMode(0o600)
	if s.info != nil {
		mode = s.info.Mode().Perm()
	}
	if len(modes) > 0 {
		mode = modes[0]
	}
	if err := f.Chmod(mode); err != nil {
		_ = f.Close()
		return editFileSnapshot{}, fmt.Errorf("%s: %w", path, err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return editFileSnapshot{}, fmt.Errorf("%s: %w", path, err)
	}
	identity, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return editFileSnapshot{}, fmt.Errorf("%s: inspect pending edit identity: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return editFileSnapshot{}, fmt.Errorf("%s: %w", path, err)
	}
	info, err := os.Lstat(f.Name())
	if err != nil {
		return editFileSnapshot{}, fmt.Errorf("%s: inspect pending edit: %w", path, err)
	}
	// Windows Lstat resolves identity lazily, so compare while the temp path exists.
	if !os.SameFile(identity, info) {
		return editFileSnapshot{}, fmt.Errorf("%s: pending edit changed before replacement", path)
	}
	if err := s.check(path); err != nil {
		return editFileSnapshot{}, err
	}
	if err := os.Rename(f.Name(), path); err != nil {
		return editFileSnapshot{}, fmt.Errorf("%s: %w", path, err)
	}
	return editFileSnapshot{data: slices.Clone(data), info: info}, nil
}

func sameEditValues(a, b []byte) bool {
	var left, right map[string]any
	if yaml.Unmarshal(a, &left) != nil || yaml.Unmarshal(b, &right) != nil {
		return false
	}
	if len(left) == 0 {
		left = map[string]any{}
	}
	if len(right) == 0 {
		right = map[string]any{}
	}
	if _, present := left["builtins"]; !present {
		if list, ok := right["builtins"].([]any); ok && len(list) == 0 {
			delete(right, "builtins")
		}
	}
	return reflect.DeepEqual(left, right)
}

func (h *editHandler) applyValidated(file string, edit func([]byte) ([]byte, error)) ([]string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	before, beforeErr := h.validationProblems()
	if beforeErr != nil {
		return nil, fmt.Errorf("%s: cannot validate configuration before the edit: %w", file, beforeErr)
	}
	original, err := snapshotEditFile(file)
	if err != nil {
		return nil, err
	}
	edited, err := edit(original.data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	baseline, tracked := h.baselines[file]
	if !tracked || !bytes.Equal(original.data, baseline.last) {
		baseline = editBaseline{original: slices.Clone(original.data), existed: original.info != nil}
	}
	restoreBaseline := sameEditValues(baseline.original, edited)
	if restoreBaseline {
		edited = slices.Clone(baseline.original)
	}
	if bytes.Equal(original.data, edited) {
		return nil, original.check(file)
	}
	var written editFileSnapshot
	if (restoreBaseline && !baseline.existed) || len(edited) == 0 {
		if err = original.check(file); err == nil {
			err = os.Remove(file)
		}
	} else {
		written, err = original.replace(file, edited)
	}
	if err != nil {
		return nil, err
	}
	after, afterErr := h.validationProblems()
	var added []string
	if afterErr != nil && beforeErr == nil {
		added = append(added, afterErr.Error())
	}
	for _, problem := range after {
		if !slices.Contains(before, problem) {
			added = append(added, problem)
		}
	}
	if len(added) == 0 {
		if err := written.check(file); err != nil {
			return nil, err
		}
		if h.baselines == nil {
			h.baselines = map[string]editBaseline{}
		}
		baseline.last = slices.Clone(edited)
		h.baselines[file] = baseline
		return nil, nil
	}
	if err := written.check(file); err != nil {
		return added, fmt.Errorf("the edit did not validate; could not restore the file: %w", err)
	}
	if original.info == nil {
		err = os.Remove(file)
	} else {
		_, err = written.replace(file, original.data, original.info.Mode().Perm())
	}
	if err != nil {
		return added, fmt.Errorf("the edit did not validate; could not restore the file: %w", err)
	}
	return added, fmt.Errorf("%s: the edit did not validate, so the file was restored", file)
}
