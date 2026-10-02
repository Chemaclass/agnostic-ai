package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/errs"
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

// A dry-run fails as the real run would, instead of counting the files
// it would write.
func TestImport_DryRunStopsOnAnExistingSpecItWouldReplace(t *testing.T) {
	importOverwriteProject(t)
	var err error

	out := captureStdout(t, func() {
		_, err = runCLI(t, "import", "claude", "--dry-run")
	})

	if errs.CodeOf(err) != errs.CodeImportWouldReplace || !strings.Contains(err.Error(), ".agnostic-ai/skills/review/SKILL.md (from claude)") {
		t.Fatalf("import --dry-run did not stop on the replaced spec: %v", err)
	}
	if strings.Contains(out, "would be written") {
		t.Errorf("dry-run counted files as written while the import would stop:\n%s", out)
	}
	if got := readFile(t, ".agnostic-ai/skills/review/SKILL.md"); got != handSkill {
		t.Errorf("dry-run changed the review skill: %q", got)
	}
}

func TestImport_DiffStopsOnAnExistingSpecItWouldReplace(t *testing.T) {
	importOverwriteProject(t)
	var err error

	captureStdout(t, func() {
		_, err = runCLI(t, "import", "claude", "--dry-run", "--diff")
	})

	if errs.CodeOf(err) != errs.CodeImportWouldReplace {
		t.Fatalf("import --dry-run --diff did not stop on the replaced spec: %v", err)
	}
}

func TestImport_DryRunWithOverwriteListsTheReplacedSpec(t *testing.T) {
	importOverwriteProject(t)

	out := captureStdout(t, func() {
		if _, err := runCLI(t, "import", "claude", "--dry-run", "--overwrite"); err != nil {
			t.Errorf("import --dry-run --overwrite: %v", err)
		}
	})

	if !strings.Contains(out, "replaces .agnostic-ai/skills/review/SKILL.md") {
		t.Errorf("dry-run did not name the replaced spec:\n%s", out)
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

	if err == nil || !strings.Contains(err.Error(), ".agnostic-ai/rules/style.md (from cursor; now holds what sync wrote for claude)") {
		t.Fatalf("import did not stop on a spec cursor never received: %v", err)
	}
	if got := readFile(t, ".agnostic-ai/rules/style.md"); got != mine {
		t.Errorf("style rule = %q, want it untouched", got)
	}
}

// Codex was the only target at the last sync, so the skill never reached
// claude: claude's own skill of the same name would replace it unseen.
func TestUse_StopsOnASyncedSpecTheNewToolNeverReceived(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	silence(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [codex]\n")
	mustWriteFile(t, ".agnostic-ai/skills/review/SKILL.md", handSkill)
	runSyncOK(t)
	mustWriteFile(t, ".claude/skills/review/SKILL.md", nativeSkill)

	_, err := runCLI(t, "use", "claude")

	if errs.CodeOf(err) != errs.CodeImportWouldReplace || !strings.Contains(err.Error(), "agnostic-ai import claude --overwrite") {
		t.Fatalf("use did not stop on a spec claude never received: %v", err)
	}
	if got := readFile(t, ".agnostic-ai/skills/review/SKILL.md"); got != handSkill {
		t.Errorf("review skill = %q, want it untouched", got)
	}
}

func TestImport_StopsOnASpecScopedToAnotherTool(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	silence(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, codex]\n")
	mine := "---\nname: review\ndescription: Mine.\ntargets: [codex]\n---\nMine\n"
	mustWriteFile(t, ".agnostic-ai/skills/review/SKILL.md", mine)
	runSyncOK(t)
	mustWriteFile(t, ".claude/skills/review/SKILL.md", nativeSkill)

	_, err := runCLI(t, "import", "claude")

	if errs.CodeOf(err) != errs.CodeImportWouldReplace {
		t.Fatalf("import claude did not stop on a skill scoped to codex: %v", err)
	}
	if got := readFile(t, ".agnostic-ai/skills/review/SKILL.md"); got != mine {
		t.Errorf("review skill = %q, want it untouched", got)
	}
}

// A comment is an edit: the spec no longer holds the bytes sync rendered.
func TestImport_StopsOnACommentAddedSinceTheLastSync(t *testing.T) {
	claudeSyncedReviewSkill(t)
	edited := strings.Replace(handSkill, "description: Mine.\n", "description: Mine.\n# keep the review short\n", 1)
	mustWriteFile(t, ".agnostic-ai/skills/review/SKILL.md", edited)

	_, err := runCLI(t, "import", "claude")

	if errs.CodeOf(err) != errs.CodeImportWouldReplace {
		t.Fatalf("import did not stop on a spec with a new comment: %v", err)
	}
	if got := readFile(t, ".agnostic-ai/skills/review/SKILL.md"); got != edited {
		t.Errorf("review skill = %q, want the comment kept", got)
	}
}

// mcpImportedFromClaude imports claude's fs server, next to codex's own
// fs server with a different command.
func mcpImportedFromClaude(t *testing.T) {
	t.Helper()
	testutil.Chdir(t, t.TempDir())
	silence(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, codex]\n")
	mustWriteFile(t, ".mcp.json", `{"mcpServers":{"fs":{"command":"npx","args":["fs-mcp"]}}}`)
	mustWriteFile(t, ".codex/config.toml", "[mcp_servers.fs]\ncommand = \"uvx\"\nargs = [\"fs-mcp\"]\n")
	if out, err := runCLI(t, "import", "claude"); err != nil {
		t.Fatalf("import claude: %v\n%s", err, out)
	}
}

// What an import of claude wrote, and nothing changed since, is claude's
// own config: a later claude import brings back an edit made there.
func TestImport_ReplacesWhatAnEarlierImportOfTheSameToolWrote(t *testing.T) {
	mcpImportedFromClaude(t)
	mustWriteFile(t, ".mcp.json", `{"mcpServers":{"fs":{"command":"bunx","args":["fs-mcp"]}}}`)

	if out, err := runCLI(t, "import", "claude"); err != nil {
		t.Fatalf("re-import of claude: %v\n%s", err, out)
	}
	if got := readFile(t, ".agnostic-ai/mcps/fs.yaml"); !strings.Contains(got, "bunx") {
		t.Errorf("fs.yaml = %q, want claude's edited server", got)
	}
}

// Codex never held claude's server: replacing it would make the next
// sync write codex's server into claude's .mcp.json.
func TestImport_StopsOnWhatAnotherToolsImportWrote(t *testing.T) {
	mcpImportedFromClaude(t)

	_, err := runCLI(t, "import", "codex")

	if errs.CodeOf(err) != errs.CodeImportWouldReplace ||
		!strings.Contains(err.Error(), ".agnostic-ai/mcps/fs.yaml (from codex; now holds what import claude wrote)") {
		t.Fatalf("import codex did not stop on claude's server: %v", err)
	}
	if got := readFile(t, ".agnostic-ai/mcps/fs.yaml"); !strings.Contains(got, "npx") {
		t.Errorf("fs.yaml = %q, want claude's server kept", got)
	}
}

func TestImport_ReimportsAChangedSettingsOverlay(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	silence(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	mustWriteFile(t, ".claude/settings.json", `{"statusLine": {"type": "command", "command": "first"}}`+"\n")
	if out, err := runCLI(t, "import", "claude"); err != nil {
		t.Fatalf("first import: %v\n%s", err, out)
	}
	mustWriteFile(t, ".claude/settings.json", `{"statusLine": {"type": "command", "command": "second"}}`+"\n")

	if out, err := runCLI(t, "import", "claude"); err != nil {
		t.Fatalf("re-import after a settings change: %v\n%s", err, out)
	}
	if got := readFile(t, ".agnostic-ai/overlays/claude.settings.json"); !strings.Contains(got, "second") {
		t.Errorf("overlay = %q, want the new statusLine", got)
	}
}

// The guard restores what a stopped run wrote instead of running the
// import in a copy first, so a real import never copies the project: it
// works with no usable temp directory.
func TestImport_RealRunDoesNotCopyTheProject(t *testing.T) {
	importOverwriteProject(t)
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "missing"))

	_, err := runCLI(t, "import", "claude")

	if errs.CodeOf(err) != errs.CodeImportWouldReplace {
		t.Fatalf("import without a temp dir: %v", err)
	}
	if _, err := os.Stat(".agnostic-ai/skills/deploy"); !os.IsNotExist(err) {
		t.Errorf("the stopped import left the deploy skill folder: %v", err)
	}
	if out, err := runCLI(t, "import", "claude", "--overwrite"); err != nil {
		t.Fatalf("import --overwrite without a temp dir: %v\n%s", err, out)
	}
}

// The singular `target:` key scopes a spec as `targets:` does.
func TestImport_StopsOnASpecWithATargetKeyForAnotherTool(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	silence(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, codex]\n")
	mine := "---\nname: review\ndescription: Mine.\ntarget: codex\n---\nMine\n"
	mustWriteFile(t, ".agnostic-ai/skills/review/SKILL.md", mine)
	runSyncOK(t)
	mustWriteFile(t, ".claude/skills/review/SKILL.md", nativeSkill)

	_, err := runCLI(t, "import", "claude")

	if errs.CodeOf(err) != errs.CodeImportWouldReplace {
		t.Fatalf("import claude did not stop on a skill with target: codex: %v", err)
	}
	if got := readFile(t, ".agnostic-ai/skills/review/SKILL.md"); got != mine {
		t.Errorf("review skill = %q, want it untouched", got)
	}
}

// A skill an earlier import wrote comes back from a later native edit.
func TestImport_ReimportsASkillAnEarlierImportWrote(t *testing.T) {
	importOverwriteProject(t)
	mustWriteFile(t, ".agnostic-ai/skills/review/SKILL.md", nativeSkill)
	if out, err := runCLI(t, "import", "claude"); err != nil {
		t.Fatalf("first import: %v\n%s", err, out)
	}
	edited := strings.Replace(nativeSkill, "Native\n", "Native, edited\n", 1)
	mustWriteFile(t, ".claude/skills/deploy/SKILL.md", "---\nname: deploy\ndescription: Ship.\n---\nShip, edited\n")
	mustWriteFile(t, ".claude/skills/review/SKILL.md", edited)

	if out, err := runCLI(t, "import", "claude"); err != nil {
		t.Fatalf("re-import of an imported skill: %v\n%s", err, out)
	}
	if got := readFile(t, ".agnostic-ai/skills/deploy/SKILL.md"); !strings.Contains(got, "Ship, edited") {
		t.Errorf("deploy skill = %q, want the native edit", got)
	}
}

// Two tools' own rules of one name: the second import may not replace
// what the first one wrote.
func TestImport_StopsOnARuleAnotherToolsImportWrote(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	silence(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, cursor]\n")
	mustWriteFile(t, ".claude/rules/style.md", "---\ndescription: Claude style.\n---\nUse tabs.\n")
	mustWriteFile(t, ".cursor/rules/style.mdc", "---\ndescription: Cursor style.\nalwaysApply: true\n---\nUse spaces.\n")
	if out, err := runCLI(t, "import", "claude"); err != nil {
		t.Fatalf("import claude: %v\n%s", err, out)
	}
	claudeRule := readFile(t, ".agnostic-ai/rules/style.md")

	_, err := runCLI(t, "import", "cursor")

	if errs.CodeOf(err) != errs.CodeImportWouldReplace ||
		!strings.Contains(err.Error(), ".agnostic-ai/rules/style.md (from cursor; now holds what import claude wrote)") {
		t.Fatalf("import cursor did not stop on claude's rule: %v", err)
	}
	if got := readFile(t, ".agnostic-ai/rules/style.md"); got != claudeRule {
		t.Errorf("style rule = %q, want claude's kept", got)
	}
}

// An import that finds a synced spec already in its tool's files records
// that tool too, so the tool's next edit comes back without a flag.
func TestImport_RecordsAToolThatAlreadyHeldASyncedSpec(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	silence(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [codex]\n")
	mustWriteFile(t, ".agnostic-ai/skills/review/SKILL.md", handSkill)
	runSyncOK(t)
	mustWriteFile(t, ".claude/skills/review/SKILL.md", handSkill)
	if out, err := runCLI(t, "import", "claude"); err != nil {
		t.Fatalf("import of an identical skill: %v\n%s", err, out)
	}
	mustWriteFile(t, ".claude/skills/review/SKILL.md", nativeSkill)

	if out, err := runCLI(t, "import", "claude"); err != nil {
		t.Fatalf("re-import after a claude edit: %v\n%s", err, out)
	}
	if got := readFile(t, ".agnostic-ai/skills/review/SKILL.md"); !strings.Contains(got, "Native") {
		t.Errorf("review skill = %q, want claude's edit", got)
	}
}

// import writes the state file to keep its records, but no sync ran: the
// first-sync picker and why's "run sync first" still apply.
func TestImport_StateFileItWritesDoesNotCountAsASync(t *testing.T) {
	importOverwriteProject(t)
	if out, err := runCLI(t, "import", "claude", "--overwrite"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}
	if _, err := os.Stat(stateFilePath(".")); err != nil {
		t.Fatalf("import kept no record: %v", err)
	}

	if !shouldPromptTargetSelection(".", &config.Config{Targets: allTargetNames()}) {
		t.Error("the first-sync picker is skipped after an import with no sync")
	}
	if err := whyNotTrackedError("CLAUDE.md", "."); !strings.Contains(err.Error(), "Run `agnostic-ai sync` first") {
		t.Errorf("why does not say to sync first after an import: %v", err)
	}
}

const corruptState = "{not json\n"

// An unreadable ledger still counts as one (#1334), so import keeps it
// instead of writing a stub over it.
func TestImport_KeepsAStateFileThatDoesNotParse(t *testing.T) {
	importOverwriteProject(t)
	mustWriteFile(t, stateFilePath("."), corruptState)

	if out, err := runCLI(t, "import", "claude", "--overwrite"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}

	if got := readFile(t, stateFilePath(".")); got != corruptState {
		t.Errorf("state file = %q, want it left as it was", got)
	}
	if ledgerMissing(".") {
		t.Error("the unreadable ledger no longer counts as one")
	}
}

func TestUse_RefusesToWriteOverAStateFileThatDoesNotParse(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	silence(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	mustWriteFile(t, ".cursor/rules/style.mdc", "---\ndescription: Cursor style.\nalwaysApply: true\n---\nUse spaces.\n")
	mustWriteFile(t, stateFilePath("."), corruptState)

	_, err := runCLI(t, "use", "cursor")

	if err == nil || !strings.Contains(err.Error(), "fix or delete it, then run agnostic-ai use again") {
		t.Fatalf("use did not refuse the unreadable state file: %v", err)
	}
	if got := readFile(t, stateFilePath(".")); got != corruptState {
		t.Errorf("state file = %q, want it left as it was", got)
	}
}

// The signal handler exits without running defers, so it gives the
// streams back and removes the held files through releaseHeldOutput.
func TestWithHeldOutput_ReleaseRestoresStderrAndRemovesHeldFiles(t *testing.T) {
	stderr := os.Stderr
	var held []string
	_ = withHeldOutput(func() (bool, error) {
		held = []string{os.Stdout.Name(), os.Stderr.Name()}
		releaseHeldOutput()
		return false, nil
	})

	if os.Stderr != stderr {
		t.Error("stderr still points at the held file")
	}
	for _, p := range held {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("held file %s is still there: %v", p, err)
		}
	}
}

// A source directory configured as an absolute path is guarded like a
// relative one: the directory and a write into it compare as one key.
func TestReplacesSpec_GuardsAnAbsoluteSourceDir(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	skills := filepath.Join(dir, "specs", "skills")
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\nsources:\n  skills: "+filepath.ToSlash(skills)+"\n")
	spec := filepath.Join(skills, "review", "SKILL.md")
	mustWriteFile(t, spec, handSkill)

	for _, path := range []string{spec, "specs/skills/review/SKILL.md"} {
		e := importPreviewEntry{
			path: filepath.ToSlash(path), existed: true, replaced: true,
			before: []byte(handSkill), after: []byte(nativeSkill), sources: []string{"claude"},
		}
		if !replacesSpec(&e, importSpecDirs("."), nil) {
			t.Errorf("%s: a write into the absolute source dir does not count as replacing a spec", path)
		}
	}
}

// A skill folder linked out of the source still holds a spec: a write
// through the link counts as replacing it.
func TestImport_StopsBeforeReplacingASkillThroughALinkedFolder(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	testutil.Chdir(t, t.TempDir())
	silence(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	mustWriteFile(t, "shared/review/SKILL.md", handSkill)
	if err := os.MkdirAll(".agnostic-ai/skills", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../../shared/review", ".agnostic-ai/skills/review"); err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, ".claude/skills/review/SKILL.md", nativeSkill)

	_, err := runCLI(t, "import", "claude")

	if errs.CodeOf(err) != errs.CodeImportWouldReplace {
		t.Fatalf("import did not stop on a skill behind a linked folder: %v", err)
	}
	if got := readFile(t, "shared/review/SKILL.md"); got != handSkill {
		t.Errorf("shared skill = %q, want it untouched", got)
	}
}

// A sync of one target keeps what an earlier sync wrote for the others,
// so cursor's own edit to a cursor-only rule still comes back. Codex
// would merge a skill rather than replace it, so cursor shows it.
func TestImport_PartialSyncKeepsWhatAnEarlierSyncWroteForOtherTools(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	silence(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, cursor]\n")
	mustWriteFile(t, ".agnostic-ai/rules/review.md", "---\nname: review\ndescription: Mine.\ntargets: [cursor]\n---\nMine\n")
	runSyncOK(t)
	if out, err := runCLI(t, "sync", "--only", "claude"); err != nil {
		t.Fatalf("sync --only claude: %v\n%s", err, out)
	}
	native := ".cursor/rules/review.mdc"
	mustWriteFile(t, native, strings.Replace(readFile(t, native), "Mine", "Mine, edited in cursor", 1))

	if out, err := runCLI(t, "import", "cursor"); err != nil {
		t.Fatalf("import cursor after a partial sync: %v\n%s", err, out)
	}
	if got := readFile(t, ".agnostic-ai/rules/review.md"); !strings.Contains(got, "edited in cursor") {
		t.Errorf("review rule = %q, want cursor's edit", got)
	}
}

// A target taken out of the config loses what sync wrote for it, so its
// own rule of the same name, written later, may not replace the spec.
func TestUse_StopsOnASpecSyncedForAToolSinceRemoved(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	silence(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, cursor]\n")
	mine := "---\nname: r\ndescription: Mine.\n---\nMine\n"
	mustWriteFile(t, ".agnostic-ai/rules/r.md", mine)
	runSyncOK(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	runSyncOK(t)
	if err := os.RemoveAll(".cursor"); err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, ".cursor/rules/r.mdc", "---\ndescription: Unrelated.\nalwaysApply: true\n---\nSomething else.\n")

	_, err := runCLI(t, "use", "cursor")

	if errs.CodeOf(err) != errs.CodeImportWouldReplace {
		t.Fatalf("use cursor did not stop on a rule cursor no longer held: %v", err)
	}
	if got := readFile(t, ".agnostic-ai/rules/r.md"); got != mine {
		t.Errorf("rule = %q, want it untouched", got)
	}
}
