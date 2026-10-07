package spec

import (
	"slices"
	"testing"
)

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
