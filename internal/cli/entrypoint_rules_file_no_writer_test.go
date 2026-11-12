package cli

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

const noWriterBody = "NO-WRITER-BODY"

// syncTree syncs a project with config and one always-on rule, and returns
// every generated file by path. The config and .agnostic-ai/ are left out:
// they hold the input and sync state, not output.
func syncTree(t *testing.T, config string, rules ...string) map[string]string {
	t.Helper()
	dir := testutil.TempCwd(t)
	silence(t)
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\n"+config)
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "AGNOSTIC_AI.md"), "# Project\n\n"+noWriterBody+"\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "style.md"), "---\nname: style\n---\nUse tabs.\n")
	for i, rule := range rules {
		mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", fmt.Sprintf("extra%d.md", i)), rule)
	}
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
// key on cline changes nothing. The key on codex is a conflict, covered by
// TestSync_RulesFileOnASharedInstructionsFile_StopsWhenRulesAreLost.
func TestSync_SharedEntryPoint_RulesFileOnlyCountsForTheWriter(t *testing.T) {
	base := "targets: [codex, cline]\n"
	t.Run("cline sets it", func(t *testing.T) {
		want := syncTree(t, base)

		got := syncTree(t, base+"outputs:\n  cline:\n    rules-file: AGENTS.md\n")

		sameTree(t, got, want)
	})
	t.Run("cline sets it, with a glob rule", func(t *testing.T) {
		glob := "---\nname: api\nglobs: \"src/api/**\"\n---\nValidate input.\n"
		want := syncTree(t, base, glob)
		if _, ok := want["src/api/AGENTS.md"]; !ok {
			t.Fatalf("the glob rule must reach src/api/AGENTS.md without the key, got %v", want)
		}
		for _, path := range []string{"AGENTS.md", "custom/RULES.md"} {
			got := syncTree(t, base+"outputs:\n  cline:\n    rules-file: "+path+"\n", glob)

			sameTree(t, got, want)
		}
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

func externalRulesFileAdapterOnPath(t *testing.T, target, path string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture requires a POSIX shell")
	}
	bin := t.TempDir()
	script := filepath.Join(bin, "agnostic-ai-adapter-"+target)
	mustWriteFile(t, script, "#!/bin/sh\ncat >/dev/null\nprintf '%s' '{\"protocol_version\":1,\"files\":[{\"path\":\""+path+"\",\"content\":\"EXTERNAL RULES\\n\"}]}'\n")
	if err := os.Chmod(script, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// An external adapter receives outputs and may write its own rules file,
// so the key keeps its meaning there: sync leaves the file to the adapter
// and lint does not warn.
func TestSync_RulesFileOnExternalTarget_StaysWithTheAdapter(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	externalRulesFileAdapterOnPath(t, "foo", "FOO.md")
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [foo]\noutputs:\n  foo:\n    file: FOO.md\n    rules-file: FOO.md\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "AGNOSTIC_AI.md"), "# Project\n\n"+noWriterBody+"\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "style.md"), "---\nname: style\n---\nUse tabs.\n")

	if out, err := runCLI(t, "sync", "--gitignore=off"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}

	data, err := os.ReadFile(filepath.Join(dir, "FOO.md"))
	if err != nil || !strings.Contains(string(data), "EXTERNAL RULES") || strings.Contains(string(data), noWriterBody) {
		t.Errorf("FOO.md must keep the adapter's content, got %q (%v)", data, err)
	}
	if out, err := runCLI(t, "sync", "--check"); err != nil {
		t.Errorf("sync --check right after sync: %v\n%s", err, out)
	}
	if out, _ := runCLI(t, "lint"); strings.Contains(out, "LINT062") {
		t.Errorf("an external adapter may write its rules file:\n%s", out)
	}
}
