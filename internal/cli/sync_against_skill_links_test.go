package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

const checkedSkillLink = ".claude/skills/link-check"

func skillLinkBlob(dir, destination string) (string, error) {
	cmd := exec.Command("git", "-C", dir, "hash-object", "-w", "--stdin")
	cmd.Stdin = strings.NewReader(destination)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func syncSharedSkillFixture(t *testing.T) error {
	t.Helper()
	root := NewRootCmd("test")
	root.SetArgs([]string{"sync"})
	return root.Execute()
}

func committedSharedSkillProject(t *testing.T) (string, bool) {
	t.Helper()
	dir := setupFixture(t)
	isolateGit(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	t.Setenv("AGNOSTIC_AI_HOME", filepath.Join(home, ".agnostic-ai"))
	git(t, dir, "init", "-q")
	git(t, dir, "config", "core.autocrlf", "false")
	git(t, dir, "config", "core.symlinks", "true")
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude, codex, cursor]\nsync:\n  shared-skills: true\ngitignore:\n  enabled: true\n  commit: [instructions, skills]\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai/skills/link-check/SKILL.md"), "---\nname: link-check\ndescription: Check a shared skill.\n---\nRead assets/example.txt.\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai/skills/link-check/assets/example.txt"), "Skill asset.\n")
	testutil.Chdir(t, dir)
	silence(t)
	if err := syncSharedSkillFixture(t); err != nil {
		t.Fatalf("sync fixture: %v", err)
	}
	_, linkErr := os.Readlink(filepath.Join(dir, checkedSkillLink))
	git(t, dir, "add", "-A")
	if linkErr != nil {
		// Retain the false-policy control on hosts where native links cannot be created.
		for _, target := range []string{"claude", "cursor"} {
			link := "." + target + "/skills/link-check"
			git(t, dir, "rm", "-q", "-r", "--cached", "--", link)
			blob, err := skillLinkBlob(dir, "../../.agents/skills/link-check")
			if err != nil {
				t.Fatalf("record synthetic link: %v", err)
			}
			git(t, dir, "update-index", "--add", "--cacheinfo", "120000,"+strings.TrimSpace(blob)+","+link)
		}
	}
	if staged := git(t, dir, "ls-files", "--stage", "--", checkedSkillLink); !strings.HasPrefix(staged, "120000 ") {
		t.Fatalf("fixture did not record a Git link: %s", staged)
	}
	git(t, dir, "commit", "-q", "-m", "shared skill")
	return dir, linkErr == nil
}

func TestSyncCheckAgainstSkillLinks_DiagnosesExportPolicyInEveryFormat(t *testing.T) {
	dir, _ := committedSharedSkillProject(t)
	git(t, dir, "config", "core.symlinks", "false")
	beforeIndex := git(t, dir, "ls-files", "--stage")
	beforeStatus := git(t, dir, "status", "--porcelain=v1", "--untracked-files=all")
	beforeConfig, err := os.ReadFile(filepath.Join(dir, ".git/config"))
	if err != nil {
		t.Fatal(err)
	}
	for _, ref := range []string{"index", "HEAD"} {
		for _, format := range []string{"human", "github", "json"} {
			t.Run(ref+"/"+format, func(t *testing.T) {
				args := []string{}
				if format == "json" {
					args = append(args, "--json")
				} else {
					args = append(args, "--format", format)
				}
				stdout, stderr, err := checkAgainst(t, ref, args...)
				if err == nil {
					t.Fatal("a Git link exported as text must fail")
				}
				cause := stdout + stderr + err.Error()
				if format == "json" {
					var out jsonOutput
					if err := json.Unmarshal([]byte(stdout), &out); err != nil {
						t.Fatalf("invalid JSON: %v\n%s", err, stdout)
					}
					cause = ""
					for _, record := range out.Errors {
						cause += record.Message + "\n"
					}
				}
				for _, want := range []string{checkedSkillLink, "core.symlinks=false", "plain file", "git config --local core.symlinks true", "shared-skills: false"} {
					if !strings.Contains(cause, want) {
						t.Errorf("diagnosis lacks %q: %s", want, cause)
					}
				}
				if format == "github" && !strings.Contains(stdout, "::error file="+checkedSkillLink+"::") {
					t.Errorf("GitHub output must annotate the affected link: %s", stdout)
				}
				if strings.Contains(err.Error(), "run agnostic-ai sync, then stage") {
					t.Errorf("regeneration is not the link-policy remedy: %v", err)
				}
				if ref == "HEAD" && !strings.Contains(cause, "commit") {
					t.Errorf("HEAD repair must explain committing the repair: %s", cause)
				}
			})
		}
	}
	if after := git(t, dir, "ls-files", "--stage"); after != beforeIndex {
		t.Error("checks changed the original index")
	}
	if after := git(t, dir, "status", "--porcelain=v1", "--untracked-files=all"); after != beforeStatus {
		t.Error("checks changed the original worktree")
	}
	afterConfig, err := os.ReadFile(filepath.Join(dir, ".git/config"))
	if err != nil || !bytes.Equal(beforeConfig, afterConfig) {
		t.Errorf("checks changed Git settings: %v", err)
	}
}

func TestSyncCheckAgainstSkillLinks_KeepsIndependentDriftAndExcludesOtherTargets(t *testing.T) {
	dir, _ := committedSharedSkillProject(t)
	git(t, dir, "config", "core.symlinks", "false")
	if stdout, stderr, err := checkAgainst(t, "index", "--only", "codex"); err != nil || strings.Contains(stdout+stderr, "core.symlinks") {
		t.Fatalf("excluded linked outputs must not cause a failure: %v\n%s\n%s", err, stdout, stderr)
	}
	editRuleSpec(t, dir, "Independent staged rule edit.\n")
	git(t, dir, "add", ".agnostic-ai/rules/r1.md")
	stdout, _, err := checkAgainst(t, "index", "--json")
	if err == nil {
		t.Fatal("link policy and independent rule drift must fail")
	}
	var out jsonOutput
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("parse check JSON: %v", err)
	}
	found := false
	for _, record := range out.Writes {
		if record.Path == ".claude/rules/r1.md" {
			found = true
		}
	}
	if !found || !strings.Contains(stdout, "core.symlinks=false") {
		t.Errorf("both rule drift and link diagnosis must remain: %s", stdout)
	}
}

func TestSyncCheckAgainstSkillLinks_TruePolicyAndOptOutRepair(t *testing.T) {
	dir, nativeLink := committedSharedSkillProject(t)
	if nativeLink {
		for _, ref := range []string{"index", "HEAD"} {
			if stdout, stderr, err := checkAgainst(t, ref); err != nil {
				t.Fatalf("native-link true-policy control: %v\n%s\n%s", err, stdout, stderr)
			}
		}
	} else {
		t.Log("native link creation failed; synthetic Git link retains the false-policy controls")
	}
	git(t, dir, "config", "core.symlinks", "false")
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude, codex, cursor]\nsync:\n  shared-skills: false\ngitignore:\n  enabled: true\n  commit: [instructions, skills]\n")
	if err := syncSharedSkillFixture(t); err != nil {
		t.Fatalf("turn off sharing: %v", err)
	}
	if info, err := os.Lstat(filepath.Join(dir, checkedSkillLink)); err != nil || !info.IsDir() {
		t.Fatalf("repair must produce a real directory: %v", err)
	}
	git(t, dir, "add", "-A")
	if stdout, stderr, err := checkAgainst(t, "index"); err != nil {
		t.Errorf("staged real copies must pass with false policy: %v\n%s\n%s", err, stdout, stderr)
	}
	git(t, dir, "commit", "-q", "-m", "commit real skill copies")
	if stdout, stderr, err := checkAgainst(t, "HEAD"); err != nil {
		t.Errorf("committed real copies must pass with false policy: %v\n%s\n%s", err, stdout, stderr)
	}
	blob, err := skillLinkBlob(dir, "somewhere-else")
	if err != nil {
		t.Fatal(err)
	}
	git(t, dir, "update-index", "--add", "--cacheinfo", "120000,"+strings.TrimSpace(blob)+",unrelated-link")
	if stdout, stderr, err := checkAgainst(t, "index"); err != nil || strings.Contains(stdout+stderr, "core.symlinks") {
		t.Errorf("an unrelated Git link must not trigger the diagnosis: %v\n%s\n%s", err, stdout, stderr)
	}
}

func TestSyncCheckAgainstSkillLinks_DiagnosesForceAddedIgnoredSkills(t *testing.T) {
	dir, nativeLink := committedSharedSkillProject(t)
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude, codex, cursor]\nsync:\n  shared-skills: true\ngitignore:\n  enabled: true\n  commit: [instructions]\n")
	if err := syncSharedSkillFixture(t); err != nil {
		t.Fatalf("configure ignored skills: %v", err)
	}
	roots := []string{".claude/skills/link-check", ".cursor/skills/link-check", ".agents/skills/link-check"}
	for _, root := range roots {
		git(t, dir, "rm", "-q", "-r", "--cached", "--", root)
		if ignored := git(t, dir, "check-ignore", "--no-index", "--", root); ignored != root {
			t.Fatalf("fixture skill root is not ignored: %s", ignored)
		}
		git(t, dir, "add", "-f", "--", root)
	}
	git(t, dir, "add", "-A")
	if !nativeLink {
		for _, link := range roots[:2] {
			git(t, dir, "rm", "-q", "-r", "--cached", "--", link)
			blob, err := skillLinkBlob(dir, "../../.agents/skills/link-check")
			if err != nil {
				t.Fatal(err)
			}
			git(t, dir, "update-index", "--add", "--cacheinfo", "120000,"+strings.TrimSpace(blob)+","+link)
		}
	}
	if recorded := git(t, dir, "ls-files", "--stage", "--", checkedSkillLink); !strings.HasPrefix(recorded, "120000 ") {
		t.Fatalf("force-added fixture must record a link: %s", recorded)
	}
	git(t, dir, "commit", "-q", "-m", "force-add ignored skill outputs")
	git(t, dir, "config", "core.symlinks", "false")
	for _, ref := range []string{"index", "HEAD"} {
		for _, filtered := range []bool{false, true} {
			args := []string{"--json"}
			if filtered {
				args = append(args, "--only", "claude")
			}
			stdout, _, err := checkAgainst(t, ref, args...)
			if err == nil {
				t.Errorf("%s filtered=%t: force-added link exported as text must fail", ref, filtered)
			}
			var out jsonOutput
			if err := json.Unmarshal([]byte(stdout), &out); err != nil {
				t.Fatalf("parse %s filtered=%t JSON: %v", ref, filtered, err)
			}
			cause := ""
			for _, failure := range out.Errors {
				cause += failure.Message + "\n"
			}
			if !strings.Contains(cause, checkedSkillLink) || !strings.Contains(cause, "core.symlinks=false") {
				t.Errorf("%s filtered=%t: tracked ignored link needs its diagnosis: %s", ref, filtered, stdout)
			}
			if filtered && strings.Contains(cause, ".cursor/skills/link-check") {
				t.Errorf("excluded Cursor must not contribute a diagnosis: %s", cause)
			}
		}
	}
}
