package cli

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestSync_FailedGitignoreWriteRestoresSweptOrphan(t *testing.T) {
	for _, args := range [][]string{{"sync"}, {"sync", "--json"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			testutil.TempCwd(t)
			mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [codex]\ngitignore:\n  enabled: true\n")
			const orphan = ".codex/skills/gone/SKILL.md"
			const body = "---\nname: gone\n---\nold\n"
			mustWriteFile(t, orphan, body)
			if err := writeStateFile(".", 0, "", "", syncLedger{outputs: []string{orphan}, sums: map[string]string{orphan: adapters.ContentSum(body)}}); err != nil {
				t.Fatal(err)
			}
			// A directory fails the write on every OS, even for root.
			if err := os.Mkdir(".gitignore", 0o755); err != nil {
				t.Fatal(err)
			}

			if out, err := runCLI(t, args...); err == nil {
				t.Fatalf("sync succeeded with .gitignore a directory:\n%s", out)
			}

			if got, err := os.ReadFile(orphan); err != nil || string(got) != body {
				t.Fatalf("swept orphan not restored: %q, %v", got, err)
			}
			if state := readStateFile("."); !slices.Contains(state.Outputs, orphan) {
				t.Fatalf("ledger forgot the orphan: %v", state.Outputs)
			}
			if err := os.Remove(".gitignore"); err != nil {
				t.Fatal(err)
			}
			if out, err := runCLI(t, args...); err != nil {
				t.Fatalf("sync: %v\n%s", err, out)
			}
			if fileExists(orphan) {
				t.Error("next sync kept the restored orphan")
			}
		})
	}
}

func TestSweepLedgerOrphans_RollbackRestoresSweptLink(t *testing.T) {
	chdir(t, t.TempDir())
	mustWriteFile(t, ".agents/skills/gone/SKILL.md", "body\n")
	const link = ".cursor/skills/gone"
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("..", "..", ".agents", "skills", "gone"), link); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	sess := adapters.NewSession()
	sess.StartTransaction()

	removed, _, err := sweepLedgerOrphans(sess, []string{link}, nil, nil)
	if err != nil || !slices.Equal(removed, []string{link}) {
		t.Fatalf("sweep removed %v, %v; want the link", removed, err)
	}
	if _, err := os.Lstat(".cursor"); !os.IsNotExist(err) {
		t.Fatalf("sweep kept the emptied folder: %v", err)
	}
	if err := sess.Rollback(); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("link not restored: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(link, "SKILL.md")); err != nil || string(got) != "body\n" {
		t.Errorf("restored link does not reach the canonical folder: %q, %v", got, err)
	}
}
