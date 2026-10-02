package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestCollectMtimes_LinkedSourceKeepsLexicalFiles(t *testing.T) {
	dir := testutil.TempCwd(t)
	physical := t.TempDir()
	asset := filepath.Join(physical, "demo", "references", "guide.txt")
	writeTestFile(t, asset, "Guide.\n")
	alias := filepath.Join(dir, "skills-link")
	testutil.DirectoryAlias(t, physical, alias)
	instructions := filepath.Join(dir, "AGNOSTIC_AI.md")
	writeTestFile(t, instructions, "Instructions.\n")

	got := collectMtimes([]string{alias, instructions})
	lexical := filepath.Join(alias, "demo", "references", "guide.txt")
	for _, path := range []string{lexical, instructions} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got[path] != info.ModTime() {
			t.Errorf("mtime for %s = %v, want %v", path, got[path], info.ModTime())
		}
	}
	if _, ok := got[asset]; ok {
		t.Errorf("snapshot used physical path %s", asset)
	}
}

func TestAddWatchPaths_LinkedSourceRegistersNestedDirectories(t *testing.T) {
	dir := testutil.TempCwd(t)
	physical := t.TempDir()
	writeTestFile(t, filepath.Join(physical, "demo", "references", "guide.txt"), "Guide.\n")
	alias := filepath.Join(dir, "skills-link")
	testutil.DirectoryAlias(t, physical, alias)
	w, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = w.Close() }()

	if err := addWatchPaths(w, []string{alias}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{alias, filepath.Join(alias, "demo"), filepath.Join(alias, "demo", "references")} {
		if !slices.Contains(w.WatchList(), path) {
			t.Errorf("watch list %v is missing %s", w.WatchList(), path)
		}
	}
}

func TestWatchSync_LinkedSourceReEmitsNestedAssets(t *testing.T) {
	assertWatchLinkedSourceAssets(t, false)
}

func TestWatchSync_PollLinkedSourceReEmitsNestedAssets(t *testing.T) {
	assertWatchLinkedSourceAssets(t, true)
}

func assertWatchLinkedSourceAssets(t *testing.T, forcePoll bool) {
	t.Helper()
	dir := testutil.TempCwd(t)
	silence(t)
	physical := t.TempDir()
	asset := filepath.Join(physical, "demo", "references", "guide.txt")
	writeTestFile(t, filepath.Join(physical, "demo", "SKILL.md"), skillSpec("demo", ""))
	writeTestFile(t, asset, "Initial linked asset.\n")
	alias := filepath.Join(dir, "skills-link")
	testutil.DirectoryAlias(t, physical, alias)
	source := alias
	if forcePoll {
		source = "skills-link"
	}
	writeTestFile(t, config.ConfigFileName, fmt.Sprintf(
		"version: 1\nsources:\n  skills: %q\ntargets: [claude]\n", filepath.ToSlash(source)))
	reloadedAlias := filepath.Join(dir, "reloaded-skills-link")
	var reloadedWatch <-chan struct{}
	if !forcePoll {
		ready := make(chan struct{}, 1)
		previous := watchAdd
		watchAdd = func(w *fsnotify.Watcher, path string) error {
			err := previous(w, path)
			if err == nil && filepath.Clean(path) == filepath.Join(reloadedAlias, "demo", "references") {
				select {
				case ready <- struct{}{}:
				default:
				}
			}
			return err
		}
		t.Cleanup(func() { watchAdd = previous })
		reloadedWatch = ready
	}
	buf, stop := startWatch(t, nil, forcePoll)
	defer stop()
	if !forcePoll && (!strings.Contains(buf.String(), "fsnotify") || strings.Contains(buf.String(), "(poll)")) {
		t.Fatalf("fsnotify test fell back to polling:\n%s", buf.String())
	}
	output := filepath.Join(dir, ".claude", "skills", "demo", "references", "guide.txt")
	waitForFileContaining(t, output, "Initial linked asset.", 5*time.Second)

	writeAndBumpMtime(t, asset, []byte("Updated linked asset.\n"))
	waitForFileContaining(t, output, "Updated linked asset.", 5*time.Second)
	waitForOutput(t, buf, "skill change · re-syncing 1 target: claude", 5*time.Second)

	reloaded := t.TempDir()
	writeTestFile(t, filepath.Join(reloaded, "demo", "SKILL.md"), skillSpec("demo", ""))
	reloadedAsset := filepath.Join(reloaded, "demo", "references", "guide.txt")
	writeTestFile(t, reloadedAsset, "Reloaded linked asset.\n")
	testutil.DirectoryAlias(t, reloaded, reloadedAlias)
	if forcePoll {
		reloadedAlias = "reloaded-skills-link"
	}
	writeAndBumpMtime(t, config.ConfigFileName, []byte(fmt.Sprintf(
		"version: 1\nsources:\n  skills: %q\ntargets: [claude]\n", filepath.ToSlash(reloadedAlias))))
	waitForFileContaining(t, output, "Reloaded linked asset.", 5*time.Second)
	if !forcePoll {
		select {
		case <-reloadedWatch:
		case <-time.After(5 * time.Second):
			t.Fatal("reloaded nested asset directory was not registered with fsnotify")
		}
	}

	writeAndBumpMtime(t, reloadedAsset, []byte("Later linked asset.\n"))
	waitForFileContaining(t, output, "Later linked asset.", 5*time.Second)
	if !forcePoll && strings.Contains(buf.String(), "(poll)") {
		t.Errorf("watch fell back to polling after the edits or config reload:\n%s", buf.String())
	}
}
