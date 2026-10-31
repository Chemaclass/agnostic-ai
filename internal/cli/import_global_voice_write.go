package cli

import (
	"fmt"
	"os"
	"path/filepath"

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

func stageImportedGlobalVoice(source, stage string, doc map[string]any, before *editFileSnapshot) (*importedVoiceUpdate, error) {
	if _, present := doc[voiceField]; !present {
		return nil, nil
	}
	path := filepath.Join(source, globalImportSettingsSpec)
	if before != nil {
		if err := before.check(path); err != nil {
			return nil, err
		}
		if before.info == nil {
			return nil, nil
		}
		merged, added, err := appendImportedGlobalVoice(path, before.data, doc)
		if err != nil {
			return nil, err
		}
		if added {
			return &importedVoiceUpdate{before: *before, data: merged}, nil
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
	return nil, stageGlobalSettingsDoc(stage, "settings/imported-voice.yaml", voice)
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
