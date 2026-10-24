package integration

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/overview"
)

func TestView_BinaryResolvesProjectGlobalAndLinkedWorktree(t *testing.T) {
	binary := handoffBuildCLI(t)
	source := t.TempDir()
	t.Setenv("AGNOSTIC_AI_HOME", source)
	t.Setenv("AGNOSTIC_AI_NO_UPDATE_CHECK", "1")
	writeProjectFile(t, source, "agnostic-ai.yaml", "targets: [claude, codex]\n")
	project := t.TempDir()
	writeProjectFile(t, project, "agnostic-ai.yaml", "version: 1\ntargets: [cursor, opencode]\nbuiltins: []\n")
	handoffGit(t, project, "init")
	handoffGit(t, project, "add", "agnostic-ai.yaml")
	handoffGit(t, project, "-c", "user.name=View Test", "-c", "user.email=view@example.invalid", "-c", "commit.gpgsign=false", "commit", "-m", "test fixture")
	linked := filepath.Join(t.TempDir(), "linked")
	handoffGit(t, project, "worktree", "add", "--detach", linked)
	nested := filepath.Join(linked, "src", "nested")
	must(t, os.MkdirAll(nested, 0755))
	outside := t.TempDir()
	for _, tc := range []struct {
		name, cwd     string
		args, targets []string
	}{
		{name: "outside", cwd: outside, targets: []string{"claude", "codex"}},
		{name: "project", cwd: project, targets: []string{"cursor", "opencode"}},
		{name: "linked nested", cwd: nested, targets: []string{"cursor", "opencode"}},
		{name: "explicit global", cwd: nested, args: []string{"--global"}, targets: []string{"claude", "codex"}},
		{name: "explicit directory", cwd: outside, args: []string{"--dir", linked}, targets: []string{"cursor", "opencode"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, binary, append([]string{"view", "--json"}, tc.args...)...)
			cmd.Dir = tc.cwd
			data, err := cmd.Output()
			if err != nil {
				t.Fatalf("view: %v", err)
			}
			var view overview.View
			if err := json.Unmarshal(data, &view); err != nil {
				t.Fatalf("overview: %v\n%s", err, data)
			}
			if !slices.Equal(view.Snapshot.Targets, tc.targets) {
				t.Errorf("targets = %v, want %v", view.Snapshot.Targets, tc.targets)
			}
		})
	}
	for _, sub := range []string{"authority", "overview"} {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		cmd := exec.CommandContext(ctx, binary, "view", sub)
		cmd.Dir = outside
		data, err := cmd.CombinedOutput()
		cancel()
		if err == nil || !strings.Contains(string(data), `unknown command "`+sub+`"`) {
			t.Errorf("view %s: %v\n%s", sub, err, data)
		}
	}
}
