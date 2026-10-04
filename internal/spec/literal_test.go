package spec

import (
	"path/filepath"
	"testing"
)

func TestParseYAMLBytes_LiteralTagReadsAsTheBareValueAndIsRecorded(t *testing.T) {
	t.Parallel()
	e, err := ParseYAMLBytes(KindMCP, []byte("name: s\ncommand: x\nenv:\n  NODE_ENV: !literal production\n  PORT: !literal 8080\n  TOKEN: ${TOKEN}\nheaders: {X-Mode: !literal fast}\n"))
	if err != nil {
		t.Fatal(err)
	}
	env := e.Meta["env"].(map[string]any)
	if env["NODE_ENV"] != "production" || env["PORT"] != "8080" {
		t.Errorf("env = %#v, want the bare strings", env)
	}
	for _, c := range []struct {
		field, key string
		want       bool
	}{
		{"env", "NODE_ENV", true}, {"env", "PORT", true}, {"env", "TOKEN", false},
		{"headers", "X-Mode", true}, {"headers", "missing", false}, {"args", "x", false},
	} {
		if got := e.MarkedLiteral(c.field, c.key); got != c.want {
			t.Errorf("MarkedLiteral(%s, %s) = %v, want %v", c.field, c.key, got, c.want)
		}
	}
}

func TestParseYAMLBytes_RepeatedKeyKeepsTheMarkOfTheValueThatWins(t *testing.T) {
	t.Parallel()
	e, err := ParseYAMLBytes(KindMCP, []byte("name: s\nenv:\n  A: !literal one\nenv:\n  B: !literal two\n"))
	if err != nil {
		t.Fatal(err)
	}
	if e.MarkedLiteral("env", "A") || !e.MarkedLiteral("env", "B") {
		t.Errorf("literals = %v, want only the second env block's B", e.Literals)
	}
}

func TestLoadLayered_LocalLayerKeepsTheMarkOfTheLayerThatSetsTheValue(t *testing.T) {
	t.Parallel()
	base, local := t.TempDir(), t.TempDir()
	mustWrite(t, filepath.Join(base, "mcps", "s.yaml"), "name: s\ncommand: x\nenv:\n  SHARED: !literal a\n  MODE: !literal b\n")
	mustWrite(t, filepath.Join(local, "mcps", "s.yaml"), "name: s\nenv:\n  MODE: ${MODE}\n  OWN: !literal c\n")

	got := loadExtending(t, base, local).MCPs[0]
	if !got.MarkedLiteral("env", "SHARED") || got.MarkedLiteral("env", "MODE") || !got.MarkedLiteral("env", "OWN") {
		t.Errorf("literals = %v, want SHARED from the base and OWN from local; local's MODE is a reference", got.Literals)
	}
}
