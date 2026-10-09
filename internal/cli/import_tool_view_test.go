package cli

import (
	"fmt"
	"path/filepath"
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

func TestImport_LeavesASpecEveryToolShowsUnchangedInOneRun(t *testing.T) {
	syncedSharedSkillProject(t)

	for range 2 {
		if out, err := runCLI(t, "import", "claude", "codex", "cursor"); err != nil {
			t.Fatalf("import claude codex cursor: %v\n%s", err, out)
		}
	}

	if got := readFile(t, ".agnostic-ai/skills/demo/SKILL.md"); got != sharedSkill {
		t.Errorf("import rewrote the skill:\n%s", got)
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

// A spec that does not load must not switch the check off for the rest.
func TestImport_StopsOnAToolEditWhileAnotherSpecDoesNotLoad(t *testing.T) {
	syncedSharedSkillProject(t)
	mustWriteFile(t, ".agnostic-ai/rules/broken.md", "---\ndescription: [unclosed\n---\nBroken.\n")
	native := sharedSkillNative["claude"]
	mustWriteFile(t, native, strings.Replace(readFile(t, native), "Shared body.", "Edited body.", 1))

	_, err := runCLI(t, "import", "claude")

	if errs.CodeOf(err) != errs.CodeImportWouldReplace || !strings.Contains(err.Error(), "::target blocks") {
		t.Fatalf("import claude = %v, want AAI-203 naming the ::target blocks", err)
	}
	if got := readFile(t, ".agnostic-ai/skills/demo/SKILL.md"); got != sharedSkill {
		t.Errorf("import changed the skill:\n%s", got)
	}
}

// A preview with skills under an absolute source directory decides as the
// real import does.
func TestImport_PreviewMatchesTheRealImportForAnAbsoluteSkillSource(t *testing.T) {
	for _, edited := range []bool{false, true} {
		t.Run(map[bool]string{false: "unchanged", true: "edited"}[edited], func(t *testing.T) {
			testutil.Chdir(t, t.TempDir())
			silence(t)
			external := t.TempDir()
			mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, codex]\nsources:\n  skills: "+filepath.ToSlash(external)+"\n")
			skill := filepath.Join(external, "demo", "SKILL.md")
			mustWriteFile(t, skill, sharedSkill)
			if out, err := runCLI(t, "sync"); err != nil {
				t.Fatalf("sync: %v\n%s", err, out)
			}
			if edited {
				native := sharedSkillNative["claude"]
				mustWriteFile(t, native, strings.Replace(readFile(t, native), "Shared body.", "Edited body.", 1))
			}

			out, previewErr := runAbsoluteImportCLI(t, "import", "claude", "--dry-run", "--diff")
			_, realErr := runAbsoluteImportCLI(t, "import", "claude")

			if (previewErr == nil) != (realErr == nil) {
				t.Errorf("preview error %v, real error %v", previewErr, realErr)
			}
			if edited && errs.CodeOf(realErr) != errs.CodeImportWouldReplace {
				t.Errorf("real import = %v, want AAI-203", realErr)
			}
			if !edited && !strings.Contains(out, "unchanged "+filepath.ToSlash(skill)) {
				t.Errorf("preview does not list the skill as unchanged:\n%s", out)
			}
			if got := readFile(t, skill); got != sharedSkill {
				t.Errorf("import changed the skill:\n%s", got)
			}
		})
	}
}

// A helper script no hook names is still the user's file: an edit to it
// comes back.
func TestImport_BringsBackAnEditToAHookHelperScript(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	silence(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	mustWriteFile(t, ".claude/hooks/fmt.sh", "#!/bin/sh\n. \"$(dirname \"$0\")/common.sh\"\nfmt\n")
	mustWriteFile(t, ".claude/hooks/common.sh", "fmt() { echo one; }\n")
	mustWriteFile(t, ".claude/settings.json", `{"hooks":{"PostToolUse":[{"matcher":"Edit","hooks":[{"type":"command","command":".claude/hooks/fmt.sh"}]}]}}`)
	for _, args := range [][]string{{"import", "claude"}, {"sync"}} {
		if out, err := runCLI(t, args...); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}
	mustWriteFile(t, ".claude/hooks/common.sh", "fmt() { echo two; }\n")

	if out, err := runCLI(t, "import", "claude"); err != nil {
		t.Fatalf("import claude: %v\n%s", err, out)
	}

	if got := readFile(t, ".agnostic-ai/scripts/claude/common.sh"); got != "fmt() { echo two; }\n" {
		t.Errorf("helper script = %q, want the edit", got)
	}
}

// Gemini inlines rules into GEMINI.md, so a rule renders to no file of
// its own; an unedited GEMINI.md still leaves the rule as it is.
func TestImport_LeavesARuleTheToolInlinesUnchanged(t *testing.T) {
	for name, rule := range map[string]string{
		"blocks": "---\ndescription: Style.\n---\nShared rule.\n\n::target claude\n\nClaude-only rule.\n\n::end\n",
		"plain":  "---\ndescription: Style.\n---\nShared rule.\n",
	} {
		t.Run(name, func(t *testing.T) {
			testutil.Chdir(t, t.TempDir())
			silence(t)
			mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, gemini]\n")
			mustWriteFile(t, ".agnostic-ai/rules/style.md", rule)
			if out, err := runCLI(t, "sync"); err != nil {
				t.Fatalf("sync: %v\n%s", err, out)
			}

			if out, err := runCLI(t, "import", "gemini"); err != nil {
				t.Fatalf("import gemini: %v\n%s", err, out)
			}

			if got := readFile(t, ".agnostic-ai/rules/style.md"); got != rule {
				t.Errorf("import gemini rewrote the rule:\n%s", got)
			}
		})
	}
}

// An unchanged skill must not vouch for an instructions file the tool
// inlines rules into.
func TestImport_BringsBackARuleEditInAnInlinedInstructionsFile(t *testing.T) {
	for target, entry := range map[string]string{
		"gemini":   "GEMINI.md",
		"opencode": "AGENTS.md",
		"aider":    "CONVENTIONS.md",
		"junie":    ".junie/AGENTS.md",
	} {
		t.Run(target, func(t *testing.T) {
			testutil.Chdir(t, t.TempDir())
			silence(t)
			mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: ["+target+"]\n")
			mustWriteFile(t, ".agnostic-ai/rules/style.md", "---\ndescription: Style.\n---\nRule text one.\n")
			mustWriteFile(t, ".agnostic-ai/skills/demo/SKILL.md", "---\nname: demo\ndescription: Demo. Use when testing.\n---\nSkill body.\n")
			if out, err := runCLI(t, "sync"); err != nil {
				t.Fatalf("sync: %v\n%s", err, out)
			}
			mustWriteFile(t, entry, strings.Replace(readFile(t, entry), "Rule text one.", "Rule text two.", 1))

			if out, err := runCLI(t, "import", target); err != nil {
				t.Fatalf("import %s: %v\n%s", target, err, out)
			}

			if got := readFile(t, ".agnostic-ai/rules/style.md"); !strings.Contains(got, "Rule text two.") {
				t.Errorf("import %s lost the edit:\n%s", target, got)
			}
		})
	}
}

// A rename in the tool's file moves the spec's output away from the path
// the last sync wrote; the edit there still counts.
func TestImport_BringsBackARenameInAToolFile(t *testing.T) {
	for kind, files := range map[string][2]string{
		"agent": {".agnostic-ai/agents/%s.md", ".claude/agents/demo.md"},
		"skill": {".agnostic-ai/skills/%s/SKILL.md", ".claude/skills/demo/SKILL.md"},
	} {
		t.Run(kind, func(t *testing.T) {
			testutil.Chdir(t, t.TempDir())
			silence(t)
			mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
			for _, name := range []string{"demo", "other"} {
				mustWriteFile(t, fmt.Sprintf(files[0], name), "---\nname: "+name+"\ndescription: A "+name+". Use when testing.\n---\nBody of "+name+".\n")
			}
			if out, err := runCLI(t, "sync"); err != nil {
				t.Fatalf("sync: %v\n%s", err, out)
			}
			native := strings.Replace(readFile(t, files[1]), "name: demo", "name: renamed", 1)
			mustWriteFile(t, files[1], strings.Replace(native, "Body of demo.", "Edited body.", 1))

			if out, err := runCLI(t, "import", "claude"); err != nil {
				t.Fatalf("import claude: %v\n%s", err, out)
			}

			got := readFile(t, fmt.Sprintf(files[0], "demo"))
			for _, want := range []string{"name: renamed", "Edited body."} {
				if !strings.Contains(got, want) {
					t.Errorf("import lost %q:\n%s", want, got)
				}
			}
		})
	}
}

// Comments on keys the edit keeps go back into the spec, inline ones
// included, so a body edit comes back without a stop.
func TestImport_KeepsFrontmatterCommentsWhenBringingBackAnEdit(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	silence(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	const agent = "---\nname: reviewer\n# Picked by hand.\ndescription: Review the diff. # read-only on purpose\nmodel: sonnet\n---\nReview what changed.\n"
	mustWriteFile(t, ".agnostic-ai/agents/reviewer.md", agent)
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	native := ".claude/agents/reviewer.md"
	mustWriteFile(t, native, strings.Replace(readFile(t, native), "Review what changed.", "Review every change.", 1))

	if out, err := runCLI(t, "import", "claude"); err != nil {
		t.Fatalf("import claude: %v\n%s", err, out)
	}

	got := readFile(t, ".agnostic-ai/agents/reviewer.md")
	for _, want := range []string{"Review every change.", "# Picked by hand.", "# read-only on purpose"} {
		if !strings.Contains(got, want) {
			t.Errorf("agent = %q, want %q", got, want)
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
