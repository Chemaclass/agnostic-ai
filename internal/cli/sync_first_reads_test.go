package cli

import (
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// The first sync shows what each tool now reads; later ones do not (#1614).
func TestSync_FirstRunShowsWhatEachToolReads(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	isolateGit(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, codex]\n")
	mustWriteFile(t, ".agnostic-ai/skills/review/SKILL.md", "---\nname: review\ndescription: Review a PR.\n---\nReview.\n")
	log := captureLog(t)

	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	for _, want := range []string{"claude now reads", "codex now reads", ".agents/skills/", "review"} {
		if !strings.Contains(log.String(), want) {
			t.Errorf("first sync output is missing %q:\n%s", want, log.String())
		}
	}

	log.Reset()
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("second sync: %v\n%s", err, out)
	}
	if strings.Contains(log.String(), "now reads") {
		t.Errorf("a later sync repeated the summary:\n%s", log.String())
	}
}
