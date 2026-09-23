package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// settingsEffortPlan says where an imported native repository effort goes.
type settingsEffortPlan int

const (
	// effortStays leaves the native value where it is: nothing in the
	// settings specs would replace it on the next sync.
	effortStays settingsEffortPlan = iota
	// effortPromote moves it into the portable settings `effort`.
	effortPromote
	// effortOverride keeps it under x-<target>, because another settings
	// spec already sets an effort that would replace it.
	effortOverride
)

// planSettingsEffort decides where target's native repository effort
// value goes. own is the settings file this importer writes, which a
// re-import replaces, so it never counts as another spec.
func planSettingsEffort(target string, value any, settingsDir, own string) (settingsEffortPlan, string, error) {
	level, ok := value.(string)
	if !ok || level == "" {
		return effortStays, "", nil
	}
	others, err := otherSettingsSpecs(settingsDir, filepath.Join(settingsDir, own))
	if err != nil {
		return effortStays, "", err
	}
	for _, entry := range others {
		if _, set := entry.Meta["effort"]; set {
			resolved := adapters.SettingsEffort(others, target)
			if resolved == nil || resolved == level {
				return effortStays, level, nil
			}
			return effortOverride, level, nil
		}
	}
	if adapters.AcceptsSettingsEffort(target, level) {
		return effortPromote, level, nil
	}
	return effortStays, level, nil
}

// otherSettingsSpecs reads every settings spec under dir except own, in
// the order sync loads them.
func otherSettingsSpecs(dir, own string) ([]spec.Entry, error) {
	var out []spec.Entry
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if errors.Is(walkErr, fs.ErrNotExist) {
				return filepath.SkipAll
			}
			return walkErr
		}
		if d.IsDir() || filepath.Ext(path) != ".yaml" || path == own {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		var meta map[string]any
		if err := yaml.Unmarshal(data, &meta); err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		out = append(out, spec.Entry{Kind: spec.KindSettings, Path: path, Meta: meta})
		return nil
	})
	return out, err
}

// settingsEffortSpec returns the settings spec body that carries level
// by plan: the portable `effort`, or the native key under x-<target>.
func settingsEffortSpec(plan settingsEffortPlan, target, nativeKey, level string) map[string]any {
	switch plan {
	case effortPromote:
		return map[string]any{"effort": level}
	case effortOverride:
		return map[string]any{"x-" + target: map[string]any{nativeKey: level}}
	}
	return nil
}

// writeSettingsSpec writes doc as the settings spec file in settingsDir.
func writeSettingsSpec(settingsDir, file string, doc map[string]any) error {
	raw, err := yaml.Marshal(doc)
	if err != nil {
		return fmt.Errorf("marshal settings %s: %w", file, err)
	}
	if err := importMkdirAll(settingsDir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", settingsDir, err)
	}
	dst := filepath.Join(settingsDir, file)
	if err := importWriteFile(dst, raw, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", dst, err)
	}
	return nil
}
