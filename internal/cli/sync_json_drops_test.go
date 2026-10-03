package cli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

type syncJSONDrops struct {
	Warnings []dropJSON `json:"warnings"`
	Notes    []dropJSON `json:"notes"`
}

type dropJSON struct {
	Target  string `json:"target"`
	Kind    string `json:"kind"`
	Count   int    `json:"count"`
	Message string `json:"message"`
}

func runSyncJSONDrops(t *testing.T) syncJSONDrops {
	t.Helper()
	var stdout, stderr bytes.Buffer
	root := NewRootCmd("test")
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"sync", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("sync --json: %v\n%s", err, stderr.String())
	}
	var out syncJSONDrops
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, stdout.String())
	}
	return out
}

func TestSyncJSON_ListsDroppedSpecsAndPlainSyncStillPrintsThem(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [codex, aider]\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "srv.yaml"), "command: npx\nargs: [srv]\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "agents", "rev.md"), "---\nname: rev\ndescription: Reviews code.\ntools: [Read]\n---\nReview.\n")
	testutil.Chdir(t, dir)
	silence(t)
	captureLogOut(t)
	warner := captureNotes(t)

	out := runSyncJSONDrops(t)

	wantWarning := dropJSON{Target: "aider", Kind: "mcp", Count: 1, Message: "1 mcp unsupported by aider"}
	if len(out.Warnings) != 1 || out.Warnings[0] != wantWarning {
		t.Errorf("warnings = %+v, want [%+v]", out.Warnings, wantWarning)
	}
	wantNotes := []dropJSON{
		{Target: "codex", Kind: "agent", Count: 1, Message: codexToolsNote + " (Codex uses tools as a configuration table, not a Claude-style allowlist; set x-codex.tools for Codex-native tool settings)"},
		{Target: "aider", Kind: "agent", Count: 1, Message: "1 agent reaches aider only via outputs.aider.rules-file"},
	}
	if len(out.Notes) != len(wantNotes) {
		t.Fatalf("notes = %+v, want %+v", out.Notes, wantNotes)
	}
	for _, want := range wantNotes {
		found := false
		for _, got := range out.Notes {
			found = found || got == want
		}
		if !found {
			t.Errorf("notes = %+v, missing %+v", out.Notes, want)
		}
	}
	if warner.Len() != 0 {
		t.Errorf("sync --json printed drops outside the JSON:\n%s", warner.String())
	}

	if _, err := runCLI(t, "sync"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(warner.String(), "1 mcp unsupported by aider") || !strings.Contains(warner.String(), codexToolsNote) {
		t.Errorf("plain sync after sync --json must still print the drops:\n%s", warner.String())
	}
}

func TestSyncJSON_LeavesOutAcceptedNotes(t *testing.T) {
	setupCoverageAcceptProject(t, acceptCodexTools)
	captureNotes(t)

	out := runSyncJSONDrops(t)

	for _, n := range out.Notes {
		if strings.Contains(n.Message, codexToolsNote) {
			t.Errorf("an accepted note must not be listed: %+v", out.Notes)
		}
	}
}

func TestSyncJSON_EmptyDropListsWhenNothingIsDropped(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "style.md"), "Prefer short functions.\n")
	testutil.Chdir(t, dir)
	silence(t)
	captureLogOut(t)

	out := runSyncJSONDrops(t)

	if out.Warnings == nil || out.Notes == nil || len(out.Warnings)+len(out.Notes) != 0 {
		t.Errorf("warnings = %#v, notes = %#v; want empty lists", out.Warnings, out.Notes)
	}
}

func runSyncJSONDropsMode(t *testing.T, args ...string) syncJSONDrops {
	t.Helper()
	var stdout, stderr bytes.Buffer
	root := NewRootCmd("test")
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs(append([]string{"sync", "--json"}, args...))
	_ = root.Execute()
	var out syncJSONDrops
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("sync --json %v: stdout is not JSON: %v\n%s\n%s", args, err, stdout.String(), stderr.String())
	}
	return out
}

func TestSyncJSONPreviewModes_ListTheDropsSyncJSONLists(t *testing.T) {
	for _, mode := range []string{"--check", "--plan", "--dry-run"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude, aider]\n")
			mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "srv.yaml"), "command: npx\nargs: [srv]\n")
			mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "agents", "rev.md"), "---\nname: rev\ndescription: Reviews code.\n---\nReview.\n")
			testutil.Chdir(t, dir)
			silence(t)
			captureLogOut(t)
			captureNotes(t)

			got := runSyncJSONDropsMode(t, mode)
			want := runSyncJSONDrops(t)

			wantWarning := dropJSON{Target: "aider", Kind: "mcp", Count: 1, Message: "1 mcp unsupported by aider"}
			wantNote := dropJSON{Target: "aider", Kind: "agent", Count: 1, Message: "1 agent reaches aider only via outputs.aider.rules-file"}
			if len(want.Warnings) != 1 || want.Warnings[0] != wantWarning || len(want.Notes) != 1 || want.Notes[0] != wantNote {
				t.Fatalf("sync --json drops = %+v, want one warning %+v and one note %+v", want, wantWarning, wantNote)
			}
			if !slices.Equal(got.Warnings, want.Warnings) || !slices.Equal(got.Notes, want.Notes) {
				t.Errorf("sync --json %s drops = %+v, want %+v", mode, got, want)
			}
		})
	}
}

func TestSyncJSONPreviewModes_EmptyDropListsWhenNothingIsDropped(t *testing.T) {
	for _, mode := range []string{"--check", "--plan", "--dry-run"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\n")
			mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "style.md"), "Prefer short functions.\n")
			testutil.Chdir(t, dir)
			silence(t)
			captureLogOut(t)

			out := runSyncJSONDropsMode(t, mode)

			if out.Warnings == nil || out.Notes == nil || len(out.Warnings)+len(out.Notes) != 0 {
				t.Errorf("warnings = %#v, notes = %#v; want empty lists", out.Warnings, out.Notes)
			}
		})
	}
}
