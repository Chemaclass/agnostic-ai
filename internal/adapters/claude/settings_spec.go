package claude

import (
	"encoding/json"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// buildSpecSettings renders the tool-neutral `settings` specs
// (`.agnostic-ai/settings/*.yaml`) into the map shape `.claude/settings.json`
// expects. It maps the cross-tool fields agnostic-ai defines today:
//
//	permissions.allow / deny / ask  -> permissions.{allow,deny,ask}
//	model                           -> model
//
// Multiple settings specs merge: permission lists concatenate (de-duped,
// source order preserved) and the last non-empty model wins. Returns an
// empty map when no spec contributes a value, so the caller can layer it
// unconditionally. Claude-specific `outputs.claude.settings` config is
// layered on top by the caller and therefore overrides these values.
func buildSpecSettings(entries []spec.Entry) map[string]any {
	out := map[string]any{}
	var permLayers []map[string]any
	for _, e := range entries {
		if m, ok := e.Meta["model"].(string); ok && m != "" {
			out["model"] = m
		}
		if p, ok := e.Meta["permissions"].(map[string]any); ok {
			permLayers = append(permLayers, p)
		}
	}
	if perms := mergePermissions(permLayers...); perms != nil {
		out["permissions"] = perms
	}
	return out
}

// mergePermissions unions the allow/deny/ask lists across the given
// permission layers in order (base first), de-duping while preserving
// first-seen order. Each layer is the `permissions` sub-map of one settings
// source (overlay/disk base, then spec, then config).
//
// The result is seeded from the base layer (layers[0]) so sibling keys this
// adapter does not model (e.g. defaultMode, additionalDirectories) carry
// through verbatim instead of being dropped when the object is re-emitted.
// Only the three list keys are recomputed. Returns nil when no layer
// contributes anything, so the caller can skip writing the key.
func mergePermissions(layers ...map[string]any) map[string]any {
	out := map[string]any{}
	if len(layers) > 0 {
		// Carry through only the sibling keys this adapter does not model;
		// the three lists are recomputed below from every layer.
		for k, v := range layers[0] {
			switch k {
			case "allow", "deny", "ask":
			default:
				out[k] = v
			}
		}
	}
	allow, deny, ask := []any{}, []any{}, []any{}
	for _, p := range layers {
		if p == nil {
			continue
		}
		allow = appendUniqueStrings(allow, p["allow"])
		deny = appendUniqueStrings(deny, p["deny"])
		ask = appendUniqueStrings(ask, p["ask"])
	}
	setPermissionList(out, "allow", allow)
	setPermissionList(out, "deny", deny)
	setPermissionList(out, "ask", ask)
	if len(out) == 0 {
		return nil
	}
	return out
}

// setPermissionList writes the unioned list for key, or removes the key when
// the union is empty so an empty base list never lingers as `[]`.
func setPermissionList(perms map[string]any, key string, vals []any) {
	if len(vals) > 0 {
		perms[key] = vals
		return
	}
	delete(perms, key)
}

// docPermissions reads the `permissions` object already present in the
// settings document (the captured overlay or the on-disk settings.json
// base). Returns nil when absent or unparseable, so an unreadable base
// degrades to "no base rules" rather than failing the whole emit.
func docPermissions(doc *emit.OrderedJSON) map[string]any {
	raw, ok := doc.Get("permissions")
	if !ok {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil
	}
	return m
}

// mapOf returns v as a map[string]any, or nil when v is not a map. Used to
// pull the `permissions` sub-map out of an assembled settings map.
func mapOf(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

// appendUniqueStrings appends each string in raw (a YAML-decoded `[]any`
// or `[]string`) to acc, skipping empties and values already present.
func appendUniqueStrings(acc []any, raw any) []any {
	seen := make(map[string]struct{}, len(acc))
	for _, v := range acc {
		if s, ok := v.(string); ok {
			seen[s] = struct{}{}
		}
	}
	add := func(s string) {
		if s == "" {
			return
		}
		if _, dup := seen[s]; dup {
			return
		}
		seen[s] = struct{}{}
		acc = append(acc, s)
	}
	switch xs := raw.(type) {
	case []any:
		for _, v := range xs {
			if s, ok := v.(string); ok {
				add(s)
			}
		}
	case []string:
		for _, s := range xs {
			add(s)
		}
	}
	return acc
}
