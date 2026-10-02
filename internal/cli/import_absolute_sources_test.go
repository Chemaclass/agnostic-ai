package cli

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/errs"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func absoluteImportProject(t *testing.T) string {
	t.Helper()
	testutil.Chdir(t, t.TempDir())
	external := filepath.Join(t.TempDir(), "portable", "skills")
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\nsources:\n  skills: "+filepath.ToSlash(external)+"\n")
	mustWriteFile(t, ".claude/skills/review/SKILL.md", nativeSkill)
	mustWriteFile(t, ".claude/skills/review/examples.md", "An example.\n")
	return external
}

func TestImport_WritesAbsoluteSourcesAndRecordsProvenance(t *testing.T) {
	external := absoluteImportProject(t)
	out, err := runAbsoluteImportCLI(t, "import", "claude")
	if err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}
	path := filepath.Join(external, "review", "SKILL.md")
	if got := readFile(t, path); !strings.Contains(got, "Native") {
		t.Errorf("skill = %q", got)
	}
	if got := readFile(t, filepath.Join(external, "review", "examples.md")); got != "An example.\n" {
		t.Errorf("asset = %q", got)
	}
	mirror := filepath.Join(".", strings.TrimLeft(strings.TrimPrefix(external, filepath.VolumeName(external)), `/\`))
	if _, err := os.Stat(mirror); err == nil {
		t.Error("import created a project mirror of the absolute source")
	}
	rec := readStateFile(".").SpecFileSums[filepath.ToSlash(path)]
	if rec.By != specSumByImport || strings.Join(rec.Sources, ",") != "claude" {
		t.Errorf("provenance = %#v", rec)
	}
	mustWriteFile(t, ".claude/skills/review/SKILL.md", strings.ReplaceAll(nativeSkill, "Native", "Edited"))
	if out, err := runAbsoluteImportCLI(t, "import", "claude"); err != nil {
		t.Fatalf("reimport: %v\n%s", err, out)
	}
	if got := readFile(t, path); !strings.Contains(got, "Edited") {
		t.Errorf("reimport = %q", got)
	}
}

func TestImport_AbsoluteSourcesPreviewDoesNotWrite(t *testing.T) {
	for _, diff := range []bool{false, true} {
		t.Run(map[bool]string{false: "summary", true: "diff"}[diff], func(t *testing.T) {
			external := absoluteImportProject(t)
			path := filepath.Join(external, "review", "SKILL.md")
			args := []string{"import", "claude", "--dry-run"}
			if diff {
				args = append(args, "--diff")
			}
			out, err := runAbsoluteImportCLI(t, args...)
			if err != nil {
				t.Fatalf("preview: %v\n%s", err, out)
			}
			if !strings.Contains(filepath.ToSlash(out), filepath.ToSlash(path)) {
				t.Errorf("preview lacks absolute destination %s:\n%s", path, out)
			}
			if _, err := os.Stat(external); !os.IsNotExist(err) {
				t.Errorf("preview wrote external source: %v", err)
			}
			if _, err := os.Stat(stateFilePath(".")); !os.IsNotExist(err) {
				t.Errorf("preview wrote project ledger: %v", err)
			}
			if _, err := os.Stat(filepath.Join(defaultBaseDir, projectLockName)); !os.IsNotExist(err) {
				t.Errorf("preview wrote project lock: %v", err)
			}
		})
	}
}

func TestImport_AbsoluteSourceConflictRollsBackThroughSymlink(t *testing.T) {
	external := absoluteImportProject(t)
	real := t.TempDir()
	if err := os.MkdirAll(filepath.Dir(external), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, external); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	path := filepath.Join(external, "review", "SKILL.md")
	mustWriteFile(t, path, handSkill)
	mustWriteFile(t, ".claude/skills/deploy/SKILL.md", "---\nname: deploy\ndescription: Ship.\n---\nShip\n")
	for _, flags := range [][]string{{"--dry-run"}, {"--dry-run", "--diff"}, nil} {
		out, err := runAbsoluteImportCLI(t, append([]string{"import", "claude"}, flags...)...)
		if errs.CodeOf(err) != errs.CodeImportWouldReplace || !strings.Contains(err.Error(), filepath.ToSlash(path)) {
			t.Errorf("flags %v: want absolute conflict, got %v\n%s", flags, err, out)
		}
		if got := readFile(t, path); got != handSkill {
			t.Errorf("flags %v: changed existing skill: %q", flags, got)
		}
		if _, err := os.Stat(filepath.Join(real, "deploy")); !os.IsNotExist(err) {
			t.Errorf("flags %v: left new skill after rollback: %v", flags, err)
		}
	}
	for _, flags := range [][]string{{"--dry-run", "--overwrite"}, {"--dry-run", "--diff", "--overwrite"}} {
		out, err := runAbsoluteImportCLI(t, append([]string{"import", "claude"}, flags...)...)
		if err != nil {
			t.Errorf("flags %v: overwrite preview: %v\n%s", flags, err, out)
		}
		if got := readFile(t, path); got != handSkill {
			t.Errorf("flags %v: overwrite preview changed skill: %q", flags, got)
		}
		if _, err := os.Stat(filepath.Join(real, "deploy")); !os.IsNotExist(err) {
			t.Errorf("flags %v: overwrite preview wrote new skill: %v", flags, err)
		}
	}
	if out, err := runAbsoluteImportCLI(t, "import", "claude", "--overwrite"); err != nil {
		t.Fatalf("overwrite: %v\n%s", err, out)
	}
	if got := readFile(t, path); !strings.Contains(got, "Native") {
		t.Errorf("overwrite = %q", got)
	}
	if info, err := os.Lstat(external); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("import changed source symlink: %v", err)
	}
}

func TestImport_AbsoluteCustomSourceKinds(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	external := t.TempDir()
	rules := filepath.Join(external, "custom-guidance")
	mcps := filepath.Join(external, "custom-servers")
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\nsources:\n  rules: "+filepath.ToSlash(rules)+"\n  mcps: "+filepath.ToSlash(mcps)+"\n")
	mustWriteFile(t, ".claude/rules/style.md", "---\ndescription: Style.\n---\nUse plain words.\n")
	mustWriteFile(t, ".mcp.json", `{"mcpServers":{"example":{"command":"example"}}}`)
	if out, err := runAbsoluteImportCLI(t, "import", "claude"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}
	if got := readFile(t, filepath.Join(rules, "style.md")); !strings.Contains(got, "Use plain words") {
		t.Errorf("rule = %q", got)
	}
	if got := readFile(t, filepath.Join(mcps, "example.yaml")); !strings.Contains(got, "command: example") {
		t.Errorf("server = %q", got)
	}
}

func runAbsoluteImportCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var err error
	out := captureStdout(t, func() { _, err = runCLI(t, args...) })
	return out, err
}

func TestImport_SyncRendersAbsoluteSourcesAndRecordsProvenance(t *testing.T) {
	external := absoluteImportProject(t)
	if out, err := runAbsoluteImportCLI(t, "import", "claude"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}
	path := filepath.Join(external, "review", "SKILL.md")
	mustWriteFile(t, path, strings.ReplaceAll(nativeSkill, "Native", "Portable"))
	if out, err := runAbsoluteImportCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if got := readFile(t, ".claude/skills/review/SKILL.md"); !strings.Contains(got, "Portable") {
		t.Errorf("sync did not render absolute source: %q", got)
	}
	rec := readStateFile(".").SpecFileSums[filepath.ToSlash(path)]
	if rec.By != specSumBySync || strings.Join(rec.Targets, ",") != "claude" {
		t.Errorf("sync provenance = %#v", rec)
	}
	asset := filepath.Join(external, "review", "examples.md")
	if rec := readStateFile(".").SpecFileSums[filepath.ToSlash(asset)]; rec.By != specSumBySync {
		t.Errorf("sync asset provenance = %#v", rec)
	}
	mustWriteFile(t, ".claude/skills/review/SKILL.md", strings.ReplaceAll(nativeSkill, "Native", "Edited"))
	if out, err := runAbsoluteImportCLI(t, "import", "claude"); err != nil {
		t.Fatalf("import after sync: %v\n%s", err, out)
	}
	if got := readFile(t, path); !strings.Contains(got, "Edited") {
		t.Errorf("import after sync = %q", got)
	}
}

func TestImport_AbsoluteSourcesPreviewReportsMultiSourceConflict(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	external := filepath.Join(t.TempDir(), "custom-rules")
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, cursor]\nsources:\n  rules: "+filepath.ToSlash(external)+"\n")
	mustWriteFile(t, ".claude/rules/style.md", "Claude style.\n")
	mustWriteFile(t, ".cursor/rules/style.mdc", "Cursor style.\n")
	out, err := runAbsoluteImportCLI(t, "import", "claude", "cursor", "--dry-run", "--diff")
	if err != nil {
		t.Fatalf("preview: %v\n%s", err, out)
	}
	path := filepath.ToSlash(filepath.Join(external, "style.md"))
	if !strings.Contains(out, "conflict "+path) || !strings.Contains(out, "cursor (last) is kept") {
		t.Errorf("missing multi-source conflict:\n%s", out)
	}
	if _, err := os.Stat(external); !os.IsNotExist(err) {
		t.Errorf("preview wrote external rules: %v", err)
	}
}

func TestImport_AbsoluteSourceInsideProjectPreviewKeepsOverwriteGuard(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "portable", "skills")
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\nsources:\n  skills: "+filepath.ToSlash(source)+"\n")
	path := filepath.Join(source, "review", "SKILL.md")
	mustWriteFile(t, path, handSkill)
	mustWriteFile(t, ".claude/skills/review/SKILL.md", nativeSkill)
	out, err := runAbsoluteImportCLI(t, "import", "claude", "--dry-run", "--diff")
	if errs.CodeOf(err) != errs.CodeImportWouldReplace || !strings.Contains(out, filepath.ToSlash(path)) {
		t.Errorf("want absolute conflict: %v\n%s", err, out)
	}
	if got := readFile(t, path); got != handSkill {
		t.Errorf("preview changed skill: %q", got)
	}
	if out, err := runAbsoluteImportCLI(t, "import", "claude", "--overwrite"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}
	mustWriteFile(t, ".claude/skills/review/SKILL.md", strings.ReplaceAll(nativeSkill, "Native", "Edited"))
	if out, err := runAbsoluteImportCLI(t, "import", "claude", "--dry-run", "--diff"); err != nil {
		t.Errorf("preview rejected recorded absolute source inside project: %v\n%s", err, out)
	}
}

func TestImport_AbsoluteSourcesPreviewKeepsExternalFilesOutOfNativeWalk(t *testing.T) {
	external := absoluteImportProject(t)
	mustWriteFile(t, filepath.Join(external, "CLAUDE.md"), "## External\n\nStray native text.\n")
	out, err := runAbsoluteImportCLI(t, "import", "claude", "--dry-run", "--diff")
	if err != nil {
		t.Fatalf("preview: %v\n%s", err, out)
	}
	if strings.Contains(out, "Stray native text") || strings.Contains(out, "source-") {
		t.Errorf("preview imported native files outside the project:\n%s", out)
	}
	if got := readFile(t, filepath.Join(external, "CLAUDE.md")); got != "## External\n\nStray native text.\n" {
		t.Errorf("preview changed external file: %q", got)
	}
}

func TestImportPreviewKeepsAbsoluteSourceInsideProject(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "portable", "skills")
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\nsources:\n  skills: "+filepath.ToSlash(source)+"\n")
	keep := importPreviewKeeps(root)
	found := false
	for _, path := range keep.paths {
		if path == "portable/skills" {
			found = true
		}
	}
	if !found {
		t.Errorf("preview can drop an ignored absolute source inside project: %v", keep.paths)
	}
}

func TestImport_ClassifiedNativeFilesUseAbsoluteSources(t *testing.T) {
	cases := []struct{ target, dir, suffix string }{
		{"cline", ".clinerules", ".md"},
		{"copilot", ".github/instructions", ".instructions.md"},
		{"windsurf", ".windsurf/rules", ".md"},
	}
	for _, tc := range cases {
		t.Run(tc.target, func(t *testing.T) {
			testutil.Chdir(t, t.TempDir())
			external := t.TempDir()
			mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: ["+tc.target+"]\nsources:\n  rules: "+filepath.ToSlash(filepath.Join(external, "guidance"))+"\n  agents: "+filepath.ToSlash(filepath.Join(external, "workers"))+"\n  skills: "+filepath.ToSlash(filepath.Join(external, "tasks"))+"\n")
			files := []struct{ native, dest, body string }{
				{"style", "guidance/style.md", "# Style\n\nPlain rule.\n"},
				{"agent-reviewer", "workers/reviewer.md", "# Agent: reviewer\n\nReview the diff.\n"},
				{"skill-validator", "tasks/validator.md", "# Skill: validator\n\nValidate input.\n"},
			}
			for _, file := range files {
				mustWriteFile(t, filepath.Join(tc.dir, file.native+tc.suffix), file.body)
			}
			out, err := runAbsoluteImportCLI(t, "import", tc.target, "--dry-run", "--diff")
			if err != nil {
				t.Fatalf("preview: %v\n%s", err, out)
			}
			for _, file := range files {
				path := filepath.Join(external, filepath.FromSlash(file.dest))
				if !strings.Contains(out, filepath.ToSlash(path)) {
					t.Errorf("preview lacks %s:\n%s", path, out)
				}
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Errorf("preview wrote %s: %v", path, err)
				}
			}
			if out, err := runAbsoluteImportCLI(t, "import", tc.target); err != nil {
				t.Fatalf("import: %v\n%s", err, out)
			}
			for _, file := range files {
				data, err := os.ReadFile(filepath.Join(external, filepath.FromSlash(file.dest)))
				if err != nil {
					t.Errorf("missing %s: %v", file.dest, err)
					continue
				}
				if !strings.Contains(string(data), strings.TrimSpace(strings.SplitN(file.body, "\n\n", 2)[1])) {
					t.Errorf("body %s = %s", file.dest, data)
				}
			}
		})
	}
}

func TestCopyImportAbsoluteSources_ExcludesContainingPreviewRoot(t *testing.T) {
	source := t.TempDir()
	project := t.TempDir()
	preview := filepath.Join(source, "preview")
	shadow := filepath.Join(preview, "project")
	mustWriteFile(t, filepath.Join(shadow, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\nsources:\n  rules: "+filepath.ToSlash(source)+"\n")
	mustWriteFile(t, filepath.Join(source, "portable.md"), "Portable rule.\n")
	blocked := filepath.Join(preview, "000-blocked")
	if err := os.Mkdir(blocked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(blocked, 0o700) })
	if _, err := os.ReadDir(blocked); err == nil {
		t.Skip("requires directory permission enforcement to bound the old copier")
	}
	copies, err := copyImportAbsoluteSources(project, shadow, preview, previewCopy{})
	if err != nil {
		t.Fatalf("source copy entered its preview root: %v", err)
	}
	if len(copies) != 1 {
		t.Fatalf("copies = %#v", copies)
	}
	if got := readFile(t, filepath.Join(copies[0].shadow, "portable.md")); got != "Portable rule.\n" {
		t.Errorf("portable source = %q", got)
	}
	if _, err := os.Stat(filepath.Join(copies[0].shadow, "preview")); !os.IsNotExist(err) {
		t.Errorf("copied preview root into source copy: %v", err)
	}
}

func TestImport_AbsoluteInternalSourcesAreSkippedByNativeScans(t *testing.T) {
	cases := []struct{ target, native string }{
		{"codex", "AGENTS.md"},
		{"gemini", "GEMINI.md"},
		{"windsurf", ".windsurf/rules/style.md"},
	}
	for _, tc := range cases {
		t.Run(tc.target, func(t *testing.T) {
			testutil.Chdir(t, t.TempDir())
			root, err := os.Getwd()
			if err != nil {
				t.Fatal(err)
			}
			source := filepath.Join(root, "portable", "rules")
			mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: ["+tc.target+"]\nsources:\n  rules: "+filepath.ToSlash(source)+"\n")
			mustWriteFile(t, filepath.Join(source, filepath.FromSlash(tc.native)), "Internal source context.\n")
			wantEntries := 1
			if tc.target == "windsurf" {
				mustWriteFile(t, ".windsurf/rules/root.md", "Root native context.\n")
				wantEntries = 2
			}
			out, err := runAbsoluteImportCLI(t, "import", tc.target, "--dry-run", "--diff")
			if err != nil {
				t.Fatalf("preview: %v\n%s", err, out)
			}
			if strings.Contains(out, "Internal source context") {
				t.Errorf("preview self-imported source:\n%s", out)
			}
			if out, err := runAbsoluteImportCLI(t, "import", tc.target); err != nil {
				t.Fatalf("import: %v\n%s", err, out)
			}
			entries, err := os.ReadDir(source)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != wantEntries {
				t.Errorf("source self-import created %d entries, want %d: %v", len(entries), wantEntries, entries)
			}
		})
	}
}

func TestImport_AbsoluteSourceAliasesSharePreviewWrites(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	external := t.TempDir()
	shared := filepath.Join(external, "shared")
	alias := filepath.Join(external, "alias")
	mustWriteFile(t, filepath.Join(shared, "foo.md"), "---\nname: foo\ndescription: Mine.\n---\nMine.\n")
	if err := os.Symlink(shared, alias); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\nsources:\n  rules: "+filepath.ToSlash(shared)+"\n  agents: "+filepath.ToSlash(alias)+"\n")
	mustWriteFile(t, ".claude/rules/foo.md", "Rule replacement.\n")
	mustWriteFile(t, ".claude/agents/foo.md", "---\nname: foo\ndescription: Agent.\n---\nAgent replacement.\n")
	preview, err := planImportPreview([]string{"claude"})
	if err != nil {
		t.Fatal(err)
	}
	if out, err := runAbsoluteImportCLI(t, "import", "claude", "--overwrite"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}
	found := 0
	for _, e := range preview.entries {
		if e.path != filepath.ToSlash(filepath.Join(shared, "foo.md")) && e.path != filepath.ToSlash(filepath.Join(alias, "foo.md")) {
			continue
		}
		found++
		if got := readFile(t, filepath.FromSlash(e.path)); got != string(e.after) {
			t.Errorf("preview differs from real alias destination %s: preview=%q real=%q", e.path, e.after, got)
		}
	}
	if found != 2 {
		t.Errorf("preview lost lexical destinations: %#v", preview.entries)
	}
}

func TestImport_AbsoluteSourcesResolveClaudeNativeReferences(t *testing.T) {
	external := absoluteImportProject(t)
	rules := filepath.Join(filepath.Dir(external), "rules")
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\nsources:\n  skills: "+filepath.ToSlash(external)+"\n  rules: "+filepath.ToSlash(rules)+"\n")
	mustWriteFile(t, "CLAUDE.md", "@.claude/rules/style.md\n\nRead ./.claude/skills/review/SKILL.md.\n")
	mustWriteFile(t, ".claude/rules/style.md", "Plain rule.\n")
	out, err := runAbsoluteImportCLI(t, "import", "claude", "--dry-run", "--diff")
	if err != nil {
		t.Fatalf("preview: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Read "+filepath.ToSlash(filepath.Join(external, "review", "SKILL.md"))) {
		t.Errorf("preview reference unresolved:\n%s", out)
	}
	if out, err := runAbsoluteImportCLI(t, "import", "claude"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}
	got := readFile(t, agnosticMainFile)
	if strings.Contains(got, "@.claude/rules/style.md") || !strings.Contains(got, "Read "+filepath.ToSlash(filepath.Join(external, "review", "SKILL.md"))) {
		t.Errorf("absolute references unresolved: %q", got)
	}
}

func TestImport_AbsoluteHardLinkedSourcesSharePreviewWrites(t *testing.T) {
	for _, layout := range []string{"external", "project", "project-link"} {
		t.Run(layout, func(t *testing.T) {
			testutil.Chdir(t, t.TempDir())
			project, err := os.Getwd()
			if err != nil {
				t.Fatal(err)
			}
			external := t.TempDir()
			rules := filepath.Join(external, "rules")
			agents := filepath.Join(external, "agents")
			if layout != "external" {
				rules = filepath.Join(project, "portable", "rules")
			}
			if layout == "project-link" {
				if err := os.MkdirAll(filepath.Dir(rules), 0o755); err != nil {
					t.Fatal(err)
				}
				real := filepath.Join(external, "rules")
				if err := os.MkdirAll(real, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(real, rules); err != nil {
					t.Skipf("symlinks unsupported: %v", err)
				}
			}
			rulePath := filepath.Join(rules, "foo.md")
			agentPath := filepath.Join(agents, "foo.md")
			const mine = "---\nname: foo\ndescription: Mine.\n---\nMine.\n"
			mustWriteFile(t, rulePath, mine)
			if err := os.MkdirAll(agents, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Link(rulePath, agentPath); err != nil {
				t.Fatal(err)
			}
			mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\nsources:\n  rules: "+filepath.ToSlash(rules)+"\n  agents: "+filepath.ToSlash(agents)+"\n")
			mustWriteFile(t, ".claude/rules/foo.md", "Rule replacement.\n")
			mustWriteFile(t, ".claude/agents/foo.md", "---\nname: foo\ndescription: Agent.\n---\nAgent replacement.\n")
			preview, err := planImportPreview([]string{"claude"})
			if err != nil {
				t.Fatal(err)
			}
			if got := readFile(t, rulePath); got != mine {
				t.Errorf("preview wrote original rules file: %q", got)
			}
			if got := readFile(t, agentPath); got != mine {
				t.Errorf("preview wrote original agent file: %q", got)
			}
			if out, err := runAbsoluteImportCLI(t, "import", "claude", "--overwrite"); err != nil {
				t.Fatalf("import: %v\n%s", err, out)
			}
			found := 0
			for _, e := range preview.entries {
				if e.path != filepath.ToSlash(rulePath) && e.path != filepath.ToSlash(agentPath) {
					continue
				}
				found++
				if got := readFile(t, filepath.FromSlash(e.path)); got != string(e.after) {
					t.Errorf("preview differs from real hard-linked destination %s: preview=%q real=%q", e.path, e.after, got)
				}
			}
			if found != 2 {
				t.Errorf("preview lost hard-linked lexical destinations: %#v", preview.entries)
			}
		})
	}
}

func TestImport_KiloRecoversRulesIntoMissingAbsoluteSource(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	source := filepath.Join(t.TempDir(), "portable", "rules")
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [kilo, codex]\nsources:\n  rules: "+filepath.ToSlash(source)+"\n")
	mustWriteFile(t, "AGENTS.md", "<!-- agnostic-ai:rules:start -->\n\n## Rules\n\n### Recovered\n\nRecovered guidance.\n\n<!-- agnostic-ai:rules:end -->\n")
	path := filepath.Join(source, "recovered.md")
	out, err := runAbsoluteImportCLI(t, "import", "kilo", "--dry-run", "--diff")
	if err != nil {
		t.Fatalf("preview: %v\n%s", err, out)
	}
	if !strings.Contains(out, filepath.ToSlash(path)) {
		t.Errorf("preview lacks absolute recovery path:\n%s", out)
	}
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Errorf("preview wrote source directory: %v", err)
	}
	if out, err := runAbsoluteImportCLI(t, "import", "kilo"); err != nil {
		t.Fatalf("recovery: %v\n%s", err, out)
	}
	if got := readFile(t, path); !strings.Contains(got, "Recovered guidance.") {
		t.Errorf("recovered rule = %q", got)
	}
	const mine = "---\nname: recovered\n---\nMy guidance.\n"
	mustWriteFile(t, path, mine)
	if out, err := runAbsoluteImportCLI(t, "import", "kilo"); err != nil {
		t.Fatalf("hand-written rule: %v\n%s", err, out)
	}
	if got := readFile(t, path); got != mine {
		t.Errorf("recovery replaced hand-written rule: %q", got)
	}
}

func TestImport_AbsoluteSourceAncestorSharesMissingProjectDestination(t *testing.T) {
	parent := t.TempDir()
	project := filepath.Join(parent, "repo")
	if err := os.Mkdir(project, 0o755); err != nil {
		t.Fatal(err)
	}
	testutil.Chdir(t, project)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\nsources:\n  rules: "+filepath.ToSlash(parent)+"\n")
	mustWriteFile(t, ".claude/rules/repo/.agnostic-ai/agents/foo.md", "Rule replacement.\n")
	mustWriteFile(t, ".claude/agents/foo.md", "---\nname: foo\ndescription: Agent.\n---\nAgent replacement.\n")
	path := filepath.Join(parent, "repo", ".agnostic-ai", "agents", "foo.md")
	preview, err := planImportPreview([]string{"claude"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("preview wrote original destination: %v", err)
	}
	if out, err := runAbsoluteImportCLI(t, "import", "claude"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}
	actual := readFile(t, path)
	found := 0
	for _, e := range preview.entries {
		if e.path != filepath.ToSlash(path) && e.path != ".agnostic-ai/agents/foo.md" {
			continue
		}
		found++
		if string(e.after) != actual {
			t.Errorf("preview differs from real overlapping destination %s: preview=%q real=%q", e.path, e.after, actual)
		}
	}
	if found != 2 {
		t.Errorf("preview lost absolute/relative lexical destinations: %#v", preview.entries)
	}
}

func TestImport_RecoveryReadsRuleNamesFromMappedAbsoluteSources(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	source := filepath.Join(t.TempDir(), "rules")
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, kilo, codex]\nsources:\n  rules: "+filepath.ToSlash(source)+"\n")
	mustWriteFile(t, "CLAUDE.md", "Shared main.\n")
	mustWriteFile(t, ".claude/rules/foo.md", "---\nname: bar\n---\nNative foo named bar.\n")
	mustWriteFile(t, "AGENTS.md", "<!-- agnostic-ai:rules:start -->\n\n## Rules\n\n### Bar\n\nRecovered guidance.\n\n<!-- agnostic-ai:rules:end -->\n")
	preview, err := planImportPreview([]string{"claude", "kilo"})
	if err != nil {
		t.Fatal(err)
	}
	extra := filepath.Join(source, "bar.md")
	for _, e := range preview.entries {
		if e.path == filepath.ToSlash(extra) {
			t.Errorf("preview recovered name already imported from foo.md: %s", e.path)
		}
	}
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Errorf("preview wrote original rules directory: %v", err)
	}
	if out, err := runAbsoluteImportCLI(t, "import", "claude", "kilo"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}
	if got := readFile(t, filepath.Join(source, "foo.md")); !strings.Contains(got, "name: bar") {
		t.Errorf("imported name = %q", got)
	}
	if _, err := os.Stat(extra); !os.IsNotExist(err) {
		t.Errorf("real import recovered duplicate name: %v", err)
	}
}

func TestImport_RecoveryReadsRuleNamesThroughAbsoluteSourceAlias(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	external := t.TempDir()
	rules := filepath.Join(external, "z-rules")
	agents := filepath.Join(external, "a-agents")
	if err := os.Mkdir(rules, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := createImportSourceAlias(rules, agents); err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, kilo, codex]\nsources:\n  rules: "+filepath.ToSlash(rules)+"\n  agents: "+filepath.ToSlash(agents)+"\n")
	mustWriteFile(t, "CLAUDE.md", "Shared main.\n")
	mustWriteFile(t, ".claude/rules/foo.md", "---\nname: bar\n---\nNative foo named bar.\n")
	mustWriteFile(t, "AGENTS.md", "<!-- agnostic-ai:rules:start -->\n\n## Rules\n\n### Bar\n\nRecovered guidance.\n\n<!-- agnostic-ai:rules:end -->\n")
	preview, err := planImportPreview([]string{"claude", "kilo"})
	if err != nil {
		t.Fatal(err)
	}
	extra := filepath.Join(rules, "bar.md")
	for _, entry := range preview.entries {
		if entry.path == filepath.ToSlash(extra) {
			t.Errorf("preview recovered a rule name already imported through its alias: %s", entry.path)
		}
	}
	if _, err := os.Stat(filepath.Join(rules, "foo.md")); !os.IsNotExist(err) {
		t.Errorf("preview wrote original rules: %v", err)
	}
	if out, err := runAbsoluteImportCLI(t, "import", "claude", "kilo"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}
	if _, err := os.Stat(extra); !os.IsNotExist(err) {
		t.Errorf("real import recovered a duplicate name: %v", err)
	}
}

func TestImport_RecoveryReadsRuleNamesFromAbsoluteRuleAlias(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	external := t.TempDir()
	shared := filepath.Join(external, "z-shared")
	rules := filepath.Join(external, "a-rules")
	mustWriteFile(t, filepath.Join(shared, "foo.md"), "---\nname: bar\n---\nExisting guidance named bar.\n")
	if err := createImportSourceAlias(shared, rules); err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [kilo, codex]\nsources:\n  rules: "+filepath.ToSlash(rules)+"\n")
	mustWriteFile(t, "AGENTS.md", "<!-- agnostic-ai:rules:start -->\n\n## Rules\n\n### Bar\n\nRecovered guidance.\n\n<!-- agnostic-ai:rules:end -->\n")
	preview, err := planImportPreview([]string{"kilo"})
	if err != nil {
		t.Fatal(err)
	}
	extra := filepath.Join(rules, "bar.md")
	for _, entry := range preview.entries {
		if entry.path == filepath.ToSlash(extra) {
			t.Errorf("preview recovered an existing rule name: %s", entry.path)
		}
	}
	if out, err := runAbsoluteImportCLI(t, "import", "kilo"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}
	if _, err := os.Stat(extra); !os.IsNotExist(err) {
		t.Errorf("real import recovered a duplicate name: %v", err)
	}
}

func TestImport_PreviewSkipsUnusedSymlinkLoop(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [cursor]\n")
	mustWriteFile(t, ".cursor/rules/foo.mdc", "Cursor rule.\n")
	if err := os.Symlink("loop", "loop"); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	if out, err := runAbsoluteImportCLI(t, "import", "cursor", "--dry-run"); err != nil {
		t.Errorf("preview opened unused symlink loop: %v\n%s", err, out)
	}
	if out, err := runAbsoluteImportCLI(t, "import", "cursor"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}
}

func TestImport_RelativeAndAbsoluteSourceAliasesShareNewFiles(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	external := t.TempDir()
	rules := filepath.Join(defaultBaseDir, "rules")
	if err := os.Mkdir(defaultBaseDir, 0o755); err != nil {
		t.Fatal(err)
	}
	absRules, err := filepath.Abs(rules)
	if err != nil {
		t.Fatal(err)
	}
	if err := createImportSourceAlias(external, absRules); err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [cursor, claude]\nsources:\n  rules: .agnostic-ai/rules\n  agents: "+filepath.ToSlash(external)+"\n")
	mustWriteFile(t, ".cursor/rules/foo.mdc", "Cursor rule.\n")
	mustWriteFile(t, ".claude/agents/foo.md", "---\nname: foo\ndescription: Agent.\n---\nClaude agent.\n")
	preview, err := planImportPreview([]string{"cursor", "claude"})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(external, "foo.md")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("preview wrote original alias destination: %v", err)
	}
	if out, err := runAbsoluteImportCLI(t, "import", "cursor", "claude"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}
	actual := readFile(t, path)
	found := 0
	for _, entry := range preview.entries {
		if entry.path != filepath.ToSlash(path) && entry.path != ".agnostic-ai/rules/foo.md" {
			continue
		}
		found++
		if string(entry.after) != actual {
			t.Errorf("preview differs from real shared destination %s: preview=%q real=%q", entry.path, entry.after, actual)
		}
	}
	if found != 2 {
		t.Errorf("preview lost relative or absolute destination: %#v", preview.entries)
	}
}

func TestImport_AbsoluteSourcePreviewSkipsUnreadableUnrelatedDirectory(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	source := t.TempDir()
	blocked := filepath.Join(source, "private")
	if err := os.Mkdir(blocked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(blocked, 0o700) })
	if _, err := os.ReadDir(blocked); err == nil {
		t.Skip("requires directory permission enforcement")
	}
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [cursor]\nsources:\n  rules: "+filepath.ToSlash(source)+"\n")
	mustWriteFile(t, ".cursor/rules/foo.mdc", "Cursor rule.\n")
	if out, err := runAbsoluteImportCLI(t, "import", "cursor", "--dry-run", "--diff"); err != nil {
		t.Errorf("preview opened unrelated unreadable directory: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(source, "foo.md")); !os.IsNotExist(err) {
		t.Errorf("preview wrote original source: %v", err)
	}
	if out, err := runAbsoluteImportCLI(t, "import", "cursor"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}
	if got := readFile(t, filepath.Join(source, "foo.md")); !strings.Contains(got, "Cursor rule.") {
		t.Errorf("imported rule = %q", got)
	}
}

func TestImport_AbsoluteSourcePreviewReportsUnreadableDestination(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	source := t.TempDir()
	path := filepath.Join(source, "foo.md")
	mustWriteFile(t, path, "Mine.\n")
	if err := os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })
	if _, err := os.ReadFile(path); err == nil {
		t.Skip("requires file permission enforcement")
	}
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [cursor]\nsources:\n  rules: "+filepath.ToSlash(source)+"\n")
	mustWriteFile(t, ".cursor/rules/foo.mdc", "Cursor rule.\n")
	for _, flags := range [][]string{{"--dry-run", "--diff", "--overwrite"}, {"--overwrite"}} {
		out, err := runAbsoluteImportCLI(t, append([]string{"import", "cursor"}, flags...)...)
		if !errors.Is(err, fs.ErrPermission) {
			t.Errorf("flags %v: want unreadable destination error, got %v\n%s", flags, err, out)
		}
		if err != nil && !strings.Contains(err.Error(), "foo.md") {
			t.Errorf("flags %v: error lost destination path: %v", flags, err)
		}
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, path); got != "Mine.\n" {
		t.Errorf("import changed unreadable source: %q", got)
	}
}
