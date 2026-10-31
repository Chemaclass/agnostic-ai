package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/chemaclass/agnostic-ai/internal/spec"

	"gopkg.in/yaml.v3"
)

type importedVoiceUpdate struct {
	before editFileSnapshot
	data   []byte
}

func snapshotImportedGlobalSettings(source string) (*editFileSnapshot, error) {
	path := filepath.Join(source, globalImportSettingsSpec)
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return &editFileSnapshot{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return nil, nil
	}
	before, err := snapshotEditFile(path)
	if err != nil {
		return nil, err
	}
	return &before, nil
}

func stageImportedGlobalVoice(source, stage string, doc map[string]any, before *editFileSnapshot, settings []spec.Entry) (*importedVoiceUpdate, error) {
	if _, present := doc[voiceField]; !present {
		return nil, nil
	}
	path := filepath.Join(source, globalImportSettingsSpec)
	if before != nil {
		if err := before.check(path); err != nil {
			return nil, err
		}
		name := "imported"
		if before.info != nil {
			entry, err := spec.ParseYAMLBytes(spec.KindSettings, before.data)
			if err != nil {
				return nil, fmt.Errorf("parse %s: %w", path, err)
			}
			if entry.Name != "" {
				name = entry.Name
			}
		}
		compatible := importedVoiceLayerCompatible(settings, name, doc)
		if before.info != nil && !importedVoiceDestinationLoaded(settings, name, path) {
			compatible = false
		}
		if before.info == nil && compatible {
			return nil, nil
		}
		if before.info != nil && compatible {
			merged, added, err := appendImportedGlobalVoice(path, before.data, doc)
			if err != nil {
				return nil, err
			}
			if added {
				return &importedVoiceUpdate{before: *before, data: merged}, nil
			}
		}
	}
	voice := map[string]any{voiceField: doc[voiceField]}
	delete(doc, voiceField)
	for _, key := range []string{"target", "targets", "target-exclude", "targets-exclude"} {
		if value, present := doc[key]; present {
			voice[key] = value
			delete(doc, key)
		}
	}
	name, err := unusedImportedVoicePath(source, settings)
	if err != nil {
		return nil, err
	}
	return nil, stageGlobalSettingsDoc(stage, name, voice)
}

func importedVoiceDestinationLoaded(settings []spec.Entry, name, path string) bool {
	for _, entry := range settings {
		if entry.Name != name {
			continue
		}
		for current := &entry; current != nil; current = current.Base {
			if filepath.Clean(current.Path) == filepath.Clean(path) {
				return true
			}
		}
	}
	return false
}

func importedVoiceLayerCompatible(settings []spec.Entry, name string, doc map[string]any) bool {
	for _, entry := range settings {
		if entry.Name != name || entry.Layer != "global-local" {
			continue
		}
		local := entry.LocalMeta
		if entry.Base == nil {
			local = entry.Meta
		}
		if _, present := local[voiceField]; present {
			return false
		}
		merged := make(map[string]any, len(doc))
		for key, value := range doc {
			merged[key] = value
		}
		for _, key := range []string{"target", "targets", "target-exclude", "targets-exclude"} {
			if value, present := local[key]; present {
				if value == nil {
					delete(merged, key)
				} else {
					merged[key] = value
				}
			}
		}
		want, effective := spec.Entry{Meta: doc}, spec.Entry{Meta: merged}
		for _, target := range globalTargetNames() {
			if want.EmitsTo(target) != effective.EmitsTo(target) {
				return false
			}
		}
	}
	return true
}

func unusedImportedVoicePath(source string, settings []spec.Entry) (string, error) {
	for i := 1; ; i++ {
		name := "imported-voice"
		if i > 1 {
			name = fmt.Sprintf("%s-%d", name, i)
		}
		if slices.ContainsFunc(settings, func(entry spec.Entry) bool { return entry.Name == name }) {
			continue
		}
		occupied := false
		for _, root := range []string{source, filepath.Join(source, "local")} {
			for _, ext := range []string{".yaml", ".yml"} {
				path := filepath.Join(root, "settings", name+ext)
				if _, err := os.Lstat(path); err == nil {
					occupied = true
				} else if !os.IsNotExist(err) {
					return "", fmt.Errorf("%s: %w", path, err)
				}
			}
		}
		if !occupied {
			return filepath.ToSlash(filepath.Join("settings", name+".yaml")), nil
		}
	}
}

func appendImportedGlobalVoice(path string, existing []byte, staged map[string]any) ([]byte, bool, error) {
	var have map[string]any
	if err := yaml.Unmarshal(existing, &have); err != nil {
		return nil, false, fmt.Errorf("parse %s: %w", path, err)
	}
	if _, present := have[voiceField]; present {
		return nil, false, nil
	}
	for _, key := range []string{"target", "targets", "target-exclude", "targets-exclude", "model", "effort"} {
		if value, present := staged[key]; present || key != "model" && key != "effort" {
			if !sameSetting(have[key], value) {
				return nil, false, nil
			}
		}
	}
	_, root, err := yamlDocument(existing)
	if err != nil {
		return nil, false, nil
	}
	merged, err := editYAMLPair(existing, root, voiceField, staged[voiceField])
	if err != nil {
		return nil, false, nil
	}
	var after map[string]any
	if err := yaml.Unmarshal(merged, &after); err != nil || !sameSetting(after[voiceField], staged[voiceField]) {
		return nil, false, nil
	}
	delete(after, voiceField)
	if len(have) > 0 && !sameSetting(have, after) {
		return nil, false, nil
	}
	return merged, true, nil
}
