package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestImportOpencodeAgentV2ReconstructsWithoutNativeFiles(t *testing.T) {
	for _, front := range []string{"disabled: false\n", "system: Native prompt\n", "request: {}\n", ""} {
		t.Run(strings.TrimSpace(front), func(t *testing.T) {
			dir := testutil.TempCwd(t)
			silence(t)
			writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [opencode]\n")
			native := "---\ndescription: Review\n" + front + "permissions:\n  - action: read\n    resource: secrets/public/*\n    effect: allow\n  - action: read\n    resource: secrets/*\n    effect: deny\n---\nReview.\n"
			path := filepath.Join(dir, ".opencode/agents/reviewer.md")
			writeFile(t, path, native)
			execCLI(t, "import", "opencode")
			imported, err := spec.ParseMarkdownBytes(spec.KindAgent, []byte(readFile(t, filepath.Join(dir, ".agnostic-ai/agents/reviewer.md"))))
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := imported.Meta["x-opencode"].(map[string]any); !ok {
				t.Fatalf("native V2 fields were not scoped: %#v", imported.Meta)
			}
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			execCLI(t, "sync")
			execCLI(t, "sync", "--check")
			execCLI(t, "lint", "--strict")
			actual, err := spec.ParseMarkdownBytes(spec.KindAgent, []byte(readFile(t, path)))
			if err != nil {
				t.Fatal(err)
			}
			expected, err := spec.ParseMarkdownBytes(spec.KindAgent, []byte(native))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(actual.Meta, expected.Meta) {
				t.Errorf("native fields changed: got %#v, want %#v", actual.Meta, expected.Meta)
			}
		})
	}
}
