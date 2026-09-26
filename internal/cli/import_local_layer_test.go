package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

const (
	sharedOverRule  = "---\nname: over\ndescription: shared rule\n---\nShared body.\n"
	sharedRevAgent  = "---\nname: rev\ndescription: shared agent\n---\nShared agent.\n"
	localRuleSource = ".agnostic-ai/local/rules/personal.md"
)

// syncWithLocalSpecs writes a project whose local layer adds a rule, an
// agent, and a folder skill, and extends one shared rule and one shared
// agent, then runs a real sync for claude and codex.
func syncWithLocalSpecs(t *testing.T) {
	t.Helper()
	testutil.TempCwd(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, codex]\n")
	writeAgnosticFile(t, "# Shared\n")
	writeFile(t, filepath.Join(".agnostic-ai", "rules", "over.md"), sharedOverRule)
	writeFile(t, filepath.Join(".agnostic-ai", "agents", "rev.md"), sharedRevAgent)
	local := filepath.FromSlash(defaultProjectUser)
	writeFile(t, filepath.FromSlash(localRuleSource), "---\nname: personal\ndescription: personal rule\n---\nPersonal body.\n")
	writeFile(t, filepath.Join(local, "rules", "over.md"), "---\nname: over\n---\n::parent\nLocal addition.\n")
	writeFile(t, filepath.Join(local, "agents", "helper.md"), "---\nname: helper\ndescription: local agent\n---\nHelper agent.\n")
	writeFile(t, filepath.Join(local, "agents", "rev.md"), "---\nname: rev\ndescription: local description\n---\n")
	writeFile(t, filepath.Join(local, "skills", "mine", "SKILL.md"), "---\nname: mine\ndescription: local skill\n---\nSkill body.\n")
	writeFile(t, filepath.Join(local, "skills", "mine", "notes.txt"), "asset\n")
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
}

// importCapturing runs `import args...` and returns what it printed.
func importCapturing(t *testing.T, args ...string) string {
	t.Helper()
	return captureStdout(t, func() {
		if out, err := runCLI(t, append([]string{"import"}, args...)...); err != nil {
			t.Fatalf("import %v: %v\n%s", args, err, out)
		}
	})
}

func assertNotExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("%s: want no shared copy of a local spec, stat err = %v", path, err)
	}
}

func assertFileEquals(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	if string(data) != want {
		t.Errorf("%s changed:\ngot:\n%s\nwant:\n%s", path, data, want)
	}
}

func TestImport_LeavesLocalOnlySpecsOutOfTheSharedSource(t *testing.T) {
	for _, sources := range [][]string{{"claude"}, {"codex"}, {"claude", "codex"}} {
		t.Run(strings.Join(sources, "+"), func(t *testing.T) {
			syncWithLocalSpecs(t)

			out := importCapturing(t, sources...)

			assertNotExist(t, filepath.Join(".agnostic-ai", "rules", "personal.md"))
			assertNotExist(t, filepath.Join(".agnostic-ai", "agents", "helper.md"))
			assertNotExist(t, filepath.Join(".agnostic-ai", "skills", "mine"))
			for _, name := range []string{"agent helper", "rule personal", "skill mine"} {
				if !strings.Contains(out, name) {
					t.Errorf("want the note to name %q:\n%s", name, out)
				}
			}
		})
	}
}

func TestImport_KeepsSharedSpecsTheLocalLayerExtends(t *testing.T) {
	for _, sources := range [][]string{{"claude"}, {"codex"}, {"claude", "codex"}} {
		t.Run(strings.Join(sources, "+"), func(t *testing.T) {
			syncWithLocalSpecs(t)

			out := importCapturing(t, sources...)

			assertFileEquals(t, filepath.Join(".agnostic-ai", "rules", "over.md"), sharedOverRule)
			assertFileEquals(t, filepath.Join(".agnostic-ai", "agents", "rev.md"), sharedRevAgent)
			if !strings.Contains(out, "rule over") {
				t.Errorf("want the note to name the extended rule:\n%s", out)
			}
			if strings.Contains(out, "merge by hand") {
				t.Errorf("want no hint to merge local content into the shared rule:\n%s", out)
			}
		})
	}
}

func TestImport_StillImportsNativeSpecsWithNoLocalCounterpart(t *testing.T) {
	syncWithLocalSpecs(t)
	writeFile(t, filepath.Join(".claude", "agents", "manual.md"), "---\nname: manual\ndescription: hand written\n---\nManual agent.\n")

	importCapturing(t, "claude")

	if _, err := os.Stat(filepath.Join(".agnostic-ai", "agents", "manual.md")); err != nil {
		t.Errorf("hand-written agent not imported: %v", err)
	}
}

func TestImport_DryRunDoesNotListLocalSpecs(t *testing.T) {
	syncWithLocalSpecs(t)

	out := importCapturing(t, "codex", "--dry-run")

	if strings.Contains(out, filepath.Join(".agnostic-ai", "rules", "personal.md")) {
		t.Errorf("dry-run lists the local rule as a shared write:\n%s", out)
	}
	if !strings.Contains(out, "rule personal") {
		t.Errorf("want the dry-run note to name the local rule:\n%s", out)
	}
}

// syncWithLocalHooks writes a project with one shared hook and two local
// hooks, one of them sharing the shared hook's event, then runs a real
// sync for claude and codex. Claude groups the shared and local
// SessionStart handlers into one native entry.
func syncWithLocalHooks(t *testing.T) {
	t.Helper()
	testutil.TempCwd(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, codex]\n")
	writeAgnosticFile(t, "# Shared\n")
	writeFile(t, filepath.Join(".agnostic-ai", "hooks", "start.yaml"), "name: start\nevent: SessionStart\ncommand: echo shared-start\n")
	local := filepath.FromSlash(defaultProjectUser)
	writeFile(t, filepath.Join(local, "hooks", "guard.yaml"), "name: guard\nevent: PreToolUse\nmatcher: Bash\ncommand: echo local-guard\n")
	writeFile(t, filepath.Join(local, "hooks", "boot.yaml"), "name: boot\nevent: SessionStart\ncommand: [echo local-one, echo local-two]\n")
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
}

// sharedHookFiles returns the content of every file in the shared hooks
// directory, keyed by file name.
func sharedHookFiles(t *testing.T) map[string]string {
	t.Helper()
	dir := filepath.Join(".agnostic-ai", "hooks")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("%s: %v", dir, err)
	}
	files := map[string]string{}
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("%s: %v", e.Name(), err)
		}
		files[e.Name()] = string(data)
	}
	return files
}

func TestImport_LeavesLocalHooksOutOfTheSharedSource(t *testing.T) {
	for _, sources := range [][]string{{"claude"}, {"codex"}, {"claude", "codex"}} {
		t.Run(strings.Join(sources, "+"), func(t *testing.T) {
			syncWithLocalHooks(t)

			out := importCapturing(t, sources...)

			sharedCopies := 0
			for name, data := range sharedHookFiles(t) {
				if strings.Contains(data, "local-") {
					t.Errorf("%s holds a local hook command:\n%s", name, data)
				}
				if strings.Contains(data, "echo shared-start") {
					sharedCopies++
				}
			}
			if sharedCopies < 2 {
				t.Errorf("want the shared handler imported next to start.yaml, found it in %d file(s)", sharedCopies)
			}
			for _, name := range []string{"hook boot", "hook guard"} {
				if !strings.Contains(out, name) {
					t.Errorf("want the note to name %q:\n%s", name, out)
				}
			}
		})
	}
}

// A destination the guard cannot read must fail the import, not be
// taken for a missing file and removed when the run ends.
func TestImport_KeepsAnUnreadableSharedSpecTheLocalLayerExtends(t *testing.T) {
	skipUnlessCanDenyDirReads(t)
	syncWithLocalSpecs(t)
	path := filepath.Join(".agnostic-ai", "rules", "over.md")
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })

	silence(t)
	_, _ = runCLI(t, "import", "claude")

	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatalf("shared rule removed: %v", err)
	}
	assertFileEquals(t, path, sharedOverRule)
}

// A local skill's name only claims the folder that holds a SKILL.md with
// that name, never a scope folder or another skill's asset folder.
func TestImport_LocalSkillNameDoesNotClaimOtherSkillFolders(t *testing.T) {
	testutil.TempCwd(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	local := filepath.FromSlash(defaultProjectUser)
	writeFile(t, filepath.Join(local, "skills", "services", "SKILL.md"), "---\nname: services\ndescription: local\n---\nLocal.\n")
	writeFile(t, filepath.Join(local, "skills", "scripts", "SKILL.md"), "---\nname: scripts\ndescription: local\n---\nLocal.\n")
	writeFile(t, filepath.Join(".claude", "skills", "tool", "SKILL.md"), "---\nname: tool\ndescription: shared\n---\nTool.\n")
	writeFile(t, filepath.Join(".claude", "skills", "tool", "scripts", "run.sh"), "echo run\n")
	writeFile(t, filepath.Join("services", "api", ".claude", "skills", "review", "SKILL.md"), "---\nname: review\ndescription: scoped\n---\nReview.\n")

	importCapturing(t, "claude")
	importCapturing(t, "opencode")

	for _, path := range []string{
		filepath.Join(".agnostic-ai", "skills", "tool", "scripts", "run.sh"),
		filepath.Join(".agnostic-ai", "skills", "services", "api", "review", "SKILL.md"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("%s not imported: %v", path, err)
		}
	}
}

// Settings, reviews, and environments merge into one native file that
// keeps no spec names, so a local spec of that kind keeps the import from
// touching the shared specs of that kind at all.
func TestImport_KeepsSharedAggregateSpecsWhenTheLocalLayerFeedsThem(t *testing.T) {
	cases := []struct {
		source, kind string
	}{
		{"claude", "settings"},
		{"codex", "settings"},
		{"copilot", "settings"},
		{"goose", "reviews"},
		{"openhands", "environments"},
	}
	shared := map[string]string{
		filepath.Join(".agnostic-ai", "settings", "base.yaml"):     "permissions:\n  allow:\n    - Bash(go test:*)\n",
		filepath.Join(".agnostic-ai", "reviews", "base.md"):        "Shared review line.\n",
		filepath.Join(".agnostic-ai", "environments", "base.yaml"): "install: echo shared-install\n",
	}
	for _, tc := range cases {
		t.Run(tc.source, func(t *testing.T) {
			testutil.TempCwd(t)
			writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: ["+tc.source+"]\n")
			writeAgnosticFile(t, "# Shared\n")
			for path, body := range shared {
				writeFile(t, path, body)
			}
			local := filepath.FromSlash(defaultProjectUser)
			writeFile(t, filepath.Join(local, "settings", "personal.yaml"), "model: local-model\npermissions:\n  allow:\n    - Bash(local-make:*)\n")
			writeFile(t, filepath.Join(local, "reviews", "mine.md"), "Local review line.\n")
			writeFile(t, filepath.Join(local, "environments", "mine.yaml"), "install: echo local-install\n")
			if out, err := runCLI(t, "sync"); err != nil {
				t.Fatalf("sync: %v\n%s", err, out)
			}

			out := importCapturing(t, tc.source)

			for path, body := range shared {
				assertFileEquals(t, path, body)
			}
			entries, err := os.ReadDir(filepath.Join(".agnostic-ai", tc.kind))
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 1 {
				t.Errorf("want only the shared %s spec, got %d files", tc.kind, len(entries))
			}
			assertNoLocalContentShared(t)
			if !strings.Contains(out, tc.kind) {
				t.Errorf("want the note to name %s:\n%s", tc.kind, out)
			}
		})
	}
}

// assertNoLocalContentShared fails when a file under .agnostic-ai/,
// outside the local layer, holds text only a local spec carries.
func assertNoLocalContentShared(t *testing.T) {
	t.Helper()
	local := filepath.FromSlash(defaultProjectUser)
	err := filepath.WalkDir(".agnostic-ai", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path == local {
				return filepath.SkipDir
			}
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), "local-") {
			t.Errorf("%s holds local content:\n%s", path, data)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Importers fold the fields of every handler in a native group onto one
// spec, so a local handler must leave before that, not after.
func TestImport_SharedHookKeepsNoFieldOfALocalHandlerInItsGroup(t *testing.T) {
	for _, source := range []string{"claude", "factory", "goose", "openhands", "trae", "windsurf", "kiro"} {
		t.Run(source, func(t *testing.T) {
			testutil.TempCwd(t)
			writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: ["+source+"]\n")
			writeAgnosticFile(t, "# Shared\n")
			writeFile(t, filepath.Join(".agnostic-ai", "hooks", "start.yaml"), "name: start\nevent: SessionStart\ncommand: echo shared-start\n")
			local := filepath.FromSlash(defaultProjectUser)
			writeFile(t, filepath.Join(local, "hooks", "zlocal.yaml"),
				"name: zlocal\nevent: SessionStart\ncommand: echo local-start\ntimeout: 99\nasync: true\nshell: powershell\nstatusMessage: local-msg\n")
			if out, err := runCLI(t, "sync"); err != nil {
				t.Fatalf("sync: %v\n%s", err, out)
			}

			out := importCapturing(t, source)

			for name, data := range sharedHookFiles(t) {
				if name == "start.yaml" {
					continue
				}
				for _, field := range []string{"local-", "99", "async", "powershell"} {
					if strings.Contains(data, field) {
						t.Errorf("%s carries %q from the local handler:\n%s", name, field, data)
					}
				}
			}
			if !strings.Contains(out, "hook zlocal") {
				t.Errorf("want the note to name the local hook:\n%s", out)
			}
		})
	}
}

// Sync rewrites a hook script path to each target's hooks directory and
// applies x-<target> overrides, so a local hook must still match there.
func TestImport_MatchesLocalHooksAsEachTargetRendersThem(t *testing.T) {
	cases := []struct {
		source, hook string
	}{
		{"codex", "name: guard\nevent: PreToolUse\nmatcher: Bash\ncommand: bash .claude/hooks/local-guard.sh\n"},
		{"gemini", "name: guard\nevent: BeforeTool\ncommand: [echo local-a, echo local-b]\nx-gemini:\n  matcher: run_shell_command\n"},
		{"gemini", "name: guard\nevent: BeforeTool\ncommand: echo base\nx-gemini:\n  command: bash .codex/hooks/local-guard.sh\n"},
	}
	for _, tc := range cases {
		t.Run(tc.source, func(t *testing.T) {
			testutil.TempCwd(t)
			writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, "+tc.source+"]\n")
			writeAgnosticFile(t, "# Shared\n")
			writeFile(t, filepath.Join(defaultProjectUser, "hooks", "guard.yaml"), tc.hook)
			if out, err := runCLI(t, "sync"); err != nil {
				t.Fatalf("sync: %v\n%s", err, out)
			}

			out := importCapturing(t, tc.source)

			assertNoLocalContentShared(t)
			if !strings.Contains(out, "hook guard") {
				t.Errorf("want the note to name the local hook:\n%s", out)
			}
		})
	}
}

// Codex respells a matcher on emit and joins the matchers of every spec
// that runs the same command, so a local hook must still match the
// matcher the native file carries.
func TestImport_MatchesLocalHooksUnderTheMatcherCodexWrites(t *testing.T) {
	cases := []struct {
		name, shared string
		locals       map[string]string
	}{
		{"matcher order",
			"name: edit\nevent: PreToolUse\nmatcher: Edit|Write\ncommand: echo shared-edit\n",
			map[string]string{"zguard": "name: zguard\nevent: PreToolUse\nmatcher: Write|Edit\ncommand: echo local-guard\nstatusMessage: local-msg\n"}},
		{"matcher spacing",
			"name: edit\nevent: PreToolUse\nmatcher: Bash|Edit\ncommand: echo shared-edit\n",
			map[string]string{"zguard": "name: zguard\nevent: PreToolUse\nmatcher: Bash | Edit\ncommand: echo local-guard\n"}},
		{"two local hooks share a command",
			"name: edit\nevent: PreToolUse\nmatcher: Edit\ncommand: echo shared-edit\n",
			map[string]string{
				"zguard":  "name: zguard\nevent: PreToolUse\nmatcher: Write\ncommand: echo local-guard\n",
				"zguard2": "name: zguard2\nevent: PreToolUse\nmatcher: Edit\ncommand: echo local-guard\n",
			}},
		{"a local hook shares a command with a shared one",
			"name: edit\nevent: PreToolUse\nmatcher: Edit\ncommand: echo shared-edit\n",
			map[string]string{"zguard": "name: zguard\nevent: PreToolUse\nmatcher: mcp__local-tool\ncommand: echo shared-edit\n"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			testutil.TempCwd(t)
			writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [codex]\n")
			writeAgnosticFile(t, "# Shared\n")
			writeFile(t, filepath.Join(".agnostic-ai", "hooks", "edit.yaml"), tc.shared)
			for name, local := range tc.locals {
				writeFile(t, filepath.Join(defaultProjectUser, "hooks", name+".yaml"), local)
			}
			if out, err := runCLI(t, "sync"); err != nil {
				t.Fatalf("sync: %v\n%s", err, out)
			}

			out := importCapturing(t, "codex")

			assertNoLocalContentShared(t)
			if !strings.Contains(out, "hook zguard") {
				t.Errorf("want the note to name the local hook:\n%s", out)
			}
		})
	}
}

// A hook script only local hooks run stays out of the scripts stash; a
// script a shared hook runs is still captured.
func TestImport_LeavesLocalHookScriptsOutOfTheSharedSource(t *testing.T) {
	testutil.TempCwd(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	writeAgnosticFile(t, "# Shared\n")
	writeFile(t, filepath.Join(".agnostic-ai", "hooks", "lint.yaml"), "name: lint\nevent: PostToolUse\ncommand: bash .claude/hooks/lint.sh\n")
	writeFile(t, filepath.Join(defaultProjectUser, "hooks", "guard.yaml"), "name: guard\nevent: PreToolUse\nmatcher: Bash\ncommand: bash .claude/hooks/guard.sh\n")
	writeFile(t, filepath.Join(".claude", "hooks", "guard.sh"), "echo local-secret\n")
	writeFile(t, filepath.Join(".claude", "hooks", "lint.sh"), "echo lint\n")
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}

	importCapturing(t, "claude")

	assertNotExist(t, filepath.Join(".agnostic-ai", "scripts", "claude", "guard.sh"))
	if _, err := os.Stat(filepath.Join(".agnostic-ai", "scripts", "claude", "lint.sh")); err != nil {
		t.Errorf("shared hook script not captured: %v", err)
	}
	assertNoLocalContentShared(t)
}

func TestImport_HookEventOrderLeavesEventsOnlyLocalHooksFeed(t *testing.T) {
	syncWithLocalHooks(t)

	importCapturing(t, "claude")

	data, err := os.ReadFile(filepath.Join(".agnostic-ai", "overlays", "claude.settings.hook-events.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "PreToolUse") {
		t.Errorf("captured an event only a local hook feeds:\n%s", data)
	}
	if !strings.Contains(string(data), "SessionStart") {
		t.Errorf("dropped the event a shared hook feeds:\n%s", data)
	}
}
