package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestUnledgeredReport_HeadAndFileHistoryShareOneRender(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Git invocation counter uses a shell wrapper")
	}
	dir, git := gitRepo(t)
	testutil.Chdir(t, dir)
	// Explicit output options require the full historical renderer.
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\ngitignore:\n  enabled: false\noutputs:\n  claude:\n    provenance-header: true\n")
	syncProject(t)
	mustWriteFile(t, ".claude/launch.json", "{\"configurations\":[{\"name\":\"manual\",\"runtimeExecutable\":\"npm\"}],\"version\":\"0.0.1\"}\n")
	git("add", "-A")
	git("commit", "-q", "-m", "adopt")
	if err := os.Remove(stateFilePath(".")); err != nil {
		t.Fatal(err)
	}
	log := recordHistoryGitRenders(t)
	cfg := &config.Config{Targets: []string{"claude"}}
	emitted := map[string]bool{}
	rep := unledgeredReport(cfg, emitted, syncStateFile{}, strandedOutput(cfg, emitted, syncStateFile{}))
	if len(rep.Leftover) != 0 || len(rep.Orphaned) != 0 {
		t.Errorf("handwritten launch classified as generated: %+v", rep)
	}
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if calls := strings.Count(string(data), "read-tree"); calls != 1 {
		t.Errorf("HEAD and its file history rendered %d times, want one:\n%s", calls, data)
	}
}

func TestHistoryRenderer_HeadCacheKeepsEightHistoricalAdmissions(t *testing.T) {
	for _, tc := range []struct {
		name                     string
		headProof, headInHistory bool
	}{
		{"HEAD proof not requested", false, false},
		{"HEAD preloaded", true, false},
		{"HEAD admitted through history", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir, git := gitRepo(t)
			testutil.Chdir(t, dir)
			var paths, commits []string
			for i := 0; i < 10; i++ {
				p := filepath.Join(".claude", fmt.Sprintf("file-%d.json", i))
				mustWriteFile(t, p, "{}\n")
				git("add", p)
				git("commit", "-q", "-m", fmt.Sprintf("file %d", i))
				sha, err := gitOutput(".", nil, "rev-parse", "HEAD")
				if err != nil {
					t.Fatal(err)
				}
				paths = append(paths, p)
				commits = append(commits, strings.TrimSpace(sha))
			}
			h := &historyRenderer{}
			if !h.prepare() {
				t.Fatal("Git history not prepared")
			}
			for i, commit := range commits {
				h.renders[commit] = map[string]string{filepath.ToSlash(paths[i]): "{}\n"}
			}
			if tc.headProof && !h.provesAtHEAD(paths[9]) {
				t.Error("preloaded HEAD did not prove its output")
			}
			if tc.headInHistory && !h.proves(paths[9]) {
				t.Error("historical HEAD did not prove its output")
			}
			allowed := 8
			if tc.headInHistory {
				allowed = 7
			}
			for i := 0; i < 9; i++ {
				if got, want := h.proves(paths[i]), i < allowed; got != want {
					t.Errorf("history %d proved=%v, want %v", i, got, want)
				}
			}
			if len(h.admitted) != maxHistoryRenders {
				t.Errorf("admitted %d commits, want eight", len(h.admitted))
			}
			if !h.proves(paths[0]) {
				t.Error("admitted commit became unavailable at the cap")
			}
		})
	}
}

func TestHistoryRenderer_PinsFileHistoryToPreparedHEAD(t *testing.T) {
	dir, git := gitRepo(t)
	testutil.Chdir(t, dir)
	mustWriteFile(t, ".claude/old.json", "{}\n")
	git("add", "-A")
	git("commit", "-q", "-m", "old")
	h := &historyRenderer{}
	if !h.prepare() {
		t.Fatal("Git history not prepared")
	}
	mustWriteFile(t, ".claude/new.json", "{}\n")
	git("add", "-A")
	git("commit", "-q", "-m", "new")
	sha, err := gitOutput(".", nil, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	h.renders[strings.TrimSpace(sha)] = map[string]string{".claude/new.json": "{}\n"}
	if h.proves(".claude/new.json") {
		t.Error("scan used a later HEAD as file history proof")
	}
	if len(h.admitted) != 0 {
		t.Error("later commit consumed the history budget")
	}
}

func TestHistoryRenderer_MissingHEADSuppliesNoProof(t *testing.T) {
	dir, _ := gitRepo(t)
	testutil.Chdir(t, dir)
	mustWriteFile(t, ".claude/launch.json", "{}\n")
	h := &historyRenderer{}
	if h.provesAtHEAD(".claude/launch.json") || h.proves(".claude/launch.json") {
		t.Error("uncommitted repository supplied proof")
	}
	if len(h.renders) != 0 || len(h.admitted) != 0 {
		t.Error("missing HEAD consumed render/history budget")
	}
}

func TestHistoryRenderer_FailedHEADRenderIsSharedWithHistory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Git invocation counter uses a shell wrapper")
	}
	dir, git := gitRepo(t)
	testutil.Chdir(t, dir)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [unknown-target]\n")
	mustWriteFile(t, ".claude/launch.json", "{}\n")
	git("add", "-A")
	git("commit", "-q", "-m", "broken historical config")
	log := recordHistoryGitRenders(t)
	h := &historyRenderer{}
	if h.provesAtHEAD(".claude/launch.json") || h.proves(".claude/launch.json") || h.proves(".claude/launch.json") {
		t.Error("failed render supplied ownership proof")
	}
	if len(h.renders) != 1 || len(h.admitted) != 1 {
		t.Errorf("render attempts=%d, historical admissions=%d", len(h.renders), len(h.admitted))
	}
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if calls := strings.Count(string(data), "read-tree"); calls != 1 {
		t.Errorf("failed commit retried %d times", calls)
	}
}

func recordHistoryGitRenders(t *testing.T) string {
	t.Helper()
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	log := filepath.Join(bin, "renders")
	wrapper := "#!/bin/sh\nif [ \"$1\" = read-tree ]; then case \"$GIT_INDEX_FILE\" in *agnostic-ai-history-config-*) ;; *) printf '%s\\n' \"$*\" >> " + adapters.ShellQuote(log) + ";; esac; fi\nexec " + adapters.ShellQuote(gitPath) + " \"$@\"\n"
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(wrapper), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return log
}
