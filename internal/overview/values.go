package overview

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/preview"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// redacted replaces a value that names or looks like a credential.
const redacted = "<redacted>"

// secretShape matches values that look like a credential: well-known
// token prefixes, a bearer token, or 32 or more key characters in one
// run. The run excludes / and -, so file paths and UUIDs, which those
// break up, stay readable.
var secretShape = regexp.MustCompile(`(?i)\b(sk-[a-z0-9_-]{12,}|ghp_[a-z0-9]{20,}|gh[ousr]_[a-z0-9]{20,}|xox[abprs]-[a-z0-9-]{10,}|xkeysib-[a-z0-9-]{10,}|akia[0-9a-z]{12,}|bearer\s+[a-z0-9._~+/=-]{16,})|[A-Za-z0-9+=_]{32,}`)

// metaSkip are the spec fields that describe the spec rather than set
// anything.
var metaSkip = map[string]bool{"name": true, "description": true}

// specValues flattens a spec's fields into dotted keys, sorted, with
// lists kept whole.
func specValues(meta map[string]any) []Value {
	var out []Value
	var walk func(prefix string, v any)
	walk = func(prefix string, v any) {
		switch t := v.(type) {
		case map[string]any:
			keys := make([]string, 0, len(t))
			for k := range t {
				keys = append(keys, k)
			}
			slices.Sort(keys)
			for _, k := range keys {
				walk(joinKey(prefix, k), t[k])
			}
		case []any:
			list := make([]string, 0, len(t))
			for _, item := range t {
				list = append(list, safeValue(prefix, scalar(item)))
			}
			out = append(out, Value{Key: prefix, Type: ValueList, List: list})
		case bool:
			out = append(out, Value{Key: prefix, Type: ValueBool, Value: scalar(t)})
		default:
			out = append(out, Value{Key: prefix, Type: ValueText, Value: ShownText(prefix, t)})
		}
	}
	keys := make([]string, 0, len(meta))
	for k := range meta {
		if !metaSkip[k] {
			keys = append(keys, k)
		}
	}
	slices.Sort(keys)
	for _, k := range keys {
		walk(k, meta[k])
	}
	if out == nil {
		return []Value{}
	}
	return out
}

func joinKey(prefix, key string) string {
	if prefix == "" {
		return key
	}
	return prefix + "." + key
}

// ShownText is the text a view shows for the scalar under key.
func ShownText(key string, v any) string {
	return safeValue(key, scalar(v))
}

// scalar renders one value: a string as written (an empty one as ""),
// anything else as JSON.
func scalar(v any) string {
	switch t := v.(type) {
	case string:
		if t == "" {
			return `""`
		}
		return t
	case nil:
		return "null"
	}
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(data)
}

// safeValue redacts a value whose key names a credential, or whose text
// looks like one. A reference such as ${API_TOKEN} names a variable and
// shows as written.
func safeValue(key, value string) string {
	last := key
	if i := strings.LastIndex(key, "."); i >= 0 {
		last = key[i+1:]
	}
	if preview.PureReference(value) {
		return value
	}
	if preview.CredentialKey(last) || secretShape.MatchString(value) {
		return redacted
	}
	return preview.RedactText(value)
}

// configValues are the keys the loaded configuration sets.
func configValues(cfg *config.Config) []Value {
	out := []Value{}
	if cfg == nil {
		return out
	}
	if len(cfg.Targets) > 0 {
		out = append(out, Value{Key: "targets", Type: ValueList, List: slices.Clone(cfg.Targets)})
	}
	if cfg.Requires != "" {
		out = append(out, Value{Key: "requires", Type: ValueText, Value: cfg.Requires})
	}
	if len(cfg.Builtins) > 0 {
		out = append(out, Value{Key: "builtins", Type: ValueList, List: slices.Clone(cfg.Builtins)})
	}
	tiers := make([]string, 0, len(cfg.Models))
	for name := range cfg.Models {
		tiers = append(tiers, name)
	}
	slices.Sort(tiers)
	for _, name := range tiers {
		tier := cfg.Models[name]
		targets := make([]string, 0, len(tier.Models))
		for target := range tier.Models {
			targets = append(targets, target)
		}
		slices.Sort(targets)
		list := make([]string, 0, len(targets)+1)
		for _, target := range targets {
			list = append(list, target+": "+tier.Models[target])
		}
		if tier.Effort != nil {
			list = append(list, "effort: "+scalar(tier.Effort))
		}
		out = append(out, Value{Key: "models." + name, Type: ValueList, List: list})
	}
	if cfg.OnUnsupported != "" {
		out = append(out, Value{Key: "on-unsupported", Type: ValueText, Value: cfg.OnUnsupported})
	}
	return out
}

// settingsValues are the values a settings spec sets.
func settingsValues(e spec.Entry) []Value {
	return specValues(e.Meta)
}
