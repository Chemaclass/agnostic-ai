package pi

import (
	"slices"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func IsControlOnlyMCP(entry spec.Entry) bool {
	native, ok := entry.Meta["x-pi"].(map[string]any)
	if !ok || len(native) == 0 {
		return false
	}
	for key := range entry.Meta {
		if !slices.Contains([]string{"name", "target", "targets", "target-exclude", "targets-exclude", "x-pi"}, key) {
			return false
		}
	}
	for key, value := range native {
		switch key {
		case "enabled":
			if _, ok := value.(bool); !ok {
				return false
			}
		case "exposure":
			if !validExposure(value) {
				return false
			}
		case "toolExposure":
			tools, ok := value.(map[string]any)
			if !ok {
				return false
			}
			for name, exposure := range tools {
				if name == "" || !validExposure(exposure) {
					return false
				}
			}
		default:
			return false
		}
	}
	return true
}

func validExposure(value any) bool {
	exposure, ok := value.(string)
	return ok && slices.Contains([]string{"codemode", "codemode-deferred", "deferred", "direct", "hidden"}, exposure)
}
