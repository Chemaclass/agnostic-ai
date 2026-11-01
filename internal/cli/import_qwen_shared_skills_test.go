package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestImportQwen_NativeSourceLinkWinsOverSharedDuplicate(t *testing.T) {
	for _, tc := range []struct {
		name  string
		flags []string
	}{
		{name: "preview", flags: []string{"--dry-run", "--diff"}},
		{name: "import"},
		{name: "overwrite", flags: []string{"--overwrite"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testutil.Chdir(t, t.TempDir())
			silence(t)
			writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [qwen]\n")
			const native = "---\nname: demo\ndescription: Demo skill.\n---\nNative guidance.\n"
			writeFile(t, ".agnostic-ai/skills/demo/SKILL.md", native)
			writeFile(t, ".agnostic-ai/skills/demo/asset.txt", "Native asset.\n")
			writeFile(t, ".agents/skills/demo/SKILL.md", strings.ReplaceAll(native, "Native", "Shared"))
			writeFile(t, ".agents/skills/demo/asset.txt", "Shared asset.\n")
			writeFile(t, ".agents/skills/demo/shared.txt", "Shared-only asset.\n")
			if err := os.MkdirAll(".qwen/skills", 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.FromSlash("../../.agnostic-ai/skills/demo"), ".qwen/skills/demo"); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}
			out, err := runCLI(t, append([]string{"import", "qwen"}, tc.flags...)...)
			if err != nil {
				t.Errorf("import: %v\n%s", err, out)
			}
			for _, path := range []string{".agnostic-ai/skills/demo/SKILL.md", ".qwen/skills/demo/SKILL.md"} {
				if got := readFile(t, path); got != native {
					t.Errorf("%s = %q, want native skill", path, got)
				}
			}
			if got := readFile(t, ".agnostic-ai/skills/demo/asset.txt"); got != "Native asset.\n" {
				t.Errorf("native asset = %q", got)
			}
			if _, err := os.Stat(".agnostic-ai/skills/demo/shared.txt"); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("shared asset imported: %v", err)
			}
		})
	}
}

func TestImportQwen_SyncedPeerSkillCopyLeavesTheSpecUnchanged(t *testing.T) {
	for _, tc := range []struct {
		name  string
		flags []string
	}{
		{name: "import"},
		{name: "overwrite", flags: []string{"--overwrite"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testutil.Chdir(t, t.TempDir())
			silence(t)
			writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [qwen, codex]\n")
			const spec = "---\nname: demo\ndescription: Demo skill.\ntargets-exclude: [qwen]\n---\nCodex only.\n"
			writeFile(t, ".agnostic-ai/skills/demo/SKILL.md", spec)
			if out, err := runCLI(t, "sync"); err != nil {
				t.Fatalf("sync: %v\n%s", err, out)
			}
			out, err := runCLI(t, append([]string{"import", "qwen"}, tc.flags...)...)
			if err != nil {
				t.Errorf("import: %v\n%s", err, out)
			}
			if got := readFile(t, ".agnostic-ai/skills/demo/SKILL.md"); got != spec {
				t.Errorf("spec changed:\n%s", got)
			}
		})
	}
}
