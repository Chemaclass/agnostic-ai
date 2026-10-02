package cli

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func claudeOnlyProject(t *testing.T) {
	t.Helper()
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "CLAUDE.md", "# My project\n\n## Conventions\n\nUse pnpm. Never touch prod.\n")
	mustWriteFile(t, ".claude/skills/review/SKILL.md", "---\nname: review\ndescription: Review a PR.\n---\nReview carefully.\n")
}

// One command takes a Claude-only repository to Claude plus Codex from
// one source, keeping what it had (#1613).
func TestUse_StartsAProjectFromWhatTheRepositoryHas(t *testing.T) {
	claudeOnlyProject(t)
	log := captureLog(t)

	if out, err := runCLI(t, "use", "codex"); err != nil {
		t.Fatalf("use codex: %v\n%s", err, out)
	}
	if !strings.Contains(log.String(), "generated files are git-ignored") {
		t.Errorf("use without a terminal must say it ignored generated files:\n%s", log.String())
	}

	if got := readFile(t, "AGENTS.md"); !strings.Contains(got, "Never touch prod.") {
		t.Errorf("AGENTS.md lacks the project's instructions:\n%s", got)
	}
	if _, err := os.Stat(".agents/skills/review/SKILL.md"); err != nil {
		t.Errorf("codex did not get the skill: %v", err)
	}
	if cfg := readFile(t, "agnostic-ai.yaml"); !strings.Contains(cfg, "claude") || !strings.Contains(cfg, "codex") {
		t.Errorf("targets miss claude or codex:\n%s", cfg)
	}
	for _, want := range []string{"codex now reads, from .agnostic-ai/:", "1 skill", "review"} {
		if !strings.Contains(log.String(), want) {
			t.Errorf("summary lacks %q:\n%s", want, log.String())
		}
	}
}

func TestUse_AgainChangesNothing(t *testing.T) {
	claudeOnlyProject(t)
	if out, err := runCLI(t, "use", "codex"); err != nil {
		t.Fatalf("use codex: %v\n%s", err, out)
	}
	before := snapshotFiles(t, "AGENTS.md", "CLAUDE.md", "agnostic-ai.yaml")
	log := captureLog(t)

	if out, err := runCLI(t, "use", "codex"); err != nil {
		t.Fatalf("use codex again: %v\n%s", err, out)
	}

	if after := snapshotFiles(t, "AGENTS.md", "CLAUDE.md", "agnostic-ai.yaml"); after["AGENTS.md"] != before["AGENTS.md"] || after["agnostic-ai.yaml"] != before["agnostic-ai.yaml"] {
		t.Error("a second use changed the project")
	}
	if !strings.Contains(log.String(), "codex already in use") {
		t.Errorf("second use did not say so:\n%s", log.String())
	}
}

// In an existing project, use adds the tool and imports its own config
// before the sync would write over it.
func TestUse_AddsAToolToAnExistingProjectAndKeepsItsConfig(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	mustWriteFile(t, ".agnostic-ai/AGNOSTIC_AI.md", "# My project\n\nUse pnpm.\n")
	runSyncOK(t)
	mustWriteFile(t, "AGENTS.md", "# Agents\n\n## Reviews\n\nKeep PRs small.\n")

	if out, err := runCLI(t, "use", "codex"); err != nil {
		t.Fatalf("use codex: %v\n%s", err, out)
	}

	got := readFile(t, "AGENTS.md")
	for _, want := range []string{"Use pnpm.", "Keep PRs small."} {
		if !strings.Contains(got, want) {
			t.Errorf("AGENTS.md lacks %q:\n%s", want, got)
		}
	}
}

func TestUse_RejectsAMistypedTool(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	_, err := runCLI(t, "use", "codx")
	if err == nil || !strings.Contains(err.Error(), "did you mean codex?") {
		t.Errorf("err = %v, want a did-you-mean", err)
	}
	if _, err := os.Stat("agnostic-ai.yaml"); err == nil {
		t.Error("use wrote a config for a mistyped tool")
	}
}

// A run that stopped before importing finishes on the next try, though
// the tool is already in targets.
func TestUse_ARetryImportsWhatAnEarlierRunLeft(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, codex]\n")
	mustWriteFile(t, "AGENTS.md", "# Agents\n\n## Reviews\n\nKeep PRs small.\n")

	if out, err := runCLI(t, "use", "codex"); err != nil {
		t.Fatalf("use codex: %v\n%s", err, out)
	}

	if got := readFile(t, ".agnostic-ai/AGNOSTIC_AI.md"); !strings.Contains(got, "Keep PRs small.") {
		t.Errorf("use did not import the left-over AGENTS.md:\n%s", got)
	}
}

// jules has no importer; its AGENTS.md is folded in as root instructions.
func TestUse_AToolWithoutAnImporterKeepsItsInstructions(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "AGENTS.md", "# Agents\n\n## Reviews\n\nKeep PRs small.\n")

	if out, err := runCLI(t, "use", "jules"); err != nil {
		t.Fatalf("use jules: %v\n%s", err, out)
	}

	if got := readFile(t, "AGENTS.md"); !strings.Contains(got, "Keep PRs small.") {
		t.Errorf("AGENTS.md lost its instructions:\n%s", got)
	}
}

// Text with no ## headings is one section, so it is kept too.
func TestUse_KeepsFlatInstructionsBesideAnExistingBody(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	mustWriteFile(t, ".agnostic-ai/AGNOSTIC_AI.md", "# P\n\nUse pnpm.\n")
	runSyncOK(t)
	mustWriteFile(t, "AGENTS.md", "Keep PRs small.\nNever touch prod.\n")

	if out, err := runCLI(t, "use", "jules"); err != nil {
		t.Logf("use jules: %v\n%s", err, out)
	}

	if got := readFile(t, "AGENTS.md"); !strings.Contains(got, "Never touch prod.") {
		t.Errorf("AGENTS.md lost its flat instructions:\n%s", got)
	}
}

// A Codex setup is often just AGENTS.md; switching to Claude keeps it.
func TestUse_KeepsAHandWrittenAgentsMdWhenSwitchingAway(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "AGENTS.md", "# Agents\n\n## Reviews\n\nKeep PRs small.\n")

	if out, err := runCLI(t, "use", "claude"); err != nil {
		t.Fatalf("use claude: %v\n%s", err, out)
	}

	if got := readFile(t, ".agnostic-ai/AGNOSTIC_AI.md"); !strings.Contains(got, "Keep PRs small.") {
		t.Errorf("AGNOSTIC_AI.md lacks the AGENTS.md text:\n%s", got)
	}
}

func TestUse_RefusesWhenTheLocalFileSetsTargets(t *testing.T) {
	for name, local := range map[string]string{
		"list": "targets: [claude, cursor]\n",
		"null": "targets: null\n",
	} {
		t.Run(name, func(t *testing.T) {
			testutil.Chdir(t, t.TempDir())
			mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
			mustWriteFile(t, "agnostic-ai.local.yaml", local)

			_, err := runCLI(t, "use", "codex")

			if err == nil || !strings.Contains(err.Error(), "agnostic-ai.local.yaml sets targets") {
				t.Errorf("err = %v, want the local file named", err)
			}
			if cfg := readFile(t, "agnostic-ai.yaml"); strings.Contains(cfg, "codex") || strings.Contains(cfg, "cursor") {
				t.Errorf("committed config changed:\n%s", cfg)
			}
		})
	}
}

func TestUse_RefusesToStartAProjectInsideAnother(t *testing.T) {
	root := t.TempDir()
	testutil.Chdir(t, root)
	isolateGit(t)
	gitInit(t, root)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	mustWriteFile(t, "web/README.md", "web\n")
	testutil.Chdir(t, "web")

	_, err := runCLI(t, "use", "codex")

	if err == nil || !strings.Contains(err.Error(), "inside the agnostic-ai project") {
		t.Errorf("err = %v, want a refusal", err)
	}
	if _, err := os.Stat("agnostic-ai.yaml"); err == nil {
		t.Error("use started a nested project")
	}
}

func TestUse_RefusesToStartAProjectInsideAnotherOutsideGit(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	isolateGit(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	mustWriteFile(t, "web/README.md", "web\n")
	testutil.Chdir(t, "web")

	_, err := runCLI(t, "use", "codex")

	if err == nil || !strings.Contains(err.Error(), "inside the agnostic-ai project") {
		t.Errorf("err = %v, want a refusal", err)
	}
	if _, err := os.Stat("agnostic-ai.yaml"); err == nil {
		t.Error("use started a nested project")
	}
}

func TestUse_LeavesAnUnmanagedInstructionsFileToItsTool(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	isolateGit(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\nsync:\n  unmanaged: [CLAUDE.md]\n")
	mustWriteFile(t, "CLAUDE.md", "# Mine\n\nSecret claude note.\n")

	if out, err := runCLI(t, "use", "codex"); err != nil {
		t.Fatalf("use codex: %v\n%s", err, out)
	}

	for _, f := range []string{".agnostic-ai/AGNOSTIC_AI.md", "AGENTS.md"} {
		if data, _ := os.ReadFile(f); strings.Contains(string(data), "Secret claude note.") {
			t.Errorf("%s took text from the unmanaged CLAUDE.md:\n%s", f, data)
		}
	}
	if data, _ := os.ReadFile("CLAUDE.md"); !strings.Contains(string(data), "Secret claude note.") {
		t.Errorf("CLAUDE.md lost its text:\n%s", data)
	}
}

// A rerun for a tool already in use leaves spec edits alone.
func TestUse_AgainKeepsSpecEdits(t *testing.T) {
	claudeOnlyProject(t)
	if out, err := runCLI(t, "use", "codex"); err != nil {
		t.Fatalf("use codex: %v\n%s", err, out)
	}
	spec := ".agnostic-ai/skills/review/SKILL.md"
	mustWriteFile(t, spec, strings.Replace(readFile(t, spec), "Review carefully.", "Review twice.", 1))

	if out, err := runCLI(t, "use", "claude"); err != nil {
		t.Fatalf("use claude: %v\n%s", err, out)
	}

	if got := readFile(t, spec); !strings.Contains(got, "Review twice.") {
		t.Errorf("use reimported .claude/ over the spec edit:\n%s", got)
	}
}

func TestUse_LeavesAnUnmanagedAgentsMdOutOfSharedInstructions(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	isolateGit(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\nsync:\n  unmanaged: [AGENTS.md]\n")
	mustWriteFile(t, "AGENTS.md", "# Mine\n\nSecret codex note.\n")

	if out, err := runCLI(t, "use", "gemini"); err != nil {
		t.Fatalf("use gemini: %v\n%s", err, out)
	}

	if data, _ := os.ReadFile(".agnostic-ai/AGNOSTIC_AI.md"); strings.Contains(string(data), "Secret codex note.") {
		t.Errorf("use folded the unmanaged AGENTS.md into shared instructions:\n%s", data)
	}
}

// A run stopped between adding claude and importing it leaves claude
// pending: sync waits, and the next use imports it.
func TestUse_FinishesAnImportAnInterruptedRunLeft(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	isolateGit(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [codex, claude]\n")
	mustWriteFile(t, ".agnostic-ai/.sync-state", `{"pending_imports":["claude"]}`)
	mustWriteFile(t, ".mcp.json", `{"mcpServers":{"srv":{"command":"srv"}}}`+"\n")
	mustWriteFile(t, ".claude/settings.json", "{}\n")

	if _, err := runCLI(t, "sync"); err == nil || !strings.Contains(err.Error(), "run agnostic-ai use claude before syncing") {
		t.Fatalf("sync err = %v, want it to wait for the pending import", err)
	}
	if out, err := runCLI(t, "use", "codex"); err != nil {
		t.Fatalf("use codex: %v\n%s", err, out)
	}

	if got := readFile(t, ".mcp.json"); !strings.Contains(got, "srv") {
		t.Errorf(".mcp.json lost its server:\n%s", got)
	}
	if m, _ := filepath.Glob(".agnostic-ai/mcps/srv*"); len(m) == 0 {
		t.Error("use did not import the pending claude MCP server")
	}
	if got := readStateFile(".").PendingImports; len(got) != 0 {
		t.Errorf("pending imports = %v, want none", got)
	}
}

func TestUse_SyncIgnoresAPendingToolNoLongerInTargets(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	isolateGit(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [codex]\n")
	mustWriteFile(t, ".agnostic-ai/.sync-state", `{"pending_imports":["cursor"]}`)

	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
}

// A tool with its own config and an unmanaged instructions file can be
// neither imported nor skipped safely, so use stops before writing.
func TestUse_RefusesAToolWhoseInstructionsAreUnmanaged(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	isolateGit(t)
	cfg := "version: 1\ntargets: [codex]\nsync:\n  unmanaged: [CLAUDE.md]\n"
	mustWriteFile(t, "agnostic-ai.yaml", cfg)
	mustWriteFile(t, "CLAUDE.md", "# Mine\n\nSecret claude note.\n")
	mustWriteFile(t, ".mcp.json", `{"mcpServers":{"srv":{"command":"srv"}}}`+"\n")
	mustWriteFile(t, ".claude/settings.json", "{}\n")

	_, err := runCLI(t, "use", "claude")

	if err == nil || !strings.Contains(err.Error(), "CLAUDE.md is in sync.unmanaged") {
		t.Fatalf("err = %v, want a refusal naming CLAUDE.md", err)
	}
	if got := readFile(t, "agnostic-ai.yaml"); got != cfg {
		t.Errorf("config changed:\n%s", got)
	}
	if got := readFile(t, ".mcp.json"); !strings.Contains(got, "srv") {
		t.Errorf(".mcp.json lost its server:\n%s", got)
	}
}

// A failed first import leaves the failed tool out of targets and keeps
// the rest, so a retry imports only what failed.
func TestUse_AFailedStartLeavesTheFailedToolOut(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	isolateGit(t)
	mustWriteFile(t, ".claude/settings.json", "{bad\n")

	_, err := runCLI(t, "use", "codex")

	if err == nil || !strings.Contains(err.Error(), "run agnostic-ai use claude") {
		t.Fatalf("err = %v, want the import failure", err)
	}
	if got := readFile(t, "agnostic-ai.yaml"); strings.Contains(got, "claude") || !strings.Contains(got, "codex") {
		t.Errorf("targets should keep codex and drop claude:\n%s", got)
	}
}

// When every tool fails, no config is left, so a retry starts again.
func TestUse_AFailedStartWithNothingImportedLeavesNoProject(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	isolateGit(t)
	mustWriteFile(t, ".claude/settings.json", "{bad\n")

	_, err := runCLI(t, "use", "claude")

	if err == nil || !strings.Contains(err.Error(), "run agnostic-ai use again") {
		t.Fatalf("err = %v, want the import failure", err)
	}
	if _, err := os.Stat("agnostic-ai.yaml"); err == nil {
		t.Error("a failed start left agnostic-ai.yaml, so a retry would skip its imports")
	}
}

// A failed import leaves the added tool out of targets, so a retry
// imports it again.
func TestUse_AFailedImportLeavesTheToolOut(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	isolateGit(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [codex]\n")
	mustWriteFile(t, ".claude/settings.json", "{bad\n")

	_, err := runCLI(t, "use", "claude")

	if err == nil || !strings.Contains(err.Error(), "left claude out of targets") {
		t.Fatalf("err = %v, want the import failure", err)
	}
	if got := readFile(t, "agnostic-ai.yaml"); strings.Contains(got, "claude") {
		t.Errorf("claude stayed in targets after a failed import:\n%s", got)
	}
}

// Tools whose adapter describes no file layout still list what they got.
func TestUse_SummaryListsSpecsForEveryTool(t *testing.T) {
	claudeOnlyProject(t)
	log := captureLog(t)

	if out, err := runCLI(t, "use", "cursor"); err != nil {
		t.Fatalf("use cursor: %v\n%s", err, out)
	}

	if !strings.Contains(log.String(), "1 skill") {
		t.Errorf("cursor summary lists nothing:\n%s", log.String())
	}
}

// A failed tool a rollback could not drop from targets stays pending.
func TestStillConfigured(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	if got := stillConfigured([]string{"claude"}); got != nil {
		t.Errorf("without a config = %v, want none", got)
	}
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [codex, claude]\n")
	if got := stillConfigured([]string{"claude", "cursor"}); !slices.Equal(got, []string{"claude"}) {
		t.Errorf("got %v, want [claude]", got)
	}
}
