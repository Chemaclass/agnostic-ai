package cli

import (
	"os"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

const (
	handSkill   = "---\nname: review\ndescription: Mine.\n---\nMine\n"
	nativeSkill = "---\nname: review\ndescription: Native.\n---\nNative\n"
)

// importOverwriteProject is a claude project whose hand-written review
// skill shares its name with a native one, next to a native skill the
// import would create.
func importOverwriteProject(t *testing.T) {
	t.Helper()
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	mustWriteFile(t, ".agnostic-ai/skills/review/SKILL.md", handSkill)
	mustWriteFile(t, ".claude/skills/review/SKILL.md", nativeSkill)
	mustWriteFile(t, ".claude/skills/deploy/SKILL.md", "---\nname: deploy\ndescription: Ship.\n---\nShip\n")
}

// Import replaced a hand-written spec of the same name without a word
// (#1620).
func TestImport_StopsBeforeReplacingAHandWrittenSkill(t *testing.T) {
	importOverwriteProject(t)

	_, err := runCLI(t, "import", "claude")

	if err == nil {
		t.Fatal("import replaced a hand-written skill without stopping")
	}
	for _, want := range []string{".agnostic-ai/skills/review/SKILL.md (from claude)", "agnostic-ai import claude --overwrite"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error lacks %q:\n%v", want, err)
		}
	}
	if got := readFile(t, ".agnostic-ai/skills/review/SKILL.md"); got != handSkill {
		t.Errorf("review skill = %q, want it untouched", got)
	}
	if _, err := os.Stat(".agnostic-ai/skills/deploy/SKILL.md"); err == nil {
		t.Error("import wrote the deploy skill before stopping")
	}
}

func TestImport_StopsBeforeReplacingAnMCPServer(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	mustWriteFile(t, ".mcp.json", `{"mcpServers":{"github":{"command":"npx","args":["github-mcp"]}}}`)
	if out, err := runCLI(t, "import", "claude"); err != nil {
		t.Fatalf("first import: %v\n%s", err, out)
	}
	mine := "command: docker\nargs: [run, github-mcp]\n"
	mustWriteFile(t, ".agnostic-ai/mcps/github.yaml", mine)

	_, err := runCLI(t, "import", "claude")

	if err == nil || !strings.Contains(err.Error(), ".agnostic-ai/mcps/github.yaml (from claude)") {
		t.Fatalf("import did not stop on the edited MCP server: %v", err)
	}
	if got := readFile(t, ".agnostic-ai/mcps/github.yaml"); got != mine {
		t.Errorf("github.yaml = %q, want it untouched", got)
	}
}

func TestImport_LeavesAnIdenticalSpecAlone(t *testing.T) {
	importOverwriteProject(t)
	mustWriteFile(t, ".agnostic-ai/skills/review/SKILL.md", nativeSkill)

	if out, err := runCLI(t, "import", "claude"); err != nil {
		t.Fatalf("import stopped on a spec it would not change: %v\n%s", err, out)
	}
	if out, err := runCLI(t, "import", "claude"); err != nil {
		t.Fatalf("a second import stopped: %v\n%s", err, out)
	}
	if got := readFile(t, ".agnostic-ai/skills/review/SKILL.md"); got != nativeSkill {
		t.Errorf("review skill = %q, want %q", got, nativeSkill)
	}
}

func TestImport_OverwriteReplacesTheSpec(t *testing.T) {
	importOverwriteProject(t)

	if out, err := runCLI(t, "import", "claude", "--overwrite"); err != nil {
		t.Fatalf("import --overwrite: %v\n%s", err, out)
	}

	if got := readFile(t, ".agnostic-ai/skills/review/SKILL.md"); !strings.Contains(got, "Native") {
		t.Errorf("review skill = %q, want the native one", got)
	}
}

// Only files there before the run count: a spec one source writes and a
// later source replaces is today's last-wins import.
func TestImport_SourcesInOneRunStillReplaceEachOther(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, cursor]\n")
	mustWriteFile(t, ".agnostic-ai/skills/deploy/SKILL.md", "---\nname: deploy\ndescription: Ship.\n---\nShip\n")
	mustWriteFile(t, ".claude/rules/style.md", "---\ndescription: Claude style.\n---\nUse tabs.\n")
	mustWriteFile(t, ".cursor/rules/style.mdc", "---\ndescription: Cursor style.\nalwaysApply: true\n---\nUse spaces.\n")

	if out, err := runCLI(t, "import", "claude", "cursor"); err != nil {
		t.Fatalf("import claude cursor: %v\n%s", err, out)
	}

	if got := readFile(t, ".agnostic-ai/rules/style.md"); !strings.Contains(got, "Use spaces.") {
		t.Errorf("style rule = %q, want cursor's, imported last", got)
	}
}

func TestImport_DryRunReportsAnExistingSpecItWouldReplace(t *testing.T) {
	importOverwriteProject(t)

	out := captureStdout(t, func() {
		if _, err := runCLI(t, "import", "claude", "--dry-run"); err != nil {
			t.Errorf("import --dry-run: %v", err)
		}
	})

	if !strings.Contains(out, ".agnostic-ai/skills/review/SKILL.md already holds different content (claude); the import stops unless --overwrite") {
		t.Errorf("dry-run did not report the replaced spec:\n%s", out)
	}
	if strings.Contains(out, "skills/deploy/SKILL.md already holds") {
		t.Errorf("dry-run reported a new spec as replaced:\n%s", out)
	}
	if got := readFile(t, ".agnostic-ai/skills/review/SKILL.md"); got != handSkill {
		t.Errorf("dry-run changed the review skill: %q", got)
	}
}

func TestImport_DiffReportsAnExistingSpecItWouldReplace(t *testing.T) {
	importOverwriteProject(t)

	out := captureStdout(t, func() {
		if _, err := runCLI(t, "import", "claude", "--dry-run", "--diff"); err != nil {
			t.Errorf("import --dry-run --diff: %v", err)
		}
	})

	if !strings.Contains(out, ".agnostic-ai/skills/review/SKILL.md already holds different content (claude)") {
		t.Errorf("diff did not report the replaced spec:\n%s", out)
	}
}

func TestInitFrom_StopsBeforeReplacingAHandWrittenSkill(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, ".agnostic-ai/skills/review/SKILL.md", handSkill)
	mustWriteFile(t, ".claude/skills/review/SKILL.md", nativeSkill)

	_, err := runCLI(t, "init", "--from", "claude")

	if err == nil || !strings.Contains(err.Error(), "agnostic-ai import claude --overwrite") {
		t.Fatalf("init --from did not stop with the overwrite remedy: %v", err)
	}
	if got := readFile(t, ".agnostic-ai/skills/review/SKILL.md"); got != handSkill {
		t.Errorf("review skill = %q, want it untouched", got)
	}
}

func TestUse_StopsBeforeReplacingAHandWrittenRule(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	runSyncOK(t)
	mine := "---\ndescription: Mine.\n---\nUse tabs.\n"
	mustWriteFile(t, ".agnostic-ai/rules/style.md", mine)
	mustWriteFile(t, ".cursor/rules/style.mdc", "---\ndescription: Cursor style.\nalwaysApply: true\n---\nUse spaces.\n")

	_, err := runCLI(t, "use", "cursor")

	if err == nil {
		t.Fatal("use replaced a hand-written rule without stopping")
	}
	for _, want := range []string{".agnostic-ai/rules/style.md (from cursor)", "agnostic-ai import cursor --overwrite"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error lacks %q:\n%v", want, err)
		}
	}
	if got := readFile(t, ".agnostic-ai/rules/style.md"); got != mine {
		t.Errorf("style rule = %q, want it untouched", got)
	}
	cfg, lerr := config.Load(".")
	if lerr != nil {
		t.Fatal(lerr)
	}
	for _, target := range cfg.Targets {
		if target == "cursor" {
			t.Errorf("use left cursor in targets after stopping: %v", cfg.Targets)
		}
	}
}

// Copilot and kiro write an agent's tools in another shape, so a sync
// and import with no edit in between change the spec bytes. The spec
// still matches what the last sync rendered, so nothing stops.
func TestImport_UneditedRoundTripNeedsNoFlag(t *testing.T) {
	const agent = "---\nname: reviewer\ndescription: Review the diff.\ntools: [Read, Grep]\neffort: high\n---\n\nReview what changed.\n"
	for _, target := range []string{"copilot", "kiro"} {
		t.Run(target, func(t *testing.T) {
			testutil.Chdir(t, t.TempDir())
			silence(t)
			mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: ["+target+"]\n")
			mustWriteFile(t, ".agnostic-ai/agents/reviewer.md", agent)
			runSyncOK(t)

			if out, err := runCLI(t, "import", target); err != nil {
				t.Fatalf("import %s after sync: %v\n%s", target, err, out)
			}
		})
	}
}

// claudeSyncedReviewSkill syncs a review skill to claude, then edits the
// native copy, the documented way to bring a native edit back.
func claudeSyncedReviewSkill(t *testing.T) {
	t.Helper()
	testutil.Chdir(t, t.TempDir())
	silence(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	mustWriteFile(t, ".agnostic-ai/skills/review/SKILL.md", handSkill)
	runSyncOK(t)
	mustWriteFile(t, ".claude/skills/review/SKILL.md", nativeSkill)
}

func TestImport_ReimportsANativeEditWithoutAFlag(t *testing.T) {
	claudeSyncedReviewSkill(t)

	if out, err := runCLI(t, "import", "claude"); err != nil {
		t.Fatalf("import after a native edit: %v\n%s", err, out)
	}

	if got := readFile(t, ".agnostic-ai/skills/review/SKILL.md"); !strings.Contains(got, "Native") {
		t.Errorf("review skill = %q, want the native edit", got)
	}
}

func TestImport_StopsWhenTheSpecChangedSinceTheLastSync(t *testing.T) {
	claudeSyncedReviewSkill(t)
	edited := strings.Replace(handSkill, "Mine\n", "Mine, edited\n", 1)
	mustWriteFile(t, ".agnostic-ai/skills/review/SKILL.md", edited)

	_, err := runCLI(t, "import", "claude")

	if err == nil || !strings.Contains(err.Error(), ".agnostic-ai/skills/review/SKILL.md (from claude)") {
		t.Fatalf("import did not stop on a spec edited since the last sync: %v", err)
	}
	if got := readFile(t, ".agnostic-ai/skills/review/SKILL.md"); got != edited {
		t.Errorf("review skill = %q, want the edit kept", got)
	}
}

// A synced spec is exempt only for a tool sync wrote it for: a rule
// scoped to claude never reached cursor, so cursor's own rule of the
// same name would replace it unseen.
func TestImport_StopsOnASyncedSpecTheSourceNeverReceived(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	silence(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, cursor]\n")
	mine := "---\nname: style\ndescription: Mine.\ntargets: [claude]\n---\nUse tabs.\n"
	mustWriteFile(t, ".agnostic-ai/rules/style.md", mine)
	runSyncOK(t)
	mustWriteFile(t, ".cursor/rules/style.mdc", "---\ndescription: Cursor style.\nalwaysApply: true\n---\nUse spaces.\n")

	_, err := runCLI(t, "import", "cursor")

	if err == nil || !strings.Contains(err.Error(), ".agnostic-ai/rules/style.md (from cursor)") {
		t.Fatalf("import did not stop on a spec cursor never received: %v", err)
	}
	if got := readFile(t, ".agnostic-ai/rules/style.md"); got != mine {
		t.Errorf("style rule = %q, want it untouched", got)
	}
}
