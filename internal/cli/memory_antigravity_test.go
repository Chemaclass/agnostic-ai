package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSync_AntigravityMemoryIncludesAreStaticAndTargeted(t *testing.T) {
	for _, tc := range []struct {
		name, extra, rule string
		personal          bool
	}{
		{"checkout", "", ".agents/rules/antigravity-memory-index.md", true},
		{"custom directory", "outputs:\n  antigravity:\n    rules-dir: native/custom/rules\n", "native/custom/rules/antigravity-memory-index.md", true},
		{"repo personal", "", ".agents/rules/antigravity-memory-index.md", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			globalAgentTestHome(t)
			root := setupMemoryConfig(t, "version: 1\ntargets: [antigravity, codex]\nbuiltins: [memory]\n"+tc.extra)
			gitInit(t, root)
			if !tc.personal {
				writeFile(t, "agnostic-ai.local.yaml", "memory:\n  personal: repo\n")
			}
			if out, err := runCLI(t, "sync"); err != nil {
				t.Fatalf("sync: %v\n%s", err, out)
			}
			before := readFile(t, tc.rule)
			if !strings.HasPrefix(before, "---\ntrigger: always_on\n") {
				t.Errorf("missing always-on frontmatter: %s", before)
			}
			for _, index := range []string{".agnostic-ai/memory/MEMORY.md", ".agnostic-ai/local/memory/MEMORY.md"} {
				ref, err := filepath.Rel(filepath.Dir(tc.rule), filepath.FromSlash(index))
				if err != nil {
					t.Fatal(err)
				}
				want := index == ".agnostic-ai/memory/MEMORY.md" || tc.personal
				if strings.Contains(before, "]("+filepath.ToSlash(ref)+")") != want {
					t.Errorf("include %s wanted=%t: %s", index, want, before)
				}
				if _, err := os.Stat(index); !os.IsNotExist(err) {
					t.Errorf("sync created a missing index %s: %v", index, err)
				}
			}
			if strings.Contains(readFile(t, "AGENTS.md"), "@[Project memory]") || strings.Contains(readFile(t, "AGENTS.md"), "antigravity-memory-index") {
				t.Error("Antigravity include reached Codex output")
			}
			for _, index := range []string{".agnostic-ai/memory/MEMORY.md", ".agnostic-ai/local/memory/MEMORY.md"} {
				writeFile(t, index, "- Synthetic index fact.\n")
			}
			if out, err := runCLI(t, "sync", "--check"); err != nil {
				t.Errorf("index existence changed sync: %v\n%s", err, out)
			}
			if got := readFile(t, tc.rule); got != before {
				t.Error("index existence changed native include bytes")
			}
			if !tc.personal && !strings.Contains(readFile(t, "AGENTS.md"), "agnostic-ai memory path") {
				t.Error("repo personal memory lost the shared lookup fallback")
			}
		})
	}
}

func TestSyncGlobal_AntigravityMemoryIncludeStaysProjectOnly(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "agnostic-ai.yaml"), "targets: [antigravity]\nbuiltins: [memory]\n")
	if _, notes, err := runGlobalAgentTest(); err != nil {
		t.Fatalf("global sync: %v\n%s", err, notes)
	}
	body := readGlobalTest(t, filepath.Join(home, ".gemini", "GEMINI.md"))
	if strings.Contains(body, "antigravity-memory-index") || strings.Contains(body, "@[Project memory]") {
		t.Error("project includes reached global instructions")
	}
	if !strings.Contains(body, "shared-memory-policy") {
		t.Error("global memory lookup fallback missing")
	}
}
