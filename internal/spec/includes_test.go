package spec

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestIncludeRefs_WritesNativeSeparatorsWithSlashes(t *testing.T) {
	body := "intro\n@" + filepath.FromSlash("apps/web/README.md") + "\n@docs/./guide.md\n"

	got := IncludeRefs(body)

	want := []string{"apps/web/README.md", "docs/guide.md"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("IncludeRefs = %q, want %q", got, want)
	}
}
