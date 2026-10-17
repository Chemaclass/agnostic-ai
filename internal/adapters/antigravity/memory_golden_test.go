package antigravity

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/builtins"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestMemoryInclude_GoldenSnapshot(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))
	t.Setenv("LocalAppData", filepath.Join(home, "cache"))
	cache, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(cache, 0o755); err != nil {
		t.Fatal(err)
	}
	root, cleanup, err := builtins.Materialize("memory")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := cleanup(); err != nil {
			t.Errorf("cleanup %s: %v", root, err)
		}
	})
	bundle, err := spec.LoadLayered([]spec.Layer{{Name: "builtin", Root: root, Sources: config.Sources{Rules: "rules"}}})
	if err != nil {
		t.Fatal(err)
	}
	var include spec.Entry
	for _, entry := range bundle.Rules {
		if entry.Name == "antigravity-memory-index" {
			include = entry
		}
	}
	if include.Name == "" {
		t.Fatal("memory built-in lacks Antigravity's native include")
	}
	for _, target := range config.DefaultTargets() {
		if include.EmitsTo(target) != (target == "antigravity") {
			t.Errorf("native include targets %s incorrectly", target)
		}
	}
	dir := testutil.TempCwd(t)
	if err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{include}), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	testutil.AssertGoldenTree(t, dir, filepath.Join(cwd, "testdata", "memory-include"))
}
