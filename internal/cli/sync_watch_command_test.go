package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestSyncWatchCommand_FollowsTargetsEditedInConfig(t *testing.T) {
	dir := setupFixture(t)
	writeTestFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "r1.md"), "---\nname: r1\ndescription: always\nalways: true\n---\nrule body\n")
	cfgPath := filepath.Join(dir, "agnostic-ai.yaml")
	writeTestFile(t, cfgPath, "version: 1\ntargets: [claude, codex]\n")

	cmd := exec.Command(os.Args[0], "-test.run=^TestSyncWatchCommandHelperProcess$")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "AGNOSTIC_AI_SYNC_WATCH_HELPER=1")
	out := &safeBuffer{}
	cmd.Stdout = out
	cmd.Stderr = out
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	waitForOutput(t, out, "watching", 20*time.Second)
	writeAndBumpMtime(t, cfgPath, []byte("version: 1\ntargets: [claude]\n"))
	waitForGone(t, filepath.Join(dir, "AGENTS.md"), 20*time.Second)
	writeAndBumpMtime(t, cfgPath, []byte("version: 1\ntargets: [claude, cursor]\n"))
	waitForFile(t, filepath.Join(dir, ".cursor", "rules", "r1.mdc"), 20*time.Second)
}

func TestSyncWatchCommandHelperProcess(t *testing.T) {
	if os.Getenv("AGNOSTIC_AI_SYNC_WATCH_HELPER") == "" {
		t.Skip("helper for TestSyncWatchCommand_FollowsTargetsEditedInConfig")
	}
	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "--watch", "--watch-poll"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
}

func TestWatchSync_KeepsFilesWhileConfigListsNoTargets(t *testing.T) {
	for name, edit := range map[string]string{
		"null list":  "version: 1\ntargets:\n",
		"empty list": "version: 1\ntargets: []\n",
	} {
		t.Run(name, func(t *testing.T) {
			dir, buf, stop := followsTargetsFixture(t, watchTargets(nil, nil, nil, []string{"claude", "codex"}))
			defer stop()

			cfgPath := filepath.Join(dir, "agnostic-ai.yaml")
			writeAndBumpMtime(t, cfgPath, []byte(edit))
			waitForOutput(t, buf, "lists no tool", 15*time.Second)

			for _, path := range []string{"AGENTS.md", "CLAUDE.md", filepath.Join(".claude", "rules", "r1.md")} {
				if _, err := os.Stat(filepath.Join(dir, path)); err != nil {
					t.Errorf("%s was removed while targets listed no tool: %v", path, err)
				}
			}

			writeAndBumpMtime(t, cfgPath, []byte("version: 1\ntargets: [claude]\n"))
			waitForGone(t, filepath.Join(dir, "AGENTS.md"), 15*time.Second)
		})
	}
}
