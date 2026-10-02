package cli

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestWatchSync_PollEmitsSkillAssetEditedDuringResync(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	writeTestFile(t, config.ConfigFileName, "version: 1\ntargets: [claude]\n")
	rule := filepath.Join(".agnostic-ai", "rules", "demo.md")
	asset := filepath.Join(".agnostic-ai", "skills", "demo", "references", "guide.txt")
	writeTestFile(t, rule, "---\nname: demo\n---\nInitial rule.\n")
	writeTestFile(t, filepath.Join(".agnostic-ai", "skills", "demo", "SKILL.md"),
		"---\nname: demo\ndescription: Demo skill.\n---\nUse the guide.\n")
	writeTestFile(t, asset, "Initial skill asset.\n")
	waitPaused, resume := startPausedPollWatch(t)

	writeAndBumpMtime(t, rule, []byte("---\nname: demo\n---\nRule emitted before the asset edit.\n"))
	waitPaused()
	waitForFileContaining(t, filepath.Join(dir, ".claude", "rules", "demo.md"),
		"Rule emitted before the asset edit.", 5*time.Second)
	emittedAsset := filepath.Join(dir, ".claude", "skills", "demo", "references", "guide.txt")
	waitForFileContaining(t, emittedAsset, "Initial skill asset.", 5*time.Second)

	writeAndBumpMtime(t, asset, []byte("Skill asset edited during the preceding sync.\n"))
	resume()
	waitForFileContaining(t, emittedAsset, "Skill asset edited during the preceding sync.", 5*time.Second)
}

func TestWatchSync_PollReloadsSourceRootsAndEmitsEditsDuringResync(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	writeTestFile(t, config.ConfigFileName, "version: 1\ntargets: [claude]\n")
	writeTestFile(t, filepath.Join(".agnostic-ai", "rules", "original.md"),
		"---\nname: original\n---\nOriginal source rule.\n")
	rule := filepath.Join("extra", "rules", "reloaded.md")
	writeTestFile(t, rule, "---\nname: reloaded\n---\nReloaded source rule.\n")
	waitPaused, resume := startPausedPollWatch(t)

	writeAndBumpMtime(t, config.ConfigFileName,
		[]byte("version: 1\ntargets: [claude]\nsources:\n  rules: extra/rules\n"))
	waitPaused()
	emittedRule := filepath.Join(dir, ".claude", "rules", "reloaded.md")
	waitForFileContaining(t, emittedRule, "Reloaded source rule.", 5*time.Second)

	writeAndBumpMtime(t, rule, []byte("---\nname: reloaded\n---\nNew root edited during the preceding sync.\n"))
	resume()
	waitForFileContaining(t, emittedRule, "New root edited during the preceding sync.", 5*time.Second)

	writeAndBumpMtime(t, rule, []byte("---\nname: reloaded\n---\nLater edit in the reloaded root.\n"))
	waitForFileContaining(t, emittedRule, "Later edit in the reloaded root.", 5*time.Second)
}

func startPausedPollWatch(t *testing.T) (waitPaused, resume func()) {
	t.Helper()
	paused := make(chan struct{})
	released := make(chan struct{})
	var pauseOnce, resumeOnce sync.Once
	previous := pollSyncPause
	pollSyncPause = func() {
		pauseOnce.Do(func() {
			close(paused)
			<-released
		})
	}
	resume = func() { resumeOnce.Do(func() { close(released) }) }
	ctx, cancel := context.WithCancel(context.Background())
	buf := captureWatchOutput(t)
	done := make(chan error, 1)
	t.Cleanup(func() {
		resume()
		cancel()
		err := <-done
		pollSyncPause = previous
		if err != nil {
			t.Errorf("polling watch: %v", err)
		}
	})
	go func() {
		done <- watchSync(ctx, 20*time.Millisecond, ".", []string{"claude"}, false, false, "off", true, 1)
	}()
	waitForOutput(t, buf, "watching", 10*time.Second)
	return func() {
		t.Helper()
		select {
		case <-paused:
		case <-time.After(10 * time.Second):
			t.Fatalf("polling watch did not pause after resync; output:\n%s", buf.String())
		}
	}, resume
}
