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
)

// portableSettingsShape says which portable fields a target's native
// settings file carries.
type portableSettingsShape struct {
	// nestedModel reads the model from `model.name` instead of `model`.
	nestedModel bool
	permissions bool
	// effortKey is the native repository effort key target promotes
	// into `effort`, when it has one.
	target, effortKey string
}

// importPortableSettingsFile is the settings spec importPortableSettings writes.
const importPortableSettingsFile = "imported.yaml"

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
					portable[key] = entries
				}
			}
			if len(portable) > 0 {
				doc["permissions"] = portable
			}
		}
	}
	if shape.effortKey != "" {
		plan, level, err := planSettingsEffort(shape.target, native[shape.effortKey], dstDir, importPortableSettingsFile)
		if err != nil {
			return 0, err
		}
		maps.Copy(doc, settingsEffortSpec(plan, shape.target, shape.effortKey, level))
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
	dst := filepath.Join(dstDir, importPortableSettingsFile)
	if err := importWriteFile(dst, raw, 0o644); err != nil {
		return 0, fmt.Errorf("write %s: %w", dst, err)
	}
	return 1, nil
}
