package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestSyncJSON_SharedSkillsPrintsOneJSONDocument(t *testing.T) {
	probe := t.TempDir()
	if err := os.Symlink(probe, filepath.Join(probe, "link")); err != nil {
		t.Skipf("shared-skill links need symlink support: %v", err)
	}
	for _, tc := range []struct {
		name        string
		args        []string
		localConfig string
	}{
		{name: "default"},
		{name: "verbose", args: []string{"--verbose"}, localConfig: "version: 1\n"},
		{name: "quiet", args: []string{"--quiet"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := setupSharedSkillsFixture(t, "version: 1\ntargets: [claude, codex, cursor]\nsync:\n  shared-skills: true\n")
			if tc.localConfig != "" {
				mustWriteFile(t, filepath.Join(dir, "agnostic-ai.local.yaml"), tc.localConfig)
			}
			testutil.Chdir(t, dir)
			stdout, stderr := captureLogOut(t), captureNotes(t)
			previousVerbosity := verbosity
			t.Cleanup(func() { verbosity = previousVerbosity })
			for pass := 0; pass < 2; pass++ {
				stdout.Reset()
				root := NewRootCmd("test")
				root.SetOut(stdout)
				root.SetErr(stderr)
				root.SetArgs(append([]string{"sync", "--all", "--json"}, tc.args...))
				if err := root.Execute(); err != nil {
					t.Fatalf("sync pass %d: %v\n%s", pass+1, err, stderr)
				}
				var out jsonOutput
				if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
					t.Errorf("sync pass %d stdout is not one JSON document: %v\n%s", pass+1, err, stdout)
				} else if out.Version != "1" || out.Command != "sync" {
					t.Errorf("sync pass %d version/command = %q/%q", pass+1, out.Version, out.Command)
				}
			}
			link := filepath.Join(dir, ".cursor", "skills", "greet")
			if _, err := os.Readlink(link); err != nil {
				t.Errorf("shared skill link %s: %v", link, err)
			}
			stdout.Reset()
			if err := runSync(t, "--all"); err != nil {
				t.Fatalf("plain sync after JSON: %v", err)
			}
			if stdout.Len() == 0 {
				t.Error("plain sync after JSON omitted its summary")
			}
		})
	}
}

func TestSyncJSON_PipedTargetSelectionKeepsStdoutJSON(t *testing.T) {
	dir := setupSharedSkillsFixture(t, "version: 1\nsync:\n  shared-skills: true\ntargets:\n"+listTargets(allTargetNames()))
	testutil.Chdir(t, dir)
	stdout, stderr := captureLogOut(t), captureNotes(t)
	root := NewRootCmd("test")
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetIn(strings.NewReader("codex\n"))
	root.SetArgs([]string{"sync", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("sync --json: %v\n%s", err, stderr)
	}
	var out jsonOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Errorf("stdout is not one JSON document: %v\n%s", err, stdout)
	} else if out.Command != "sync" || out.Version != "1" {
		t.Errorf("version/command = %q/%q", out.Version, out.Command)
	}
	if !strings.Contains(stderr.String(), "saved 1 target(s) to agnostic-ai.yaml") {
		t.Errorf("stderr omitted saved target summary: %s", stderr)
	}
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("load saved targets: %v", err)
	}
	if !slices.Equal(cfg.Targets, []string{"codex"}) {
		t.Errorf("saved targets = %v, want [codex]", cfg.Targets)
	}
	if _, err := os.Stat(filepath.Join(dir, ".agents", "skills", "greet", "SKILL.md")); err != nil {
		t.Errorf("selected Codex skill missing: %v", err)
	}
}
