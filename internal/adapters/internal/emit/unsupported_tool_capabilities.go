package emit

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func WithoutUnsupportedDelete(b spec.Bundle, target, mode string) (spec.Bundle, error) {
	filter := func(entries []spec.Entry, field string) ([]spec.Entry, error) {
		out := slices.Clone(entries)
		for i, e := range out {
			if e.Kind == spec.KindAgent && target == "kiro" {
				continue
			}
			custom, _ := e.Meta["x-"+target].(map[string]any)
			if field == "permissions" {
				nativeKey := ""
				switch target {
				case "kilo":
					nativeKey = "permission"
				case "windsurf":
					nativeKey = "permissions"
				}
				if _, authoritative := custom[nativeKey].(map[string]any); nativeKey != "" && authoritative {
					continue
				}
			} else {
				if target == "kilo" && field == "tools" {
					if _, authoritative := custom["permission"]; authoritative {
						continue
					}
				}
				if _, authoritative := custom[field]; authoritative {
					continue
				}
			}

			meta := e.Meta
			if field == "permissions" {
				meta, _ = e.Meta[field].(map[string]any)
			}
			keys := []string{field}
			if field == "permissions" {
				keys = []string{"allow", "ask", "deny"}
			}
			for _, key := range keys {
				values := StringSlice(meta[key])
				if scalar, ok := meta[key].(string); ok {
					values = append(values, spec.SplitToolList(scalar)...)
				}
				var kept []string
				removed := false
				for _, value := range values {
					if value == "Delete" {
						removed = true
						continue
					}
					kept = append(kept, value)
				}
				if !removed {
					continue
				}
				source := field
				if field == "tools" && e.CapabilityField != "" {
					source = e.CapabilityField
				}
				if field == "permissions" {
					source += "." + key
				}
				message := fmt.Sprintf("%s: %s: %s capability delete has no native tool or rule", target, e.Path, source)
				if mode == OnUnsupportedError {
					return nil, fmt.Errorf("%s", message)
				}
				if mode != OnUnsupportedSilent {
					NoteFieldNoOp(target, e.Kind, source, 1, "delete has no native tool or rule")
				}
				e.Meta = maps.Clone(e.Meta)
				if field == "permissions" {
					meta = maps.Clone(meta)
					e.Meta[field] = meta
				} else {
					meta = e.Meta
				}
				if _, scalar := meta[key].(string); scalar {
					meta[key] = strings.Join(kept, ", ")
				} else {
					items := make([]any, len(kept))
					for j, value := range kept {
						items[j] = value
					}
					meta[key] = items
				}
			}
			out[i] = e
		}
		return out, nil
	}
	var err error
	b.Agents, err = filter(b.Agents, "tools")
	if err != nil {
		return b, err
	}
	b.Skills, err = filter(b.Skills, "allowed-tools")
	if err != nil {
		return b, err
	}
	b.Settings, err = filter(b.Settings, "permissions")
	return b, err
}
