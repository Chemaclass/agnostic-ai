package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

var emptyTargetsOutputs = []string{
	".agents/skills/pr/SKILL.md",
	".claude/rules/style.md",
	".claude/skills/pr/SKILL.md",
	"AGENTS.md",
	"CLAUDE.md",
}

const emptyTargetsStatePath = ".agnostic-ai/.sync-state"

// syncedEmptyTargetsProject syncs a project with claude and codex, then
// leaves it ready for the test to empty the targets list.
func syncedEmptyTargetsProject(t *testing.T) string {
	t.Helper()
	t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
	dir := testutil.TempCwd(t)
	writeFile(t, filepath.Join(dir, ".agnostic-ai/AGNOSTIC_AI.md"), "Root.\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai/rules/style.md"), "---\nname: style\n---\n\nUse tabs.\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai/skills/pr/SKILL.md"), "---\nname: pr\ndescription: Open a pull request.\n---\n\nBody.\n")
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude, codex]\n")
	if out, err := runCLI(t, "sync", "--all"); err != nil {
		t.Fatalf("first sync: %v\n%s", err, out)
	}
	requireEmptyTargetsOutputs(t, true)
	return dir
}

func requireEmptyTargetsOutputs(t *testing.T, want bool) {
	t.Helper()
	for _, p := range emptyTargetsOutputs {
		if _, err := os.Stat(p); (err == nil) != want {
			t.Errorf("%s exists = %v, want %v", p, err == nil, want)
		}
	}
}

func TestSync_EmptyTargetsKeepsEveryFile(t *testing.T) {
	for _, tc := range []struct{ name, config, local string }{
		{"empty list", "version: 1\ntargets: []\n", ""},
		{"null", "version: 1\ntargets:\n", ""},
		{"local override", "version: 1\ntargets: [claude, codex]\n", "version: 1\ntargets: []\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			syncedEmptyTargetsProject(t)
			before, err := os.ReadFile(emptyTargetsStatePath)
			if err != nil {
				t.Fatal(err)
			}
			writeFile(t, "agnostic-ai.yaml", tc.config)
			if tc.local != "" {
				writeFile(t, "agnostic-ai.local.yaml", tc.local)
			}

			out, err := runCLI(t, "sync")

			if err != nil {
				t.Fatalf("sync: %v\n%s", err, out)
			}
			if !strings.Contains(out, "targets lists no tool") {
				t.Errorf("missing warning:\n%s", out)
			}
			requireEmptyTargetsOutputs(t, true)
			after, err := os.ReadFile(emptyTargetsStatePath)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) {
				t.Error(".sync-state changed")
			}
		})
	}
}

func TestSyncCheck_EmptyTargetsReportsNoDrift(t *testing.T) {
	syncedEmptyTargetsProject(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: []\n")

	if out, err := runCLI(t, "sync", "--check"); err != nil {
		t.Errorf("sync --check: %v\n%s", err, out)
	}
	requireEmptyTargetsOutputs(t, true)
}

func TestStatus_EmptyTargetsShowsNoDrift(t *testing.T) {
	syncedEmptyTargetsProject(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: []\n")
	cfg, b, err := loadProject(".")
	if err != nil {
		t.Fatal(err)
	}

	drift, _, err := captureAllAndDiff(cfg.Targets, cfg, b)

	if err != nil {
		t.Fatal(err)
	}
	if drift != 0 {
		t.Errorf("drift = %d, want 0", drift)
	}
}

func TestDoctorFix_EmptyTargetsKeepsFiles(t *testing.T) {
	syncedEmptyTargetsProject(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: []\n")
	var out bytes.Buffer
	root := NewRootCmd("test")
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetIn(strings.NewReader(""))
	root.SetArgs([]string{"doctor", "--fix"})

	_ = root.Execute()

	requireEmptyTargetsOutputs(t, true)
	if !strings.Contains(out.String(), "targets lists no tool") {
		t.Errorf("doctor did not warn:\n%s", out.String())
	}
}

func TestSyncJSON_EmptyTargetsWarnsInJSON(t *testing.T) {
	syncedEmptyTargetsProject(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: []\n")
	var stdout, stderr bytes.Buffer
	root := NewRootCmd("test")
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"sync", "--json"})

	if err := root.Execute(); err != nil {
		t.Fatalf("sync --json: %v", err)
	}

	var got struct {
		Version  string `json:"version"`
		Command  string `json:"command"`
		Warnings []struct {
			Target     string `json:"target"`
			Message    string `json:"message"`
			Suggestion string `json:"suggestion"`
		} `json:"warnings"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, stdout.String())
	}
	if len(got.Warnings) != 1 || !strings.Contains(got.Warnings[0].Message, "targets lists no tool") {
		t.Fatalf("warnings = %+v", got.Warnings)
	}
	if got.Version != "1" || got.Command != "sync" || got.Warnings[0].Target != "agnostic-ai" {
		t.Errorf("unexpected envelope: %+v", got)
	}
	requireEmptyTargetsOutputs(t, true)
}

func TestSync_AllowEmptyTargetsRemovesFiles(t *testing.T) {
	syncedEmptyTargetsProject(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: []\n")

	if out, err := runCLI(t, "sync", "--allow-empty-targets"); err != nil {
		t.Fatalf("sync --allow-empty-targets: %v\n%s", err, out)
	}

	requireEmptyTargetsOutputs(t, false)
	if out, err := runCLI(t, "sync", "--check"); err != nil {
		t.Errorf("sync --check after removal: %v\n%s", err, out)
	}
}

func TestSync_AllowEmptyTargetsConflicts(t *testing.T) {
	syncedEmptyTargetsProject(t)
	for _, flag := range []string{"--check", "--plan", "--watch", "--global"} {
		out, err := runCLI(t, "sync", "--allow-empty-targets", flag)
		if err == nil || !strings.Contains(err.Error(), "--allow-empty-targets cannot be combined") {
			t.Errorf("%s: err = %v\n%s", flag, err, out)
		}
	}
}

func TestWatch_FirstPassKeepsFilesWithEmptyTargets(t *testing.T) {
	syncedEmptyTargetsProject(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: []\n")
	buf := captureWatchOutput(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- watchSync(ctx, 20*time.Millisecond, ".", nil, false, false, "off", true, 1)
	}()

	waitForOutput(t, buf, "watching", 10*time.Second)
	cancel()
	if err := <-done; err != nil && ctx.Err() == nil {
		t.Fatal(err)
	}

	requireEmptyTargetsOutputs(t, true)
	if n := strings.Count(buf.String(), "targets lists no tool"); n != 1 {
		t.Errorf("warning printed %d times, want 1:\n%s", n, buf.String())
	}
}

func TestSync_RemovingOneTargetStillRemovesItsFiles(t *testing.T) {
	syncedEmptyTargetsProject(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")

	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}

	for _, p := range []string{".agents/skills/pr/SKILL.md", "AGENTS.md"} {
		if _, err := os.Stat(p); err == nil {
			t.Errorf("%s should be removed with codex", p)
		}
	}
	for _, p := range []string{".claude/rules/style.md", ".claude/skills/pr/SKILL.md", "CLAUDE.md"} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s should stay: %v", p, err)
		}
	}
}
