package spec

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
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

	t.Run("merged literal and key order survive", func(t *testing.T) {
		src := "base: &b {env: {NODE_ENV: !literal production}, toolExposure: {'b*': x, 'a*': y}}\n<<: *b\n"
		e, err := decodeYAMLEntry([]byte(src))
		if err != nil {
			t.Fatal(err)
		}
		if !e.MarkedLiteral("env", "NODE_ENV") {
			t.Fatalf("literal mark lost: %v", e.Literals)
		}
		if got := e.NestedKeys["toolExposure"]; !reflect.DeepEqual(got, []string{"b*", "a*"}) {
			t.Fatalf("nested keys = %v", got)
		}
	})

	t.Run("nested merge keeps key order", func(t *testing.T) {
		e, err := decodeYAMLEntry([]byte("toolExposure: {<<: {'b*': x}, 'a*': y}\n"))
		if err != nil {
			t.Fatal(err)
		}
		if got := e.NestedKeys["toolExposure"]; !reflect.DeepEqual(got, []string{"b*", "a*"}) {
			t.Fatalf("nested keys = %v", got)
		}
	})

	t.Run("repeated keys are errors", func(t *testing.T) {
		for _, src := range []string{"<<: {a: 1, a: 2}\n", "<<: {a: 1}\n<<: {b: 2}\n"} {
			if _, err := decodeYAMLEntry([]byte(src)); err == nil {
				t.Fatalf("no error for %q", src)
			}
		}
	})

	t.Run("markdown error line matches the file", func(t *testing.T) {
		_, _, err := splitFrontmatter([]byte("---\nname: x\n<<: nope\n---\n"))
		var typeErr *yaml.TypeError
		if !errors.As(err, &typeErr) || !strings.Contains(typeErr.Errors[0], "line 3") {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("heavily aliased merges stay fast", func(t *testing.T) {
		var b strings.Builder
		b.WriteString("a0: &a0 {k: v}\n")
		for i := 1; i < 12; i++ {
			fmt.Fprintf(&b, "a%d: &a%d {<<: [%s]}\n", i, i, strings.Repeat(fmt.Sprintf("*a%d,", i-1), 10))
		}
		b.WriteString("<<: *a11\n")
		start := time.Now()
		_, _ = decodeYAMLEntry([]byte(b.String()))
		if d := time.Since(start); d > 2*time.Second {
			t.Fatalf("took %v", d)
		}
	})
}
