package config

import (
	"fmt"
	"os"
	"reflect"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/suggest"
)

// UnknownKeysError lists the keys config files hold that no setting
// reads. Requires is the loaded config's requires value, so a caller can
// tell an older binary about the upgrade before the keys a newer release
// added.
type UnknownKeysError struct {
	Requires string
	Keys     []string
}

func (e *UnknownKeysError) Error() string { return strings.Join(e.Keys, "\n") }

var unmarshalerType = reflect.TypeFor[yaml.Unmarshaler]()

// unknownKeys reports each key in the YAML file at path that Config does
// not read, as `path:line: unknown key "a.b"`, with a did-you-mean when a
// sibling key is close.
func unknownKeys(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil || len(doc.Content) == 0 {
		return nil, nil
	}
	var out []string
	walkKeys(doc.Content[0], reflect.TypeFor[Config](), "", func(n *yaml.Node, key string, known []string) {
		msg := fmt.Sprintf("%s:%d: unknown key %q", path, n.Line, key)
		if s := suggest.Name(n.Value, known); s != "" {
			msg += fmt.Sprintf(" (did you mean %s?)", s)
		}
		out = append(out, msg)
	})
	return out, nil
}

func walkKeys(n *yaml.Node, t reflect.Type, prefix string, unknown func(n *yaml.Node, key string, known []string)) {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if reflect.PointerTo(t).Implements(unmarshalerType) {
		return
	}
	switch t.Kind() {
	case reflect.Struct:
		if n.Kind != yaml.MappingNode {
			return
		}
		fields, open := yamlFields(t)
		known := make([]string, 0, len(fields))
		for k := range fields {
			known = append(known, k)
		}
		for i := 0; i+1 < len(n.Content); i += 2 {
			key, val := n.Content[i], n.Content[i+1]
			ft, ok := fields[key.Value]
			switch {
			case ok:
				walkKeys(val, ft, prefix+key.Value+".", unknown)
			case !open:
				unknown(key, prefix+key.Value, known)
			}
		}
	case reflect.Map:
		if n.Kind != yaml.MappingNode {
			return
		}
		for i := 0; i+1 < len(n.Content); i += 2 {
			walkKeys(n.Content[i+1], t.Elem(), prefix+n.Content[i].Value+".", unknown)
		}
	case reflect.Slice:
		if n.Kind != yaml.SequenceNode {
			return
		}
		for i, item := range n.Content {
			walkKeys(item, t.Elem(), fmt.Sprintf("%s[%d].", strings.TrimSuffix(prefix, "."), i), unknown)
		}
	}
}

// yamlFields maps each YAML key t reads to its field type. open is true
// when an inline map takes any other key.
func yamlFields(t reflect.Type) (fields map[string]reflect.Type, open bool) {
	fields = map[string]reflect.Type{}
	for f := range t.Fields() {
		if !f.IsExported() {
			continue
		}
		name, opts, _ := strings.Cut(f.Tag.Get("yaml"), ",")
		if name == "-" {
			continue
		}
		if strings.Contains(","+opts+",", ",inline,") {
			ft := f.Type
			for ft.Kind() == reflect.Pointer {
				ft = ft.Elem()
			}
			if ft.Kind() == reflect.Map {
				open = true
				continue
			}
			inner, innerOpen := yamlFields(ft)
			for k, v := range inner {
				fields[k] = v
			}
			open = open || innerOpen
			continue
		}
		if name == "" {
			name = strings.ToLower(f.Name)
		}
		fields[name] = f.Type
	}
	return fields, open
}
