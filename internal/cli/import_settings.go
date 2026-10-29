package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// portableSettingsShape says which portable fields a target's native
// settings file carries.
type portableSettingsShape struct {
	// nestedModel reads the model from `model.name` instead of `model`.
	nestedModel bool
	permissions bool
	// target names the settings spec, `<target>.yaml`, so each importer
	// keeps its own file under `import all`.
	target string
	// effortKey is the native repository effort key target promotes
	// into `effort`, when it has one.
	effortKey string
	// hatchKeys are native keys copied verbatim under `x-<target>`,
	// for repository keys sync writes back through that hatch.
	hatchKeys []string
}

// importPortableSettings reads the portable settings fields exposed by a
// target and writes one settings spec. Other native keys stay in place and
// are not promoted into a cross-target source.
func importPortableSettings(root, nativePath, dstDir string, shape portableSettingsShape) (int, error) {
	path := filepath.Join(root, nativePath)
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", path, err)
	}
	data, _ = adapters.StripJSONC(data)
	var native map[string]any
	if err := json.Unmarshal(data, &native); err != nil {
		return 0, fmt.Errorf("parse %s: %w", path, err)
	}

	doc := map[string]any{}
	if shape.nestedModel {
		if model, ok := native["model"].(map[string]any); ok {
			if name, _ := model["name"].(string); name != "" {
				doc["model"] = name
			}
		}
	} else if model, _ := native["model"].(string); model != "" {
		doc["model"] = model
	}
	if shape.permissions {
		if value, ok := native["permissions"].(map[string]any); ok {
			portable := map[string]any{}
			for _, key := range []string{"allow", "deny", "ask"} {
				if entries, ok := value[key].([]any); ok && len(entries) > 0 {
					neutral := make([]any, len(entries))
					for i, raw := range entries {
						neutral[i] = raw
						if rule, ok := raw.(string); ok {
							neutral[i] = importNeutralPermissionRule(rule)
						}
					}
					portable[key] = neutral
				}
			}
			if len(portable) > 0 {
				doc["permissions"] = portable
			}
		}
	}
	if shape.effortKey != "" {
		plan, level, err := planSettingsEffort(shape.target, native[shape.effortKey], dstDir, shape.target+".yaml")
		if err != nil {
			return 0, err
		}
		maps.Copy(doc, settingsEffortSpec(plan, shape.target, shape.effortKey, level))
	}
	if len(shape.hatchKeys) > 0 {
		if err := importSettingsHatch(doc, native, dstDir, shape); err != nil {
			return 0, err
		}
	}
	if len(doc) == 0 {
		return 0, nil
	}
	raw, err := yaml.Marshal(doc)
	if err != nil {
		return 0, fmt.Errorf("marshal settings from %s: %w", path, err)
	}
	if err := importMkdirAll(dstDir, 0o755); err != nil {
		return 0, fmt.Errorf("create %s: %w", dstDir, err)
	}
	dst := filepath.Join(dstDir, shape.target+".yaml")
	if err := importWriteFile(dst, raw, 0o644); err != nil {
		return 0, fmt.Errorf("write %s: %w", dst, err)
	}
	return 1, nil
}

// importSettingsHatch copies shape.hatchKeys from native into the
// `x-<target>` block of the target's own settings spec. The block's other
// keys are hand-written and absent from what import reads, so they stay.
// A key another settings spec sets is left alone: sync lets the last spec
// win the whole key, so copying the merged native value here would
// duplicate that spec's entries.
func importSettingsHatch(doc, native map[string]any, dstDir string, shape portableSettingsShape) error {
	hatchName := "x-" + shape.target
	own := filepath.Join(dstDir, shape.target+".yaml")
	hatch := map[string]any{}
	data, err := os.ReadFile(own)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("read %s: %w", own, err)
	}
	if err == nil {
		var existing map[string]any
		if err := yaml.Unmarshal(data, &existing); err != nil {
			return fmt.Errorf("parse %s: %w", own, err)
		}
		if kept, ok := existing[hatchName].(map[string]any); ok {
			maps.Copy(hatch, kept)
		}
	}
	if shape.effortKey != "" {
		delete(hatch, shape.effortKey)
	}
	if imported, ok := doc[hatchName].(map[string]any); ok {
		maps.Copy(hatch, imported)
	}
	others, err := otherSettingsSpecs(dstDir, own)
	if err != nil {
		return err
	}
	for _, key := range shape.hatchKeys {
		if settingsSpecsSetHatchKey(others, hatchName, key) {
			continue
		}
		if value, ok := native[key]; ok {
			hatch[key] = value
		} else {
			delete(hatch, key)
		}
	}
	if len(hatch) == 0 {
		delete(doc, hatchName)
		return nil
	}
	doc[hatchName] = hatch
	return nil
}

func settingsSpecsSetHatchKey(entries []spec.Entry, hatchName, key string) bool {
	for _, entry := range entries {
		if hatch, ok := entry.Meta[hatchName].(map[string]any); ok {
			if _, set := hatch[key]; set {
				return true
			}
		}
	}
	return false
}
