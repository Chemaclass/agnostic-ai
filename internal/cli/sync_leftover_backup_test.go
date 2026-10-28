package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/gitquery"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestSync_BackupOfAnOutputNoLongerGeneratedStaysIgnored(t *testing.T) {
	dir, _ := gitRepo(t)
	testutil.Chdir(t, dir)
	silence(t)
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\ngitignore:\n  enabled: true\n")
	mustWriteFile(t, filepath.Join(dir, "CLAUDE.md"), handWrittenClaude)
	runSyncOK(t, "--backup")

	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [cursor]\ngitignore:\n  enabled: true\n")
	runSyncOK(t)

	if !gitIgnored(t, dir, "CLAUDE.md.bak") {
		t.Error("CLAUDE.md.bak is not ignored once claude is no longer a target")
	}
	if ignore := readFile(t, filepath.Join(dir, ".gitignore")); strings.Contains(ignore, "CLAUDE.md.bak") {
		t.Errorf("the committed block lists a backup only this checkout made:\n%s", ignore)
	}
}

func TestBuiltinChanges_PersonalToggleKeepsALeftoverBackupIgnored(t *testing.T) {
	personalProject(t, "[]")
	root, _ := os.Getwd()
	git(t, root, "init", "--quiet")
	gitquery.Reset()
	mustWriteFile(t, "CLAUDE.md", handWrittenClaude)
	runSyncOK(t, "--backup")
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [cursor]\nbuiltins: []\ngitignore:\n  enabled: true\n")
	runSyncOK(t)
	mustWriteFile(t, "agnostic-ai.local.yaml", "builtins: []\n")

	h := newBuiltinChangeHandler(".", false)
	proposal := builtinScopedProposal(t, h, audiencePersonal, "caveman", true)
	builtinAction(t, h, builtinApplyPath, proposal.ID)

	if !gitIgnored(t, root, "CLAUDE.md.bak") {
		exclude, _ := os.ReadFile(filepath.Join(".git", "info", "exclude"))
		t.Errorf("a built-in toggle dropped the leftover backup from info/exclude:\n%s", exclude)
	}
}
