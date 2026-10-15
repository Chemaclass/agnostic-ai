package spec

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// A YAML alias that refers to its own ancestor is a cycle. Loading the
// spec finishes, whatever kind it is.
func TestKeyOrder_CyclicAliasFinishes(t *testing.T) {
	for _, kind := range []Kind{KindMCP, KindSettings, KindHook} {
		done := make(chan struct{})
		go func() {
			defer close(done)
			e, err := ParseYAMLBytes(kind, []byte("name: x\ncommand: y\nenv: &loop\n  SELF: *loop\n"))
			if err == nil && e.KeyOrder("env") != nil {
				t.Errorf("%s: recorded an order for a value that did not decode: %v", kind, e.KeyOrder("env"))
			}
		}()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatalf("%s: loading a cyclic alias did not finish", kind)
		}
	}
}

func TestKeyOrder_RecordsNestedObjectsInSourceOrder(t *testing.T) {
	e, err := ParseYAMLBytes(KindSettings, []byte("x-opencode:\n  permission:\n    external_directory:\n      /tmp/a/**: allow\n      \"*\": deny\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := e.KeyOrder("x-opencode", "permission", "external_directory"), []string{"/tmp/a/**", "*"}; !slices.Equal(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}
}

// A local layer that repeats a key keeps the shared layer's place for it,
// so an inherited catch-all allow stays ahead of a specific deny.
func TestKeyOrder_LocalLayerKeepsTheSharedOrder(t *testing.T) {
	base, err := ParseYAMLBytes(KindSettings, []byte("x-opencode:\n  permission:\n    external_directory:\n      \"*\": allow\n      /secret/**: deny\n"))
	if err != nil {
		t.Fatal(err)
	}
	over, err := ParseYAMLBytes(KindSettings, []byte("x-opencode:\n  permission:\n    external_directory:\n      /secret/**: deny\n      /new/**: allow\n"))
	if err != nil {
		t.Fatal(err)
	}
	merged := extendEntry(base, over)
	if got, want := merged.KeyOrder("x-opencode", "permission", "external_directory"), []string{"*", "/secret/**", "/new/**"}; !slices.Equal(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}
}

func TestKeyOrder_RecordsMarkdownPermissionOrder(t *testing.T) {
	e, err := ParseMarkdownBytes(KindAgent, []byte("---\npermission:\n  read:\n    secrets/public/*: allow\n    secrets/*: deny\n  bash: ask\n---\nReview.\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := e.KeyOrder("permission"), []string{"read", "bash"}; !slices.Equal(got, want) {
		t.Errorf("tool order = %v, want %v", got, want)
	}
	if got, want := e.KeyOrder("permission", "read"), []string{"secrets/public/*", "secrets/*"}; !slices.Equal(got, want) {
		t.Errorf("pattern order = %v, want %v", got, want)
	}
}

func TestKeyOrder_LoadsMarkdownPermissionOrder(t *testing.T) {
	dir := testutil.TempCwd(t)
	path := filepath.Join(dir, "reviewer.md")
	data := []byte("---\nx-opencode:\n  permission:\n    read:\n      secrets/public/*: allow\n      secrets/*: deny\n    bash: ask\n---\nReview.\n")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	e, err := parseMarkdown(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := e.KeyOrder("x-opencode", "permission"), []string{"read", "bash"}; !slices.Equal(got, want) {
		t.Errorf("tool order = %v, want %v", got, want)
	}
	if got, want := e.KeyOrder("x-opencode", "permission", "read"), []string{"secrets/public/*", "secrets/*"}; !slices.Equal(got, want) {
		t.Errorf("pattern order = %v, want %v", got, want)
	}
}
