package cli

import (
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/errs"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// sharedSkill holds what only some tools see: a comment, a workspace, and
// a block for each of claude, codex, and cursor.
const sharedSkill = `---
name: demo
description: Demo skill. Use when testing.
# keep this comment
workspaces: [apps/web]
---
Shared body.

::target claude

Claude-only step.

::end

::target codex

Codex-only step.

::end

::target cursor

Cursor-only step.

::end
`

// syncedSharedSkillProject is a project agnostic-ai synced for claude,
// codex, and cursor, with one skill whose spec holds more than each tool
// shows.
func syncedSharedSkillProject(t *testing.T) {
	t.Helper()
	testutil.Chdir(t, t.TempDir())
	silence(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, codex, cursor]\n")
	mustWriteFile(t, "apps/web/.keep", "")
	mustWriteFile(t, ".agnostic-ai/skills/demo/SKILL.md", sharedSkill)
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
}

var sharedSkillNative = map[string]string{
	"claude": ".claude/skills/demo/SKILL.md",
	"codex":  ".agents/skills/demo/SKILL.md",
	"cursor": ".cursor/skills/demo/SKILL.md",
}

// Import rewrote a skill from one tool's view of it, dropping the blocks,
// workspaces, and comments the other tools need (#1938).
func TestImport_LeavesASpecTheToolShowsUnchanged(t *testing.T) {
	for _, source := range []string{"claude", "codex", "cursor"} {
		t.Run(source, func(t *testing.T) {
			syncedSharedSkillProject(t)

			if out, err := runCLI(t, "import", source); err != nil {
				t.Fatalf("import %s: %v\n%s", source, err, out)
			}

			if got := readFile(t, ".agnostic-ai/skills/demo/SKILL.md"); got != sharedSkill {
				t.Errorf("import %s rewrote the skill:\n%s", source, got)
			}
			if out, err := runCLI(t, "sync", "--check"); err != nil {
				t.Errorf("sync --check after import %s: %v\n%s", source, err, out)
			}
		})
	}
}

func TestImport_StopsWhenAToolEditWouldDropWhatOtherToolsNeed(t *testing.T) {
	for _, source := range []string{"claude", "codex", "cursor"} {
		t.Run(source, func(t *testing.T) {
			syncedSharedSkillProject(t)
			native := sharedSkillNative[source]
			mustWriteFile(t, native, strings.Replace(readFile(t, native), "Shared body.", "Edited body.", 1))

			_, err := runCLI(t, "import", source)

			if errs.CodeOf(err) != errs.CodeImportWouldReplace {
				t.Fatalf("import %s = %v, want AAI-203", source, err)
			}
			for _, want := range []string{".agnostic-ai/skills/demo/SKILL.md", "::target blocks"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error lacks %q:\n%v", want, err)
				}
			}
			if got := readFile(t, ".agnostic-ai/skills/demo/SKILL.md"); got != sharedSkill {
				t.Errorf("import %s changed the skill:\n%s", source, got)
			}
		})
	}
}

func TestImport_DryRunListsNoChangeForASpecTheToolShowsUnchanged(t *testing.T) {
	syncedSharedSkillProject(t)

	out := captureStdout(t, func() {
		if _, err := runCLI(t, "import", "claude", "--dry-run", "--diff"); err != nil {
			t.Errorf("import --dry-run --diff: %v", err)
		}
	})

	if !strings.Contains(out, "unchanged .agnostic-ai/skills/demo/SKILL.md") {
		t.Errorf("preview does not list the skill as unchanged:\n%s", out)
	}
}

func TestImport_BringsBackAToolEditOfASpecItShowsWhole(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	silence(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, codex]\n")
	mustWriteFile(t, ".agnostic-ai/skills/demo/SKILL.md", "---\nname: demo\ndescription: Demo skill. Use when testing.\n---\nShared body.\n")
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	native := ".claude/skills/demo/SKILL.md"
	mustWriteFile(t, native, strings.Replace(readFile(t, native), "Shared body.", "Edited body.", 1))

	if out, err := runCLI(t, "import", "claude"); err != nil {
		t.Fatalf("import claude: %v\n%s", err, out)
	}

	if got := readFile(t, ".agnostic-ai/skills/demo/SKILL.md"); !strings.Contains(got, "Edited body.") {
		t.Errorf("import did not bring back the edit:\n%s", got)
	}
}

func TestImport_KeepsKeysTheToolDoesNotShowWhenBringingBackAnEdit(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	silence(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, cursor]\n")
	mustWriteFile(t, "apps/web/.keep", "")
	mustWriteFile(t, ".agnostic-ai/skills/demo/SKILL.md", "---\nname: demo\ndescription: Demo skill. Use when testing.\n# keep this comment\nworkspaces: [apps/web]\n---\nShared body.\n")
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	native := sharedSkillNative["claude"]
	mustWriteFile(t, native, strings.Replace(readFile(t, native), "Shared body.", "Edited body.", 1))

	if out, err := runCLI(t, "import", "claude"); err != nil {
		t.Fatalf("import claude: %v\n%s", err, out)
	}

	got := readFile(t, ".agnostic-ai/skills/demo/SKILL.md")
	for _, want := range []string{"Edited body.", "# keep this comment\nworkspaces: [apps/web]"} {
		if !strings.Contains(got, want) {
			t.Errorf("import claude = %q, want %q", got, want)
		}
	}
}

// Overlays an older release wrote: a bare hook event list and a `hooks`
// placeholder in the settings overlay (#1941).
const (
	oldClaudeOverlay    = "{\n  \"statusLine\": {\n    \"type\": \"command\",\n    \"command\": \"echo hi\"\n  },\n  \"hooks\": null,\n  \"model\": \"opus\"\n}\n"
	oldClaudeHookEvents = "[\n  \"PreToolUse\"\n]\n"
)

func TestImport_LeavesOverlaysAnOlderReleaseWrote(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	silence(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	mustWriteFile(t, ".agnostic-ai/overlays/claude.settings.json", oldClaudeOverlay)
	mustWriteFile(t, ".agnostic-ai/overlays/claude.settings.hook-events.json", oldClaudeHookEvents)
	mustWriteFile(t, ".agnostic-ai/hooks/pre-tool-use.yaml", "event: PreToolUse\nmatcher: Bash\ncommand: echo pre\n")
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	hook := readFile(t, ".agnostic-ai/hooks/pre-tool-use.yaml")

	if out, err := runCLI(t, "import", "claude"); err != nil {
		t.Fatalf("import claude: %v\n%s", err, out)
	}

	for path, want := range map[string]string{
		".agnostic-ai/overlays/claude.settings.json":             oldClaudeOverlay,
		".agnostic-ai/overlays/claude.settings.hook-events.json": oldClaudeHookEvents,
		".agnostic-ai/hooks/pre-tool-use.yaml":                   hook,
	} {
		if got := readFile(t, path); got != want {
			t.Errorf("%s = %q, want it unchanged", path, got)
		}
	}
}

func TestImport_OverwriteDropsWhatTheToolDoesNotShow(t *testing.T) {
	syncedSharedSkillProject(t)
	native := sharedSkillNative["claude"]
	mustWriteFile(t, native, strings.Replace(readFile(t, native), "Shared body.", "Edited body.", 1))

	if out, err := runCLI(t, "import", "claude", "--overwrite"); err != nil {
		t.Fatalf("import --overwrite: %v\n%s", err, out)
	}

	got := readFile(t, ".agnostic-ai/skills/demo/SKILL.md")
	if !strings.Contains(got, "Edited body.") || strings.Contains(got, "Codex-only step.") {
		t.Errorf("import --overwrite = %q, want claude's view", got)
	}
}
