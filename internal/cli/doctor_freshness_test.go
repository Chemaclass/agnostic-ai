package cli

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// cloneSetupRepo is a Git repository that ignores the root instruction
// files of claude and codex, as the managed block does.
func cloneSetupRepo(t *testing.T) (string, *config.Config, []string) {
	t.Helper()
	dir := setupGitRepo(t)
	writeFile(t, filepath.Join(dir, ".gitignore"), "/AGENTS.md\n/CLAUDE.md\n/.claude/skills/\n")
	return dir, &config.Config{Version: 1, Targets: []string{"claude", "codex", "gemini"}}, []string{"AGENTS.md", "CLAUDE.md", ".claude/skills/handoff/SKILL.md", "GEMINI.md"}
}

func TestCollectCloneSetup_WarnsWithoutFreshnessAndNamesCloudAgents(t *testing.T) {
	dir, cfg, generated := cloneSetupRepo(t)

	r := collectCloneSetup(dir, cfg, spec.Bundle{}, generated, true)

	if r.Ignored != 3 || !r.Stale || r.Freshness != "" {
		t.Errorf("ignored=%d stale=%v freshness=%q, want 3 ignored, stale, no freshness", r.Ignored, r.Stale, r.Freshness)
	}
	want := []cloudInstructionGap{
		{Path: "AGENTS.md", Agents: []string{"Codex Cloud"}},
		{Path: "CLAUDE.md", Agents: []string{"Claude Code on the web"}},
	}
	if len(r.Cloud) != len(want) {
		t.Fatalf("cloud gaps = %+v, want %+v", r.Cloud, want)
	}
	for i := range want {
		if r.Cloud[i].Path != want[i].Path || !slices.Equal(r.Cloud[i].Agents, want[i].Agents) {
			t.Errorf("cloud gap %d = %+v, want %+v", i, r.Cloud[i], want[i])
		}
	}
}

func TestCollectCloneSetup_QuietWhenSomethingRegenerates(t *testing.T) {
	cases := map[string]struct {
		files  map[string]string
		bundle spec.Bundle
	}{
		"install-hook --post-checkout": {
			files: map[string]string{".git/hooks/post-checkout": "#!/bin/sh\n" + postCheckoutHook.text()},
		},
		"hand-written post-merge hook": {
			files: map[string]string{".git/hooks/post-merge": "#!/bin/sh\nagnostic-ai sync --keep-edits\n"},
		},
		"lefthook calling a sync script": {
			files: map[string]string{
				"lefthook.yml":                 "post-checkout:\n  commands:\n    sync:\n      run: test \"{3}\" = 1 || exit 0; .agnostic-ai/scripts/sync.sh\n",
				".agnostic-ai/scripts/sync.sh": "#!/bin/sh\nbin=./agnostic-ai\n\"$bin\" sync --keep-edits\n",
			},
		},
		"husky post-checkout": {
			files: map[string]string{".husky/post-checkout": "npx agnostic-ai sync -q\n"},
		},
		"pre-commit post-merge stage": {
			files: map[string]string{".pre-commit-config.yaml": "repos:\n  - repo: local\n    hooks:\n      - id: sync\n        entry: agnostic-ai sync\n        stages: [post-merge]\n"},
		},
		"session-start hook spec": {
			files: map[string]string{".agnostic-ai/scripts/sync.sh": "#!/bin/sh\nexec agnostic-ai sync --keep-edits\n"},
			bundle: spec.Bundle{Hooks: []spec.Entry{{Name: "sync-on-start", Meta: map[string]any{
				"on": "session-start", "command": `"$CLAUDE_PROJECT_DIR/.agnostic-ai/scripts/sync.sh"`,
			}}}},
		},
		"environment install": {
			bundle: spec.Bundle{Environments: []spec.Entry{{Name: "dev", Meta: map[string]any{"install": "npm ci\nnpx agnostic-ai project\n"}}}},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			dir, cfg, generated := cloneSetupRepo(t)
			for rel, text := range tc.files {
				writeFile(t, filepath.Join(dir, filepath.FromSlash(rel)), text)
			}

			r := collectCloneSetup(dir, cfg, tc.bundle, generated, true)

			if r.Stale || r.Freshness == "" {
				t.Errorf("stale=%v freshness=%q, want a freshness mechanism", r.Stale, r.Freshness)
			}
		})
	}
}

func TestCollectCloneSetup_IgnoresChecksOnlyHooks(t *testing.T) {
	dir, cfg, generated := cloneSetupRepo(t)
	writeFile(t, filepath.Join(dir, "lefthook.yml"), "pre-commit:\n  commands:\n    sync:\n      run: agnostic-ai sync\npost-checkout:\n  commands:\n    check:\n      run: agnostic-ai sync --check\n")

	if r := collectCloneSetup(dir, cfg, spec.Bundle{}, generated, true); !r.Stale {
		t.Errorf("a pre-commit sync and a post-checkout check keep nothing fresh after a pull: %+v", r)
	}
}

func TestCollectCloneSetup_QuietWhenRootFilesAreCommitted(t *testing.T) {
	dir, cfg, generated := cloneSetupRepo(t)
	writeFile(t, filepath.Join(dir, ".gitignore"), "/.claude/skills/\n")
	writeFile(t, filepath.Join(dir, "lefthook.yml"), "post-merge:\n  commands:\n    sync:\n      run: agnostic-ai sync\n")

	r := collectCloneSetup(dir, cfg, spec.Bundle{}, generated, true)

	if r.findings() != 0 {
		t.Errorf("committed root files and a sync hook must report nothing: %+v", r)
	}
}

func TestCollectCloneSetup_TrackedFileIsNotIgnored(t *testing.T) {
	dir, cfg, generated := cloneSetupRepo(t)
	writeFile(t, filepath.Join(dir, "AGENTS.md"), "# notes\n")
	git(t, dir, "add", "-f", "AGENTS.md")

	r := collectCloneSetup(dir, cfg, spec.Bundle{}, generated, true)

	if len(r.Cloud) != 1 || r.Cloud[0].Path != "CLAUDE.md" {
		t.Errorf("a tracked AGENTS.md reaches cloud agents; gaps = %+v", r.Cloud)
	}
}

func TestCollectCloneSetup_EnvironmentInstallCoversItsTarget(t *testing.T) {
	dir, cfg, generated := cloneSetupRepo(t)
	bundle := spec.Bundle{Environments: []spec.Entry{{Name: "dev", Meta: map[string]any{
		"target": "codex", "install": "agnostic-ai sync",
	}}}}

	r := collectCloneSetup(dir, cfg, bundle, generated, true)

	if len(r.Cloud) != 1 || r.Cloud[0].Path != "CLAUDE.md" {
		t.Errorf("a Codex install that syncs generates AGENTS.md in the cloud; gaps = %+v", r.Cloud)
	}
}

func TestCollectCloneSetup_ProjectChecksSkipLocalHooks(t *testing.T) {
	dir, cfg, generated := cloneSetupRepo(t)

	r := collectCloneSetup(dir, cfg, spec.Bundle{}, generated, false)

	if r.Stale || len(r.Cloud) != 2 {
		t.Errorf("--checks project keeps the cloud check and skips the hook check: %+v", r)
	}
}

func TestCollectCloneSetup_AutosyncBuiltinCountsAsFreshness(t *testing.T) {
	for _, local := range []bool{true, false} {
		dir, cfg, generated := cloneSetupRepo(t)
		cfg.Builtins = []string{"handoff", config.AutosyncBuiltin}

		r := collectCloneSetup(dir, cfg, spec.Bundle{}, generated, local)

		if r.Stale || r.Freshness != "builtin autosync" {
			t.Errorf("local=%v: stale=%v freshness=%q, want autosync to count", local, r.Stale, r.Freshness)
		}
		if len(r.Cloud) != 2 {
			t.Errorf("local=%v: autosync does not reach cloud clones, so the cloud gaps stay: %+v", local, r.Cloud)
		}
	}
}

func TestCollectCloneSetup_OutsideGitReportsNothing(t *testing.T) {
	isolateGit(t)
	dir := t.TempDir()
	cfg := &config.Config{Version: 1, Targets: []string{"codex"}}

	if r := collectCloneSetup(dir, cfg, spec.Bundle{}, []string{"AGENTS.md"}, true); r.findings() != 0 || r.Ignored != 0 {
		t.Errorf("outside Git nothing is ignored: %+v", r)
	}
}

func TestCollectCloneSetup_GroupsAgentsSharingAFile(t *testing.T) {
	dir, _, generated := cloneSetupRepo(t)
	cfg := &config.Config{Version: 1, Targets: []string{"codex", "jules", "cursor", "copilot"}}

	r := collectCloneSetup(dir, cfg, spec.Bundle{}, generated, true)

	if len(r.Cloud) != 1 || r.Cloud[0].Path != "AGENTS.md" || !slices.Equal(r.Cloud[0].Agents, []string{"Codex Cloud", "Jules", "Cursor cloud agents"}) {
		t.Errorf("gaps = %+v, want AGENTS.md read by Codex Cloud, Jules and Cursor cloud agents", r.Cloud)
	}
}

func TestTextRunsSync(t *testing.T) {
	cases := map[string]bool{
		"agnostic-ai sync":                            true,
		"npx agnostic-ai sync --keep-edits":           true,
		"go run ./cmd/agnostic-ai sync":               true,
		"pnpm exec agnostic-ai project":               true,
		"agnostic-ai sync --check":                    false,
		"agnostic-ai project --check --against index": false,
		"agnostic-ai sync --dry-run":                  false,
		"# agnostic-ai sync":                          false,
		"agnostic-ai hook memory":                     false,
		"make build":                                  false,
	}
	for text, want := range cases {
		if got := textRunsSync(text, t.TempDir(), 1); got != want {
			t.Errorf("textRunsSync(%q) = %v, want %v", text, got, want)
		}
	}
}

func TestReportCloneSetup_PrintsNextStepsAndStaysQuietWhenFine(t *testing.T) {
	var out strings.Builder
	cmd := &cobra.Command{}
	cmd.SetOut(&out)

	reportCloneSetup(cmd, cloneSetupReport{Ignored: 2})
	if out.Len() != 0 {
		t.Errorf("a fine setup prints nothing:\n%s", out.String())
	}

	reportCloneSetup(cmd, cloneSetupReport{Ignored: 2, Stale: true, Cloud: []cloudInstructionGap{{Path: "AGENTS.md", Agents: []string{"Codex Cloud", "Jules"}}}})
	for _, want := range []string{
		"Clones and cloud agents:",
		"2 generated files are ignored by Git",
		"→ agnostic-ai install-hook --post-checkout",
		"AGENTS.md is ignored, so Codex Cloud and Jules start without project instructions",
		"gitignore.commit: [instructions]",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("report lacks %q:\n%s", want, out.String())
		}
	}
}

func TestDoctorNextStep_CloneSetupReplacesAllPassed(t *testing.T) {
	var out strings.Builder
	cmd := &cobra.Command{}
	cmd.SetOut(&out)

	doctorNextStep(cmd, false, false, false, false, nil, 0, 0, 0, nil, 0, 0, cloneSetupReport{Stale: true})

	if strings.Contains(out.String(), "All checks passed") || !strings.Contains(out.String(), "install-hook --post-checkout") {
		t.Errorf("next step must name the hook, not claim all passed:\n%s", out.String())
	}
}
