package testutil

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestDisableGitBackgroundWork_CommitStartsNoBackgroundGit(t *testing.T) {
	t.Setenv("GIT_CONFIG_COUNT", "")
	for i := range quietGitConfig {
		t.Setenv("GIT_CONFIG_KEY_"+strconv.Itoa(i), "")
		t.Setenv("GIT_CONFIG_VALUE_"+strconv.Itoa(i), "")
	}
	if err := DisableGitBackgroundWork(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	trace := filepath.Join(t.TempDir(), "git-trace.jsonl")
	t.Setenv("GIT_TRACE2_EVENT", trace)
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q"},
		{"-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false", "commit", "-q", "--allow-empty", "-m", "base"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	data, err := os.ReadFile(trace)
	if err != nil {
		t.Fatalf("read Git trace: %v", err)
	}
	sawCommit := false
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var event struct {
			Event string   `json:"event"`
			Name  string   `json:"name"`
			Argv  []string `json:"argv"`
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("parse Git trace: %v", err)
		}
		if event.Event == "cmd_name" && event.Name == "commit" {
			sawCommit = true
		}
		if event.Event == "child_start" {
			t.Errorf("commit started a Git child process: %v", event.Argv)
		}
	}
	if !sawCommit {
		t.Error("Git trace did not record the commit")
	}
}
