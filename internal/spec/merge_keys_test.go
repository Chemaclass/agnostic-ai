package spec

import (
	"reflect"
	"strings"
	"testing"
)

func TestDecodeYAMLEntryMergeKeys(t *testing.T) {
	t.Run("mcp spec splices the aliased mapping", func(t *testing.T) {
		e, err := decodeYAMLEntry([]byte("base: &b {command: npx, args: [server]}\n<<: *b\n"))
		if err != nil {
			t.Fatal(err)
		}
		if e.Meta["command"] != "npx" {
			t.Fatalf("command = %v, want npx; meta = %v", e.Meta["command"], e.Meta)
		}
		if _, ok := e.Meta["<<"]; ok {
			t.Fatalf("literal << key kept: %v", e.Meta)
		}
		if !reflect.DeepEqual(e.MetaKeys, []string{"base", "command", "args"}) {
			t.Fatalf("keys = %v", e.MetaKeys)
		}
		if len(e.DuplicateKeys) != 0 {
			t.Fatalf("duplicates = %v", e.DuplicateKeys)
		}
	})

	t.Run("settings spec keeps explicit keys over merged ones", func(t *testing.T) {
		src := "defaults: &d\n  permissions: {allow: [Read]}\n  model: fast\nother: &o {tools: [a]}\n<<: [*d, *o]\nmodel: slow\n"
		e, err := decodeYAMLEntry([]byte(src))
		if err != nil {
			t.Fatal(err)
		}
		if e.Meta["model"] != "slow" {
			t.Fatalf("model = %v, want slow", e.Meta["model"])
		}
		if _, ok := e.Meta["permissions"].(map[string]any); !ok {
			t.Fatalf("permissions not merged: %v", e.Meta)
		}
		if _, ok := e.Meta["tools"]; !ok {
			t.Fatalf("tools not merged: %v", e.Meta)
		}
		if len(e.DuplicateKeys) != 0 {
			t.Fatalf("duplicates = %v", e.DuplicateKeys)
		}
	})

	t.Run("earlier merge source wins", func(t *testing.T) {
		e, err := decodeYAMLEntry([]byte("a: &a {x: 1}\nb: &b {x: 2}\n<<: [*a, *b]\n"))
		if err != nil {
			t.Fatal(err)
		}
		if e.Meta["x"] != 1 {
			t.Fatalf("x = %v, want 1", e.Meta["x"])
		}
	})

	t.Run("inline mapping", func(t *testing.T) {
		e, err := decodeYAMLEntry([]byte("<<: {command: npx}\n"))
		if err != nil {
			t.Fatal(err)
		}
		if e.Meta["command"] != "npx" {
			t.Fatalf("meta = %v", e.Meta)
		}
	})

	t.Run("scalar merge value is an error", func(t *testing.T) {
		_, err := decodeYAMLEntry([]byte("<<: nope\n"))
		if err == nil || !strings.Contains(err.Error(), "merge") {
			t.Fatalf("err = %v", err)
		}
	})
}
