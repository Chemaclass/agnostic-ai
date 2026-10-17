package integration

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestImportReconcile_ReportsCommittedChangesWithoutApplying(t *testing.T) {
	packageDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	name := "agnostic-ai"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	binary := filepath.Join(t.TempDir(), name)
	build := exec.Command("go", "build", "-o", binary, "./cmd/agnostic-ai")
	build.Dir = filepath.Join(packageDir, "..", "..")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v\n%s", err, out)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "missing-config"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
	t.Setenv("AGNOSTIC_AI_NO_UPDATE_CHECK", "1")
	for _, c := range []struct {
		name, prefix, action, want string
		shared                     bool
	}{
		{"root update", "", "update", "update", false},
		{"nested update", "apps/demo project", "update", "update", false},
		{"shared owner deletion", "", "remove", "conflict", true},
		{"shared owner update", "", "update", "conflict", true},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			git := func(args ...string) string {
				t.Helper()
				cmd := exec.Command("git", append([]string{"-c", "user.name=Test", "-c", "user.email=test@example.com", "-c", "commit.gpgsign=false"}, args...)...)
				cmd.Dir = dir
				out, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("git %v: %v\n%s", args, err, out)
				}
				return strings.TrimSpace(string(out))
			}
			write := func(name, content string) {
				t.Helper()
				file := filepath.Join(dir, name)
				must(t, os.MkdirAll(filepath.Dir(file), 0o755))
				must(t, os.WriteFile(file, []byte(content), 0o644))
			}
			projectPath := func(relative string) string { return filepath.Join(c.prefix, relative) }
			git("init", "-q")
			write(projectPath("agnostic-ai.yaml"), "version: 1\ntargets: [cursor]\nsources:\n  skills: specs/skills\n")
			write(projectPath("native/example/SKILL.md"), "original")
			if c.shared {
				write("other/example/SKILL.md", "original")
			}
			if c.prefix != "" {
				write("native/root-only/SKILL.md", "root original")
			}
			git("add", ".")
			git("commit", "-qm", "base")
			base := git("rev-parse", "HEAD")
			write(projectPath("specs/skills/example/SKILL.md"), "original")
			git("rm", "-qr", projectPath("native"))
			if c.shared {
				git("rm", "-qr", "other")
			}
			git("add", "-A")
			git("commit", "-qm", "migrate")
			migrated := git("rev-parse", "HEAD")
			git("checkout", "-q", base)
			if c.action == "remove" {
				git("rm", "-q", projectPath("native/example/SKILL.md"))
			} else {
				write(projectPath("native/example/SKILL.md"), "updated")
			}
			if c.prefix != "" {
				write("native/root-only/SKILL.md", "root update")
			}
			git("add", ".")
			git("commit", "-qm", "upstream")
			upstream := git("rev-parse", "HEAD")
			git("checkout", "-q", migrated)
			write("unrelated.txt", "retain staged data")
			git("add", "unrelated.txt")
			before := git("status", "--porcelain")
			indexPath := git("rev-parse", "--git-path", "index")
			if !filepath.IsAbs(indexPath) {
				indexPath = filepath.Join(dir, indexPath)
			}
			indexBefore, err := os.ReadFile(indexPath)
			must(t, err)
			args := []string{"import", "reconcile", "--base", base, "--migrated", migrated, "--upstream", upstream, "--map", "native=specs/skills"}
			if c.shared {
				args = append(args, "--map", "other=specs/skills")
			}
			for _, jsonOutput := range []bool{false, true} {
				commandArgs := append([]string(nil), args...)
				if jsonOutput {
					commandArgs = append(commandArgs, "--json")
				}
				cmd := exec.Command(binary, commandArgs...)
				cmd.Dir = filepath.Join(dir, c.prefix)
				out, err := cmd.Output()
				if err != nil {
					t.Fatalf("reconcile: %v", err)
				}
				if jsonOutput {
					var plan struct {
						Entries []struct{ Action, Source, Destination string }
					}
					if err := json.Unmarshal(out, &plan); err != nil {
						t.Fatal(err)
					}
					if len(plan.Entries) != 1 || plan.Entries[0].Action != c.want || plan.Entries[0].Source != "native/example/SKILL.md" || plan.Entries[0].Destination != "specs/skills/example/SKILL.md" {
						t.Errorf("plan: %s", out)
					}
				} else if !strings.Contains(string(out), c.want+"\tnative/example/SKILL.md\tspecs/skills/example/SKILL.md") {
					t.Errorf("text plan disagrees: %s", out)
				}
			}
			if status := git("status", "--porcelain"); status != before {
				t.Errorf("changed project: %s", status)
			}
			indexAfter, err := os.ReadFile(indexPath)
			must(t, err)
			if !bytes.Equal(indexBefore, indexAfter) {
				t.Error("planning changed index bytes")
			}
			data, err := os.ReadFile(filepath.Join(dir, projectPath("specs/skills/example/SKILL.md")))
			must(t, err)
			if string(data) != "original" {
				t.Errorf("applied update: %q", data)
			}
		})
	}
}
