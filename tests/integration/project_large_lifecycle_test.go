package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
)

func TestProjectLifecycle_CloneAndWorktreeRestoreOutputsAndValidateTheIndex(t *testing.T) {
	repository, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	name := "agnostic-ai"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	binary := filepath.Join(t.TempDir(), name)
	// High fixture versions identify candidate builds, not published releases.
	build := exec.Command("go", "build", "-ldflags", "-X main.version=99.1.0 -X github.com/chemaclass/agnostic-ai/internal/cli.candidateVersion=99.1.0", "-o", binary, "./cmd/agnostic-ai")
	build.Dir = repository
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v\n%s", err, out)
	}
	t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
	t.Setenv("CODEX_HOME", t.TempDir())
	t.Setenv("AGNOSTIC_AI_NO_UPDATE_CHECK", "1")
	t.Setenv("AGNOSTIC_AI_TARGET", "")
	t.Setenv("AGNOSTIC_AI_PROJECT_BOOTSTRAP", "")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_COUNT", "0")
	gitConfig := filepath.Join(t.TempDir(), "gitconfig")
	mustWrite(t, gitConfig, "[user]\n name = lifecycle\n email = lifecycle@example.invalid\n[commit]\n gpgsign = false\n[core]\n autocrlf = false\n")
	t.Setenv("GIT_CONFIG_GLOBAL", gitConfig)
	emptyHooks := t.TempDir()

	run := func(dir, command string, args ...string) (string, error) {
		t.Helper()
		if command == "git" {
			args = append([]string{"-c", "core.hooksPath=" + emptyHooks}, args...)
		}
		cmd := exec.Command(command, args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	git := func(dir string, args ...string) string {
		t.Helper()
		out, err := run(dir, "git", args...)
		if err != nil {
			t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
		}
		return out
	}
	cli := func(dir string, args ...string) string {
		t.Helper()
		out, err := run(dir, binary, args...)
		if err != nil {
			t.Fatalf("agnostic-ai %v in %s: %v\n%s", args, dir, err, out)
		}
		return out
	}
	status := func(dir, want string) {
		t.Helper()
		if got := git(dir, "status", "--porcelain=v1", "--untracked-files=all"); got != want {
			t.Errorf("Git status in %s:\n%s\nwant:\n%s", dir, got, want)
		}
	}

	source := setupFixture(t)
	mustWrite(t, filepath.Join(source, "agnostic-ai.yaml"), "version: 1\ntargets: [claude, codex, cursor]\ngitignore:\n  enabled: true\n  commit: [instructions, hooks]\n")
	const scopedSource = ".agnostic-ai/rules/api.md"
	const scopedBody = "Version every route.\n"
	const scopedPrefix = "---\nname: api\ndescription: API conventions.\nscope: services/api\n---\n"
	mustWrite(t, filepath.Join(source, scopedSource), scopedPrefix+scopedBody)
	mustWrite(t, filepath.Join(source, "services/api/main.go"), "package api\n")
	for i := 0; i < 495; i++ {
		skill := fmt.Sprintf("catalog-%04d", i)
		mustWrite(t, filepath.Join(source, ".agnostic-ai/skills", skill, "SKILL.md"),
			fmt.Sprintf("---\nname: %s\ndescription: Read catalog guidance %d.\n---\nRead the catalog guidance.\n", skill, i))
	}
	const assetSource = ".agnostic-ai/skills/catalog-0000/references/guide.md"
	const asset = "Catalog reference for the initial branch.\n"
	mustWrite(t, filepath.Join(source, assetSource), asset)
	mustWrite(t, filepath.Join(source, "CLAUDE.md"), "# Existing project\n\nKeep the public API stable.\n")
	mustWrite(t, filepath.Join(source, ".claude/rules/adopted.md"), "---\nname: adopted\ndescription: Adopted native rule.\n---\nRetain the existing convention.\n")
	git(source, "init", "-q", "-b", "main")
	git(source, "add", "-A")
	git(source, "commit", "-qm", "native project and portable catalog")
	clone := filepath.Join(t.TempDir(), "fresh clone with spaces")
	git(source, "clone", "-q", "--no-local", source, clone)
	status(clone, "")
	if out, err := run(clone, binary, "sync"); err == nil || !strings.Contains(out, "import claude") {
		t.Fatalf("native instructions must stop sync with adoption advice: %v\n%s", err, out)
	}
	assertContains(t, filepath.Join(clone, "CLAUDE.md"), "Keep the public API stable.")
	status(clone, "?? .gitignore\n")
	assertContains(t, filepath.Join(clone, ".gitignore"), "agnostic-ai")
	cli(clone, "import", "claude")
	if out := cli(clone, "validate"); !strings.Contains(out, "loaded 500 entries") {
		t.Errorf("fixture must contain 500 specs after adoption:\n%s", out)
	}
	cli(clone, "sync")
	assertContains(t, filepath.Join(clone, ".agnostic-ai/rules/adopted.md"), "Retain the existing convention.")
	assertContains(t, filepath.Join(clone, "services/api/AGENTS.md"), scopedBody)
	for _, output := range []string{"CLAUDE.md", "AGENTS.md", ".claude/settings.json", ".codex/hooks.json", ".cursor/hooks.json"} {
		if info, err := os.Stat(filepath.Join(clone, output)); err != nil || !info.Mode().IsRegular() {
			t.Fatalf("setup missing %s: %v", output, err)
		}
	}
	for _, output := range []string{"CLAUDE.md", "services/api/AGENTS.md", ".claude/settings.json"} {
		cmd := exec.Command("git", "check-ignore", "--no-index", "-q", output)
		cmd.Dir = clone
		if err := cmd.Run(); err == nil {
			t.Errorf("selected committed output is ignored: %s", output)
		} else if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 1 {
			t.Fatalf("check-ignore %s: %v", output, err)
		}
	}
	const skillOutput = ".claude/skills/catalog-0000/SKILL.md"
	const assetOutput = ".claude/skills/catalog-0000/references/guide.md"
	if out, err := run(clone, "git", "check-ignore", "--no-index", assetOutput); err != nil || !strings.Contains(out, assetOutput) {
		t.Fatalf("skill assets must be ignored: %v\n%s", err, out)
	}
	assertLifecycleIntentionalStatus(t, git(clone, "status", "--porcelain=v1", "--untracked-files=all"))
	git(clone, "add", "-A")
	git(clone, "commit", "-qm", "adopt and commit selected outputs")
	status(clone, "")
	for _, skillsDir := range []string{".claude/skills", ".agents/skills", ".cursor/skills"} {
		for i := 0; i < 495; i++ {
			skill := fmt.Sprintf("catalog-%04d", i)
			path := filepath.Join(clone, skillsDir, skill, "SKILL.md")
			info, err := os.Stat(path)
			if err != nil || !info.Mode().IsRegular() {
				t.Errorf("catalog skill missing from %s: %v", path, err)
				continue
			}
			assertContains(t, path, "name: "+skill)
			assertContains(t, path, "Read the catalog guidance.")
		}
		path := filepath.Join(clone, skillsDir, "catalog-0000", "references", "guide.md")
		copied, err := os.ReadFile(path)
		if err != nil || string(copied) != asset {
			t.Errorf("catalog asset in %s: %v, got %q", path, err, copied)
		}
	}
	initial := lifecycleOutputs(t, clone)
	if string(initial[assetOutput]) != asset {
		t.Errorf("copied asset = %q, want %q", initial[assetOutput], asset)
	}
	for i := 0; i < 2; i++ {
		cli(clone, "sync")
		assertLifecycleOutputs(t, clone, initial)
		status(clone, "")
	}
	cli(clone, "sync", "--check", "--against", "index")
	initialCommit := strings.TrimSpace(git(clone, "rev-parse", "HEAD"))
	linked := filepath.Join(t.TempDir(), "linked worktree with spaces")
	git(clone, "worktree", "add", "--detach", linked, initialCommit)
	if _, err := os.Stat(filepath.Join(linked, assetOutput)); !os.IsNotExist(err) {
		t.Fatalf("fresh worktree unexpectedly has ignored asset: %v", err)
	}
	cli(linked, "sync", "--keep-edits")
	assertLifecycleOutputs(t, linked, initial)
	status(linked, "")
	cli(linked, "sync")
	assertLifecycleOutputs(t, linked, initial)
	status(linked, "")

	t.Run("project doctor checks clone and linked worktree without repairs or trust", func(t *testing.T) {
		const driftPath = ".cursor/hooks.json"
		for _, dir := range []string{clone, linked} {
			for _, drift := range []bool{false, true} {
				if drift {
					mustWrite(t, filepath.Join(dir, driftPath), "{\"edited\":true}\n")
				}
				beforeFiles := lifecycleFiles(t, dir)
				beforeOutputs := lifecycleOutputs(t, dir)
				beforeStatus := git(dir, "status", "--porcelain=v1", "--untracked-files=all")
				index := strings.TrimSpace(git(dir, "rev-parse", "--git-path", "index"))
				if !filepath.IsAbs(index) {
					index = filepath.Join(dir, index)
				}
				beforeIndex, err := os.ReadFile(index)
				if err != nil {
					t.Fatal(err)
				}
				for _, asJSON := range []bool{false, true} {
					args := []string{"doctor", "--scope", "project"}
					if asJSON {
						args = append(args, "--json")
					}
					cmd := exec.Command(binary, args...)
					cmd.Dir = dir
					raw, err := cmd.Output()
					if (err != nil) != drift {
						t.Errorf("project doctor with drift=%t: %v\n%s", drift, err, raw)
					}
					if asJSON {
						var report struct {
							HookTrustCheck struct{ Status, Reason string } `json:"hook_trust_check"`
							HookTrust      []json.RawMessage               `json:"hook_trust"`
							Writes         []struct{ Path, Action string } `json:"writes"`
						}
						if err := json.Unmarshal(raw, &report); err != nil {
							t.Fatalf("decode project doctor: %v\n%s", err, raw)
						}
						if report.HookTrustCheck.Status != "skipped" || !strings.Contains(report.HookTrustCheck.Reason, "project scope") || report.HookTrust == nil || len(report.HookTrust) != 0 {
							t.Errorf("project doctor must explicitly skip local trust: %s", raw)
						}
						if !drift && (report.Writes == nil || len(report.Writes) != 0) {
							t.Errorf("clean project doctor reports writes: %s", raw)
						}
						if drift {
							found := false
							for _, write := range report.Writes {
								if write.Path == driftPath && write.Action == "edited" {
									found = true
								}
							}
							if !found {
								t.Errorf("project doctor misses generated drift: %s", raw)
							}
						}
					} else if !strings.Contains(string(raw), "SKIPPED") || !strings.Contains(string(raw), "Codex hook trust") || !strings.Contains(string(raw), "project scope") || (drift && !strings.Contains(string(raw), driftPath)) {
						t.Errorf("project doctor lacks skipped trust or drift details: %s", raw)
					}
					afterIndex, err := os.ReadFile(index)
					if err != nil {
						t.Fatal(err)
					}
					if !bytes.Equal(beforeIndex, afterIndex) || !reflect.DeepEqual(beforeFiles, lifecycleFiles(t, dir)) {
						t.Error("project doctor changed project files or the index")
					}
					assertLifecycleOutputs(t, dir, beforeOutputs)
					status(dir, beforeStatus)
					entries, err := os.ReadDir(os.Getenv("CODEX_HOME"))
					if err != nil || len(entries) != 0 {
						t.Errorf("project doctor populated local hook trust: %v, entries=%v", err, entries)
					}
				}
			}
			mustWrite(t, filepath.Join(dir, driftPath), string(initial[driftPath]))
			assertLifecycleOutputs(t, dir, initial)
			status(dir, "")
		}
	})

	t.Run("exact package pin changes across clone and linked worktree", func(t *testing.T) {
		matching := filepath.Join(t.TempDir(), name)
		build := exec.Command("go", "build", "-ldflags", "-X main.version=99.2.0 -X github.com/chemaclass/agnostic-ai/internal/cli.candidateVersion=99.2.0", "-o", matching, "./cmd/agnostic-ai")
		build.Dir = repository
		if out, err := build.CombinedOutput(); err != nil {
			t.Fatalf("build matching fixture candidate: %v\n%s", err, out)
		}
		helper, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		installerDir := t.TempDir()
		installer := filepath.Join(installerDir, "pnpm")
		launcher := "#!/bin/sh\nexec \"$AGNOSTIC_LIFECYCLE_TEST_EXECUTABLE\" -test.run=^TestProjectLifecycleInstallerProcess$ -- \"$@\"\n"
		if runtime.GOOS == "windows" {
			installer += ".cmd"
			launcher = "@echo off\r\n\"%AGNOSTIC_LIFECYCLE_TEST_EXECUTABLE%\" -test.run=^TestProjectLifecycleInstallerProcess$ -- %*\r\nexit /b %errorlevel%\r\n"
		}
		if err := os.WriteFile(installer, []byte(launcher), 0o755); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PATH", installerDir+string(os.PathListSeparator)+os.Getenv("PATH"))
		t.Setenv("AGNOSTIC_LIFECYCLE_INSTALLER", "1")
		t.Setenv("AGNOSTIC_LIFECYCLE_TEST_EXECUTABLE", helper)
		t.Setenv("AGNOSTIC_LIFECYCLE_OLD_BINARY", binary)
		t.Setenv("AGNOSTIC_LIFECYCLE_NEW_BINARY", matching)
		installs := func(dir, want string) {
			t.Helper()
			data, err := os.ReadFile(filepath.Join(dir, "node_modules", "lifecycle-installs.log"))
			if os.IsNotExist(err) && want == "" {
				return
			}
			if err != nil || string(data) != want {
				t.Errorf("locked install calls: %v\n%s\nwant:\n%s", err, data, want)
			}
		}
		unchanged := func(dir string, check func()) {
			t.Helper()
			beforeOutputs := lifecycleOutputs(t, dir)
			beforeStatus := git(dir, "status", "--porcelain=v1", "--untracked-files=all")
			index := strings.TrimSpace(git(dir, "rev-parse", "--git-path", "index"))
			if !filepath.IsAbs(index) {
				index = filepath.Join(dir, index)
			}
			beforeIndex, err := os.ReadFile(index)
			if err != nil {
				t.Fatal(err)
			}
			check()
			afterIndex, err := os.ReadFile(index)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(beforeIndex, afterIndex) {
				t.Error("project check changed the index")
			}
			status(dir, beforeStatus)
			assertLifecycleOutputs(t, dir, beforeOutputs)
		}
		git(clone, "checkout", "-qb", "dependency-base")
		configPath := filepath.Join(clone, "agnostic-ai.yaml")
		config, err := os.ReadFile(configPath)
		if err != nil {
			t.Fatal(err)
		}
		mustWrite(t, configPath, "requires: '>=99.1.0 <100.0.0'\n"+string(config))
		ignorePath := filepath.Join(clone, ".gitignore")
		ignore, err := os.ReadFile(ignorePath)
		if err != nil {
			t.Fatal(err)
		}
		mustWrite(t, ignorePath, string(ignore)+"node_modules/\n")
		contractOutputs := cloneLifecycleOutputs(initial)
		contractOutputs[".gitignore"] = []byte(string(ignore) + "node_modules/\n")
		packagePath := filepath.Join(clone, "package.json")
		mustWrite(t, packagePath, `{ "private": true, "packageManager": "pnpm@10.0.0", "devDependencies": { "agnostic-ai": "99.1.0" } }`+"\n")
		mustWrite(t, filepath.Join(clone, "pnpm-lock.yaml"), "lockfileVersion: '9.0'\nfixtureVersion: 99.1.0\n")
		git(clone, "add", "agnostic-ai.yaml", ".gitignore", "package.json", "pnpm-lock.yaml")
		git(clone, "commit", "-qm", "fixture candidate package contract")
		oldCommit := strings.TrimSpace(git(clone, "rev-parse", "HEAD"))
		git(linked, "checkout", "--detach", "-q", oldCommit)
		for _, dir := range []string{clone, linked} {
			unchanged(dir, func() {
				out, err := run(dir, binary, "project", "--check")
				if err == nil || !strings.Contains(out, "missing") || !strings.Contains(out, "agnostic-ai project --bootstrap") {
					t.Errorf("missing local package must explain recovery: %v\n%s", err, out)
				}
			})
			installs(dir, "")
			for i := 0; i < 2; i++ {
				unchanged(dir, func() { cli(dir, "project", "--bootstrap") })
				installs(dir, "99.1.0 install --frozen-lockfile marker=1\n")
			}
			assertLifecycleOutputs(t, dir, contractOutputs)
			status(dir, "")
			unchanged(dir, func() { cli(dir, "project", "--check") })
		}
		git(clone, "checkout", "-qb", "dependency-new")
		mustWrite(t, packagePath, `{ "private": true, "packageManager": "pnpm@10.0.0", "devDependencies": { "agnostic-ai": "99.2.0" } }`+"\n")
		mustWrite(t, filepath.Join(clone, "pnpm-lock.yaml"), "lockfileVersion: '9.0'\nfixtureVersion: 99.2.0\n")
		git(clone, "add", "package.json", "pnpm-lock.yaml")
		git(clone, "commit", "-qm", "change fixture candidate package pin")
		newCommit := strings.TrimSpace(git(clone, "rev-parse", "HEAD"))
		git(linked, "checkout", "--detach", "-q", newCommit)
		for _, dir := range []string{clone, linked} {
			assertLifecycleOutputs(t, dir, contractOutputs)
			status(dir, "")
			unchanged(dir, func() {
				out, err := run(dir, binary, "project", "--check")
				if err == nil || !strings.Contains(out, "package.json pins agnostic-ai to 99.2.0") || !strings.Contains(out, "is 99.1.0") || !strings.Contains(out, "agnostic-ai project --bootstrap") {
					t.Errorf("stale local package must fail with recovery: %v\n%s", err, out)
				}
				if _, err := os.Stat(filepath.Join(dir, "package-lock.json")); !os.IsNotExist(err) {
					t.Errorf("check installed dependencies: %v", err)
				}
			})
			installs(dir, "99.1.0 install --frozen-lockfile marker=1\n")
			for i := 0; i < 2; i++ {
				unchanged(dir, func() { cli(dir, "project", "--bootstrap") })
				installs(dir, "99.1.0 install --frozen-lockfile marker=1\n99.2.0 install --frozen-lockfile marker=1\n")
				unchanged(dir, func() { cli(dir, "project", "--check") })
			}
			if out := cli(dir, "project", "--", "--version"); !strings.Contains(out, "99.2.0") {
				t.Errorf("project did not select the installed candidate: %s", out)
			}
			// Keep the installed fixture ignored when returning to the original branch.
			exclude := strings.TrimSpace(git(dir, "rev-parse", "--git-path", "info/exclude"))
			if !filepath.IsAbs(exclude) {
				exclude = filepath.Join(dir, exclude)
			}
			existing, err := os.ReadFile(exclude)
			if err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			mustWrite(t, exclude, string(existing)+"\nnode_modules/\n")
		}
		git(clone, "checkout", "-q", "main")
		git(linked, "checkout", "--detach", "-q", initialCommit)
		assertLifecycleOutputs(t, clone, initial)
		assertLifecycleOutputs(t, linked, initial)
		status(clone, "")
		status(linked, "")
	})

	t.Run("migration plans after native and canonical branch changes", func(t *testing.T) {
		migration := filepath.Join(t.TempDir(), "migration clone with spaces")
		git(clone, "clone", "-q", "--no-local", clone, migration)
		git(migration, "checkout", "--detach", "-q", initialCommit)
		cli(migration, "sync", "--keep-edits")
		assertLifecycleOutputs(t, migration, initial)
		status(migration, "")
		for _, skill := range []string{"migration-edit", "migration-delete", "migration-conflict"} {
			mustWrite(t, filepath.Join(migration, "native", skill, "SKILL.md"),
				fmt.Sprintf("---\nname: %s\ndescription: Migration fixture.\n---\nOriginal guidance.\n", skill))
		}
		git(migration, "add", "native")
		git(migration, "commit", "-qm", "native skills before migration")
		base := strings.TrimSpace(git(migration, "rev-parse", "HEAD"))
		for _, skill := range []string{"migration-edit", "migration-delete", "migration-conflict"} {
			data, err := os.ReadFile(filepath.Join(migration, "native", skill, "SKILL.md"))
			if err != nil {
				t.Fatal(err)
			}
			mustWrite(t, filepath.Join(migration, ".agnostic-ai/skills", skill, "SKILL.md"), string(data))
		}
		git(migration, "rm", "-qr", "native")
		cli(migration, "sync")
		git(migration, "add", "-A")
		git(migration, "commit", "-qm", "move native skills to canonical sources")
		migrated := strings.TrimSpace(git(migration, "rev-parse", "HEAD"))
		git(migration, "checkout", "--detach", "-q", base)
		for _, skill := range []string{"migration-edit", "migration-add", "migration-conflict"} {
			mustWrite(t, filepath.Join(migration, "native", skill, "SKILL.md"),
				fmt.Sprintf("---\nname: %s\ndescription: Migration fixture.\n---\nNative branch guidance.\n", skill))
		}
		git(migration, "rm", "-qr", "native/migration-delete")
		git(migration, "add", "native")
		git(migration, "commit", "-qm", "edit add and delete native skills")
		upstream := strings.TrimSpace(git(migration, "rev-parse", "HEAD"))
		git(migration, "checkout", "--detach", "-q", migrated)
		mustWrite(t, filepath.Join(migration, ".agnostic-ai/skills/migration-conflict/SKILL.md"),
			"---\nname: migration-conflict\ndescription: Migration fixture.\n---\nCanonical branch guidance.\n")
		cli(migration, "sync")
		git(migration, "add", "-A")
		git(migration, "commit", "-qm", "change canonical guidance independently")
		current := strings.TrimSpace(git(migration, "rev-parse", "HEAD"))
		migrationLinked := filepath.Join(t.TempDir(), "migration linked worktree with spaces")
		git(migration, "worktree", "add", "--detach", migrationLinked, current)
		cli(migrationLinked, "sync", "--keep-edits")
		assertLifecycleOutputs(t, migrationLinked, lifecycleOutputs(t, migration))
		for _, dir := range []string{migration, migrationLinked} {
			status(dir, "")
			mustWrite(t, filepath.Join(dir, "staged-note.txt"), "Keep this staged note.\n")
			git(dir, "add", "staged-note.txt")
			beforeStatus := git(dir, "status", "--porcelain=v1", "--untracked-files=all")
			if beforeStatus != "A  staged-note.txt\n" {
				t.Fatalf("migration setup changed unrelated paths: %s", beforeStatus)
			}
			beforeFiles := lifecycleFiles(t, dir)
			index := strings.TrimSpace(git(dir, "rev-parse", "--git-path", "index"))
			if !filepath.IsAbs(index) {
				index = filepath.Join(dir, index)
			}
			beforeIndex, err := os.ReadFile(index)
			if err != nil {
				t.Fatal(err)
			}
			args := []string{"import", "reconcile", "--base", base, "--migrated", migrated, "--upstream", upstream, "--map", "native=.agnostic-ai/skills"}
			want := map[string]string{"migration-edit": "update", "migration-add": "add", "migration-delete": "remove", "migration-conflict": "conflict"}
			for _, asJSON := range []bool{false, true} {
				commandArgs := append([]string(nil), args...)
				if asJSON {
					commandArgs = append(commandArgs, "--json")
				}
				out := cli(dir, commandArgs...)
				if asJSON {
					var plan struct {
						Base, Migrated, Upstream, Current string
						Entries                           []struct{ Action, Source, Destination string }
					}
					if err := json.Unmarshal([]byte(out), &plan); err != nil {
						t.Fatalf("decode migration plan: %v\n%s", err, out)
					}
					if plan.Base != base || plan.Migrated != migrated || plan.Upstream != upstream || plan.Current != current || len(plan.Entries) != len(want) {
						t.Errorf("unexpected migration revisions or entries: %s", out)
					}
					seen := map[string]bool{}
					for _, entry := range plan.Entries {
						skill := strings.TrimSuffix(strings.TrimPrefix(entry.Source, "native/"), "/SKILL.md")
						if action, ok := want[skill]; !ok || seen[skill] || entry.Action != action || entry.Source != "native/"+skill+"/SKILL.md" || entry.Destination != ".agnostic-ai/skills/"+skill+"/SKILL.md" {
							t.Errorf("unexpected migration entry: %+v", entry)
						}
						seen[skill] = true
					}
				} else {
					for skill, action := range want {
						if !strings.Contains(out, action+"\tnative/"+skill+"/SKILL.md\t.agnostic-ai/skills/"+skill+"/SKILL.md") {
							t.Errorf("text migration plan misses %s: %s", skill, out)
						}
					}
				}
				afterIndex, err := os.ReadFile(index)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(beforeIndex, afterIndex) {
					t.Error("migration planning changed actual index bytes")
				}
				status(dir, beforeStatus)
				if !reflect.DeepEqual(beforeFiles, lifecycleFiles(t, dir)) {
					t.Error("migration planning changed project file bytes")
				}
			}
		}
	})

	git(clone, "checkout", "-qb", "alternate")
	const alternateAsset = "Catalog reference for the alternate branch.\n"
	mustWrite(t, filepath.Join(clone, assetSource), alternateAsset)
	mustWrite(t, filepath.Join(clone, scopedSource), scopedPrefix+"Document every route.\n")
	cli(clone, "sync")
	alternate := lifecycleOutputs(t, clone)
	if string(alternate[assetOutput]) != alternateAsset || bytes.Equal(alternate["services/api/AGENTS.md"], initial["services/api/AGENTS.md"]) {
		t.Fatal("alternate branch did not change its asset and scoped output")
	}
	assertLifecycleIntentionalStatus(t, git(clone, "status", "--porcelain=v1", "--untracked-files=all"))
	git(clone, "add", "-A")
	git(clone, "commit", "-qm", "alternate guidance and asset")
	status(clone, "")
	for _, branch := range []string{"main", "alternate", "main"} {
		git(clone, "checkout", "-q", branch)
		cli(clone, "sync", "--keep-edits")
		want := initial
		if branch == "alternate" {
			want = alternate
		}
		assertLifecycleOutputs(t, clone, want)
		status(clone, "")
	}
	assertLifecycleOutputs(t, linked, initial)
	status(linked, "")

	const editedOutput = "services/api/AGENTS.md"
	manual := []byte("Manual scoped instructions.\n")
	mustWrite(t, filepath.Join(linked, editedOutput), string(manual))
	manualSkill := append(bytes.Clone(initial[skillOutput]), []byte("Local skill note.\n")...)
	mustWrite(t, filepath.Join(linked, skillOutput), string(manualSkill))
	beforeUnknown := lifecycleOutputs(t, linked)
	indexPath := strings.TrimSpace(git(linked, "rev-parse", "--git-path", "index"))
	if !filepath.IsAbs(indexPath) {
		indexPath = filepath.Join(linked, indexPath)
	}
	unknownIndex, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	if out, err := run(linked, binary, "sync", "--keep-edits"); err == nil || !strings.Contains(out, "Manual scoped instructions.") || !strings.Contains(out, "is in no spec") || !strings.Contains(out, ".agnostic-ai/") {
		t.Fatalf("unknown manual instructions must stop sync with recovery: %v\n%s", err, out)
	}
	assertLifecycleOutputs(t, linked, beforeUnknown)
	afterUnknownIndex, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(unknownIndex, afterUnknownIndex) {
		t.Error("unknown manual instructions failure changed the index")
	}
	status(linked, " M "+editedOutput+"\n")
	manual = append(bytes.Clone(initial[editedOutput]), manual...)
	mustWrite(t, filepath.Join(linked, editedOutput), string(manual))
	cli(linked, "sync", "--keep-edits")
	kept := lifecycleOutputs(t, linked)
	wantKept := cloneLifecycleOutputs(initial)
	wantKept[editedOutput], wantKept[skillOutput] = manual, manualSkill
	if !reflect.DeepEqual(kept, wantKept) {
		t.Error("keep-edits changed outputs beyond the two manual edits")
	}
	status(linked, " M "+editedOutput+"\n")
	check := exec.Command(binary, "sync", "--check", "--json")
	check.Dir = linked
	raw, err := check.Output()
	out := string(raw)
	if err == nil {
		t.Fatal("check accepted preserved manual output drift")
	}
	var drift struct {
		Writes []struct{ Path, Action string }
	}
	if err := json.Unmarshal([]byte(out), &drift); err != nil {
		t.Fatalf("decode drift: %v\n%s", err, out)
	}
	for _, path := range []string{editedOutput, skillOutput} {
		found := false
		for _, write := range drift.Writes {
			if write.Path == path && write.Action == "edited" {
				found = true
			}
		}
		if !found {
			t.Errorf("drift does not identify edited %s:\n%s", path, out)
		}
	}
	assertLifecycleOutputs(t, linked, wantKept)
	status(linked, " M "+editedOutput+"\n")
	recovery := cli(linked, "sync")
	if !strings.Contains(recovery, ".bak") || !strings.Contains(recovery, "hand edit") {
		t.Errorf("recovery must explain preserved backups:\n%s", recovery)
	}
	for path, want := range map[string][]byte{editedOutput: manual, skillOutput: manualSkill} {
		backup, err := os.ReadFile(filepath.Join(linked, path+".bak"))
		if err != nil || !bytes.Equal(backup, want) {
			t.Errorf("recovery did not retain %s: %v, bytes=%q", path, err, backup)
		}
		if err := os.Remove(filepath.Join(linked, path+".bak")); err != nil {
			t.Fatal(err)
		}
	}
	assertLifecycleOutputs(t, linked, initial)
	status(linked, "")
	cli(linked, "sync", "--check")

	mustWrite(t, filepath.Join(clone, scopedSource), scopedPrefix+"Check every route.\n")
	git(clone, "add", "--", scopedSource)
	cli(clone, "sync")
	cli(clone, "sync", "--check")
	before := lifecycleOutputs(t, clone)
	beforeStatus := git(clone, "status", "--porcelain=v1", "--untracked-files=all")
	assertLifecycleIntentionalStatus(t, beforeStatus)
	beforeIndex := git(clone, "ls-files", "--stage", "-z")
	out, err = run(clone, binary, "sync", "--check", "--against", "index")
	if err == nil || !strings.Contains(out, "Git index") || !strings.Contains(out, "stage") || !strings.Contains(out, editedOutput) {
		t.Errorf("index mismatch lacks actionable recovery and scoped output: %v\n%s", err, out)
	}
	assertLifecycleOutputs(t, clone, before)
	status(clone, beforeStatus)
	if after := git(clone, "ls-files", "--stage", "-z"); after != beforeIndex {
		t.Error("index validation changed the staged index")
	}
	git(clone, "add", "-A")
	cli(clone, "sync", "--check", "--against", "index")
	git(clone, "commit", "-qm", "stage source and matching outputs")
	status(clone, "")
	assertLifecycleOutputs(t, clone, before)
	entries, err := os.ReadDir(os.Getenv("CODEX_HOME"))
	if err != nil || len(entries) != 0 {
		t.Errorf("lifecycle populated local hook trust: %v, entries=%v", err, entries)
	}
}

func lifecycleOutputs(t *testing.T, root string) map[string][]byte {
	t.Helper()
	out := map[string][]byte{}
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if entry.IsDir() {
			if rel == ".git" || rel == ".agnostic-ai" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(rel, ".claude/") || strings.HasPrefix(rel, ".cursor/") || strings.HasPrefix(rel, ".codex/") || strings.HasPrefix(rel, ".agents/") || filepath.Base(path) == "AGENTS.md" || filepath.Base(path) == "CLAUDE.md" || rel == ".gitignore" || rel == ".worktreeinclude" {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			out[rel] = data
		}
		return nil
	}); err != nil {
		t.Fatalf("read lifecycle outputs in %s: %v", root, err)
	}
	return out
}

func cloneLifecycleOutputs(source map[string][]byte) map[string][]byte {
	out := make(map[string][]byte, len(source))
	for path, data := range source {
		out[path] = bytes.Clone(data)
	}
	return out
}

func assertLifecycleOutputs(t *testing.T, root string, want map[string][]byte) {
	t.Helper()
	got := lifecycleOutputs(t, root)
	var different []string
	for path, data := range want {
		if !bytes.Equal(got[path], data) {
			different = append(different, path)
		}
	}
	for path := range got {
		if _, exists := want[path]; !exists {
			different = append(different, path)
		}
	}
	sort.Strings(different)
	if len(different) > 0 {
		t.Errorf("lifecycle outputs differ in %s: %v", root, different)
	}
}

func assertLifecycleIntentionalStatus(t *testing.T, status string) {
	t.Helper()
	for _, line := range strings.Split(strings.TrimSuffix(status, "\n"), "\n") {
		if len(line) < 4 {
			t.Errorf("unexpected status line: %q", line)
			continue
		}
		path := line[3:]
		switch {
		case path == "agnostic-ai.yaml", path == ".gitignore", path == ".worktreeinclude", path == "CLAUDE.md", path == "AGENTS.md":
		case strings.HasPrefix(path, ".agnostic-ai/"):
		case strings.HasPrefix(path, ".claude/rules/"), strings.HasPrefix(path, ".cursor/rules/"):
		case path == ".claude/settings.json", path == ".codex/hooks.json", path == ".cursor/hooks.json":
		case path == "services/api/AGENTS.md", path == "services/api/CLAUDE.md":
		default:
			t.Errorf("lifecycle changed an unintended path: %s", line)
		}
	}
}

func lifecycleFiles(t *testing.T, root string) map[string][]byte {
	t.Helper()
	files := map[string][]byte{}
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == ".git" {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !entry.IsDir() {
			files[filepath.ToSlash(rel)], err = os.ReadFile(path)
		}
		return err
	}); err != nil {
		t.Fatalf("read project files in %s: %v", root, err)
	}
	return files
}

func TestProjectLifecycleInstallerProcess(t *testing.T) {
	if os.Getenv("AGNOSTIC_LIFECYCLE_INSTALLER") != "1" {
		return
	}
	args := os.Args
	for len(args) > 0 && args[0] != "--" {
		args = args[1:]
	}
	if len(args) == 0 || !reflect.DeepEqual(args[1:], []string{"install", "--frozen-lockfile"}) || os.Getenv("AGNOSTIC_AI_PROJECT_BOOTSTRAP") != "1" {
		t.Fatalf("unexpected bootstrap installer invocation: %v", args)
	}
	data, err := os.ReadFile("package.json")
	if err != nil {
		t.Fatal(err)
	}
	var pkg struct{ DevDependencies map[string]string }
	if err := json.Unmarshal(data, &pkg); err != nil {
		t.Fatal(err)
	}
	version := pkg.DevDependencies["agnostic-ai"]
	candidate := map[string]string{"99.1.0": os.Getenv("AGNOSTIC_LIFECYCLE_OLD_BINARY"), "99.2.0": os.Getenv("AGNOSTIC_LIFECYCLE_NEW_BINARY")}[version]
	lock, err := os.ReadFile("pnpm-lock.yaml")
	if err != nil || candidate == "" || string(lock) != "lockfileVersion: '9.0'\nfixtureVersion: "+version+"\n" {
		t.Fatalf("fixture package pin and lock disagree: %v, version %q, lock %q", err, version, lock)
	}
	data, err = os.ReadFile(candidate)
	if err != nil {
		t.Fatal(err)
	}
	local := filepath.Join("node_modules", ".bin", "agnostic-ai")
	if runtime.GOOS == "windows" {
		local = filepath.Join("node_modules", "agnostic-ai", "bin", "fixture.exe")
	}
	if err := os.MkdirAll(filepath.Dir(local), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(local, data, 0o755); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		mustWrite(t, filepath.Join("node_modules", "agnostic-ai", "bin", "agnostic-ai.js"), "const { spawnSync } = require('node:child_process');\nconst path = require('node:path');\nconst result = spawnSync(path.join(__dirname, 'fixture.exe'), process.argv.slice(2), { stdio: 'inherit' });\nif (result.error) throw result.error;\nprocess.exit(result.status === null ? 1 : result.status);\n")
	}
	log, err := os.OpenFile(filepath.Join("node_modules", "lifecycle-installs.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := fmt.Fprintf(log, "%s install --frozen-lockfile marker=1\n", version)
	closeErr := log.Close()
	if writeErr != nil || closeErr != nil {
		t.Fatalf("record install: %v, close: %v", writeErr, closeErr)
	}
}
