package config

import (
	"fmt"
	"reflect"

	"gopkg.in/yaml.v3"
)

// SupportedVersion is the only config schema version this release reads.
const SupportedVersion = 1

// typeErrors reports each value in the YAML document data, read from
// path, that has a shape Config cannot hold, as
// `path:line: key: expected <shape>, got <shape>`. A document that is
// not valid YAML reports nothing; decoding it names the syntax error.
func typeErrors(path string, data []byte) []string {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil || len(doc.Content) == 0 {
		return nil
	}
	var out []string
	walkTypes(doc.Content[0], reflect.TypeFor[Config](), "", func(n *yaml.Node, key, want string) {
		if key != "" {
			key += ": "
		}
		out = append(out, fmt.Sprintf("%s:%d: %sexpected %s, got %s", path, n.Line, key, want, nodeShape(n)))
	})
	return out
}

func walkTypes(n *yaml.Node, t reflect.Type, key string, wrong func(n *yaml.Node, key, want string)) {
	if n.Kind == yaml.AliasNode {
		n = n.Alias
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if n.Kind == yaml.ScalarNode && n.Tag == "!!null" || reflect.PointerTo(t).Implements(unmarshalerType) {
		return
	}
	switch t.Kind() {
	case reflect.Struct, reflect.Map:
		if n.Kind != yaml.MappingNode {
			wrong(n, key, "a map")
			return
		}
		var fields map[string]reflect.Type
		if t.Kind() == reflect.Struct {
			fields, _ = yamlFields(t)
		}
		for i := 0; i+1 < len(n.Content); i += 2 {
			k, v := n.Content[i], n.Content[i+1]
			if k.Tag == "!!merge" {
				walkMergedTypes(v, t, key, wrong)
				continue
			}
			ft := t
			if t.Kind() == reflect.Map {
				ft = t.Elem()
			} else if ft = fields[k.Value]; ft == nil {
				continue
			}
			walkTypes(v, ft, joinKey(key, k.Value), wrong)
		}
	case reflect.Slice:
		if n.Kind != yaml.SequenceNode {
			wrong(n, key, "a list")
			return
		}
		for i, item := range n.Content {
			walkTypes(item, t.Elem(), fmt.Sprintf("%s[%d]", key, i), wrong)
		}
	case reflect.Interface:
	default:
		if n.Kind != yaml.ScalarNode || n.Decode(reflect.New(t).Interface()) != nil {
			wrong(n, key, scalarShape(t))
		}
	}
}

func walkMergedTypes(n *yaml.Node, t reflect.Type, key string, wrong func(n *yaml.Node, key, want string)) {
	if n.Kind == yaml.AliasNode {
		n = n.Alias
	}
	if n.Kind == yaml.SequenceNode {
		for _, item := range n.Content {
			walkMergedTypes(item, t, key, wrong)
		}
		return
	}
	walkTypes(n, t, key, wrong)
}

func joinKey(prefix, key string) string {
	if prefix == "" {
		return key
	}
	return prefix + "." + key
}

func scalarShape(t reflect.Type) string {
	switch t.Kind() {
	case reflect.Bool:
		return "true or false"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "a whole number"
	case reflect.Float32, reflect.Float64:
		return "a number"
	default:
		return "a string"
	}
}

func nodeShape(n *yaml.Node) string {
	switch {
	case n.Kind == yaml.MappingNode:
		return "a map"
	case n.Kind == yaml.SequenceNode:
		return "a list"
	case n.Tag == "!!bool":
		return n.Value
	case n.Tag == "!!int" || n.Tag == "!!float":
		return "the number " + n.Value
	default:
		return fmt.Sprintf("the string %q", n.Value)
	}
}

// versionError refuses a version key other than SupportedVersion. A
// missing version is left to `doctor config`, and one that is not a
// whole number to typeErrors.
func versionError(path string, data []byte) string {
	var doc struct {
		Version yaml.Node `yaml:"version"`
	}
	var version int
	if yaml.Unmarshal(data, &doc) != nil || doc.Version.Tag != "!!int" || doc.Version.Decode(&version) != nil || version == SupportedVersion {
		return ""
	}
	return fmt.Sprintf("%s:%d: version: %d is not supported; this release reads version: %d", path, doc.Version.Line, version, SupportedVersion)
}
