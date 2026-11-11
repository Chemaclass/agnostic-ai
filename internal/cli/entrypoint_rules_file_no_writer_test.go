package cli

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

const noWriterBody = "NO-WRITER-BODY"

// syncTree syncs a project with config and one always-on rule, and returns
// every generated file by path. The config and .agnostic-ai/ are left out:
// they hold the input and sync state, not output.
func syncTree(t *testing.T, config string) map[string]string {
	t.Helper()
	dir := testutil.TempCwd(t)
	silence(t)
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\n"+config)
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "AGNOSTIC_AI.md"), "# Project\n\n"+noWriterBody+"\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "style.md"), "---\nname: style\n---\nUse tabs.\n")
	if out, err := runCLI(t, "sync", "--gitignore=off"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	tree := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel == ".agnostic-ai" {
				return filepath.SkipDir
			}
			return nil
		}
		if rel == "agnostic-ai.yaml" {
			return nil
		}
		data, err := os.ReadFile(path)
		tree[rel] = string(data)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return tree
}

func sameTree(t *testing.T, got, want map[string]string) {
	t.Helper()
	for path, content := range want {
		if g, ok := got[path]; !ok {
			t.Errorf("%s: missing", path)
		} else if g != content {
			t.Errorf("%s: differs\n--- got\n%s\n--- want\n%s", path, g, content)
		}
	}
	for path := range got {
		if _, ok := want[path]; !ok {
			t.Errorf("%s: unexpected", path)
		}
	}
}

// A target whose adapter writes no rules file used to treat
// outputs.<target>.rules-file as a claim on its entry point: sync skipped
// AGENTS.md and nothing wrote it (#3295). The key now has no effect there,
// whether it names the entry point or another path.
func TestSync_RulesFileOnTargetWithoutWriter_HasNoEffect(t *testing.T) {
	for _, target := range adapters.Names() {
		if adapters.WritesRulesFile(target) {
			continue
		}
		paths := []string{"custom/RULES.md"}
		if entry := adapters.EntryPointPath(nil, target); entry != "" {
			paths = append(paths, entry)
		}
		for _, path := range paths {
			t.Run(target+"/"+path, func(t *testing.T) {
				base := "targets: [" + target + "]\n"
				want := syncTree(t, base)

				got := syncTree(t, base+"outputs:\n  "+target+":\n    rules-file: "+path+"\n")

				sameTree(t, got, want)
				if entry := adapters.EntryPointPath(nil, target); entry != "" && !strings.Contains(got[entry], noWriterBody) {
					t.Errorf("%s must carry the AGNOSTIC_AI.md body, got:\n%s", entry, got[entry])
				}
				if _, ok := got["custom/RULES.md"]; ok {
					t.Error("custom/RULES.md: a target without a writer must not write it")
				}
			})
		}
	}
}

// codex and cline share AGENTS.md. Only codex writes a rules file, so the
// key on cline changes nothing, with or without the key on codex.
func TestSync_SharedEntryPoint_RulesFileOnlyCountsForTheWriter(t *testing.T) {
	base := "targets: [codex, cline]\n"
	t.Run("cline sets it", func(t *testing.T) {
		want := syncTree(t, base)

		got := syncTree(t, base+"outputs:\n  cline:\n    rules-file: AGENTS.md\n")

		sameTree(t, got, want)
	})
	t.Run("both set it", func(t *testing.T) {
		want := syncTree(t, base+"outputs:\n  codex:\n    rules-file: AGENTS.md\n")

		got := syncTree(t, base+"outputs:\n  codex:\n    rules-file: AGENTS.md\n  cline:\n    rules-file: AGENTS.md\n")

		sameTree(t, got, want)
	})
}

// Scoped rules refuse a rules-file only on a target that writes one.
func TestSync_ScopedRuleWithRulesFileOnTargetWithoutWriter(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [cline]\noutputs:\n  cline:\n    rules-file: AGENTS.md\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "payments.md"), "---\nname: payments\nscope: payments\n---\nRound to cents.\n")

	if out, err := runCLI(t, "sync", "--gitignore=off"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
}
