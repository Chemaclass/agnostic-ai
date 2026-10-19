package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// A native turn limit or background flag imports as the portable field
// and syncs back to the same native key.
func TestImport_AgentTurnLimitAndBackgroundRoundTrip(t *testing.T) {
	cases := []struct {
		target, dir string
		native      string
		portable    map[string]any
	}{
		{"claude", ".claude/agents", "maxTurns: 12\nbackground: true\n", map[string]any{"maxTurns": 12, "background": true}},
		{"junie", ".junie/agents", "maxTurns: 12\n", map[string]any{"maxTurns": 12}},
		{"qoder", ".qoder/agents", "maxTurns: 12\nbackground: true\n", map[string]any{"maxTurns": 12, "background": true}},
		{"gemini", ".gemini/agents", "max_turns: 12\n", map[string]any{"maxTurns": 12}},
		{"cursor", ".cursor/agents", "is_background: true\n", map[string]any{"background": true}},
		{"opencode", ".opencode/agents", "steps: 12\n", map[string]any{"maxTurns": 12}},
	}
	for _, tc := range cases {
		t.Run(tc.target, func(t *testing.T) {
			root := testutil.TempCwd(t)
			silence(t)
			writeFile(t, filepath.Join(root, "agnostic-ai.yaml"), "version: 1\ntargets: ["+tc.target+"]\n")
			nativeFile := filepath.Join(root, tc.dir, "reviewer.md")
			writeFile(t, nativeFile, "---\nname: reviewer\ndescription: Review changes.\n"+tc.native+"---\n\nFind bugs.\n")
			if out, err := runRoot(t, "import", tc.target); err != nil {
				t.Fatalf("import: %v\n%s", err, out)
			}
			data, err := os.ReadFile(filepath.Join(root, ".agnostic-ai", "agents", "reviewer.md"))
			if err != nil {
				t.Fatal(err)
			}
			meta, _ := splitMdcFrontmatter(data)
			for k, want := range tc.portable {
				if !reflect.DeepEqual(meta[k], want) {
					t.Errorf("%s=%v, want %v; imported:\n%s", k, meta[k], want, data)
				}
			}
			if err := os.Remove(nativeFile); err != nil {
				t.Fatal(err)
			}
			mustSync(t, "--only", tc.target)
			synced, err := os.ReadFile(nativeFile)
			if err != nil {
				t.Fatal(err)
			}
			for _, line := range strings.Split(strings.TrimSpace(tc.native), "\n") {
				if !strings.Contains(string(synced), line) {
					t.Errorf("sync lost %q:\n%s", line, synced)
				}
			}
		})
	}
}

func TestImportPortableAgentKey_KeepsANativeValueItCannotMap(t *testing.T) {
	for _, src := range []string{
		"---\nmax_turns: lots\n---\n\nBody.\n",
		"---\nmax_turns: 3\nmaxTurns: 4\n---\n\nBody.\n",
		"---\nname: a\n---\n\nBody.\n",
	} {
		if got := string(importPortableAgentKey([]byte(src), "max_turns", "maxTurns")); got != src {
			t.Errorf("changed %q to %q", src, got)
		}
	}
	got := string(importPortableAgentKey([]byte("---\nname: a # keep\nmax_turns: 3 # turns\n---\n\nBody.\n"), "max_turns", "maxTurns"))
	if want := "---\nname: a # keep\nmaxTurns: 3 # turns\n---\n\nBody.\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
