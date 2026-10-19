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
		if got, _ := importPortableAgentKey([]byte(src), "", "gemini", "max_turns", "maxTurns", false); string(got) != src {
			t.Errorf("changed %q to %q", src, got)
		}
	}
	out, err := importPortableAgentKey([]byte("---\nname: a # keep\nmax_turns: 3 # turns\n---\n\nBody.\n"), "", "gemini", "max_turns", "maxTurns", false)
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	if want := "---\nname: a # keep\nmaxTurns: 3 # turns\n---\n\nBody.\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// A value the spec sets only for one tool goes back under x-<target> on
// re-import, so the shared value other tools get survives.
func TestImport_AgentLimitOverrideStaysTargetScoped(t *testing.T) {
	cases := []struct {
		target, dir, native, spec string
		shared                    map[string]any
		override                  map[string]any
	}{
		{"cursor", ".cursor/agents", "is_background: true\n", "background: true\nx-cursor:\n  is_background: false\n", map[string]any{"background": true}, map[string]any{"is_background": true}},
		{"gemini", ".gemini/agents", "max_turns: 4\n", "maxTurns: 12\nx-gemini:\n  max_turns: 3\n", map[string]any{"maxTurns": 12}, map[string]any{"max_turns": 4}},
		{"gemini", ".gemini/agents", "max_turns: 4\n", "maxTurns: 12\nx-gemini:\n  maxTurns: 3\n", map[string]any{"maxTurns": 12}, map[string]any{"maxTurns": 4}},
		{"opencode", ".opencode/agents", "steps: 4\n", "maxTurns: 12\nx-opencode:\n  steps: 3\n", map[string]any{"maxTurns": 12}, map[string]any{"steps": 4}},
	}
	for _, tc := range cases {
		t.Run(tc.target, func(t *testing.T) {
			root := testutil.TempCwd(t)
			silence(t)
			writeFile(t, filepath.Join(root, "agnostic-ai.yaml"), "version: 1\ntargets: [claude, "+tc.target+"]\n")
			specFile := filepath.Join(root, ".agnostic-ai", "agents", "reviewer.md")
			writeFile(t, specFile, "---\nname: reviewer\ndescription: Review changes.\n"+tc.spec+"---\n\nFind bugs.\n")
			writeFile(t, filepath.Join(root, tc.dir, "reviewer.md"), "---\nname: reviewer\ndescription: Review changes.\n"+tc.native+"---\n\nFind bugs.\n")
			if out, err := runRoot(t, "import", tc.target, "--overwrite"); err != nil {
				t.Fatalf("import: %v\n%s", err, out)
			}
			data, err := os.ReadFile(specFile)
			if err != nil {
				t.Fatal(err)
			}
			meta, _ := splitMdcFrontmatter(data)
			for k, want := range tc.shared {
				if !reflect.DeepEqual(meta[k], want) {
					t.Errorf("shared %s=%v, want %v:\n%s", k, meta[k], want, data)
				}
			}
			custom, _ := meta["x-"+tc.target].(map[string]any)
			for k, want := range tc.override {
				if !reflect.DeepEqual(custom[k], want) {
					t.Errorf("x-%s.%s=%v, want %v:\n%s", tc.target, k, custom[k], want, data)
				}
			}
		})
	}
}

// Gemini and OpenCode native files hold every key, so a key deleted
// there stays deleted, and the write-back keeps order and comments.
func TestImport_AgentLimitWriteBackKeepsOnlyTheScopedKey(t *testing.T) {
	cases := []struct {
		target, dir, native, spec, want string
	}{
		{"opencode", ".opencode/agents", "steps: 4 # tuned\n", "maxTurns: 12\nx-opencode:\n  steps: 3\n  hidden: true\n",
			"---\nname: reviewer\ndescription: Review changes.\nmaxTurns: 12\nx-opencode:\n  steps: 4 # tuned\n---\n\nFind bugs.\n"},
		{"gemini", ".gemini/agents", "max_turns: 4\n", "maxTurns: 12\nx-gemini:\n  max_turns: 3\n  temperature: 0.2\n",
			"---\nname: reviewer\ndescription: Review changes.\nmaxTurns: 12\nx-gemini:\n  max_turns: 4\n---\n\nFind bugs.\n"},
	}
	for _, tc := range cases {
		t.Run(tc.target, func(t *testing.T) {
			root := testutil.TempCwd(t)
			silence(t)
			writeFile(t, filepath.Join(root, "agnostic-ai.yaml"), "version: 1\ntargets: [claude, "+tc.target+"]\n")
			specFile := filepath.Join(root, ".agnostic-ai", "agents", "reviewer.md")
			writeFile(t, specFile, "---\nname: reviewer\ndescription: Review changes.\n"+tc.spec+"---\n\nFind bugs.\n")
			writeFile(t, filepath.Join(root, tc.dir, "reviewer.md"), "---\nname: reviewer\ndescription: Review changes.\n"+tc.native+"---\n\nFind bugs.\n")
			if out, err := runRoot(t, "import", tc.target, "--overwrite"); err != nil {
				t.Fatalf("import: %v\n%s", err, out)
			}
			data, err := os.ReadFile(specFile)
			if err != nil {
				t.Fatal(err)
			}
			if string(data) != tc.want {
				t.Errorf("got:\n%s\nwant:\n%s", data, tc.want)
			}
		})
	}
}
