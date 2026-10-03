package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The hooks page prints the guard script `init --demo` seeds, so a reader
// copies the code that ships.
func TestDemoHookScript_HooksPageShowsSeededScript(t *testing.T) {
	t.Parallel()
	read := func(rel string) string {
		t.Helper()
		data, err := os.ReadFile(filepath.Join("..", "..", rel))
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	const (
		scriptRel = "internal/cli/initdata/scripts/no-force-push.sh"
		docRel    = "docs/site/content/docs/spec-format/hooks.md"
	)
	if script := read(scriptRel); !strings.Contains(read(docRel), "```sh\n"+script+"```\n") {
		t.Errorf("%s does not show %s verbatim; copy the script into its sh block", docRel, scriptRel)
	}
}
