package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// approvalsHome points the home config at a fresh folder and returns it.
func approvalsHome(t *testing.T, shared string) string {
	t.Helper()
	home := t.TempDir()
	source := filepath.Join(home, ".agnostic-ai")
	t.Setenv("HOME", home)
	t.Setenv(envUserGlobalRoot, source)
	t.Setenv(config.EnvApprovals, "")
	if shared != "" {
		mustWriteFile(t, filepath.Join(source, config.ConfigFileName), shared)
	}
	testutil.Chdir(t, t.TempDir())
	return source
}

func approvalsStatusJSON(t *testing.T) approvalsStatusOutput {
	t.Helper()
	out, err := runCLI(t, "approvals", "status", "--json")
	if err != nil {
		t.Fatalf("approvals status: %v\n%s", err, out)
	}
	var got approvalsStatusOutput
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	return got
}

func toolStatus(t *testing.T, got approvalsStatusOutput, tool string) approvalsToolStatus {
	t.Helper()
	for _, s := range got.Tools {
		if s.Tool == tool {
			return s
		}
	}
	t.Fatalf("no %s in %+v", tool, got.Tools)
	return approvalsToolStatus{}
}

func TestApprovalsStatus_DefaultIsAsk(t *testing.T) {
	approvalsHome(t, "version: 1\ntargets: [claude, codex]\n")

	out, err := runCLI(t, "approvals", "status")
	if err != nil {
		t.Fatalf("approvals status: %v\n%s", err, out)
	}

	for _, want := range []string{"claude", "codex", "ask"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "yolo") || strings.Contains(out, "review") {
		t.Errorf("a config without approvals named a loosening mode:\n%s", out)
	}
}

func TestApprovalsStatus_RoutePerTool(t *testing.T) {
	approvalsHome(t, "version: 1\ntargets: [claude, codex, cursor]\napprovals:\n  mode: yolo\n  tools:\n    codex: review\n")

	got := approvalsStatusJSON(t)

	claude := toolStatus(t, got, "claude")
	if claude.Mode != config.ApprovalsYolo || claude.Effective != config.ApprovalsYolo || claude.Route == "" || claude.From != "approvals.mode" {
		t.Errorf("claude: %+v", claude)
	}
	codex := toolStatus(t, got, "codex")
	if codex.Mode != config.ApprovalsReview || codex.Effective != config.ApprovalsReview || codex.From != "approvals.tools.codex" {
		t.Errorf("codex: %+v", codex)
	}
	cursor := toolStatus(t, got, "cursor")
	if cursor.Mode != config.ApprovalsYolo || cursor.Effective != config.ApprovalsAsk || cursor.Route != "" {
		t.Errorf("cursor: %+v", cursor)
	}

	out, err := runCLI(t, "approvals", "status")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "falls back to ask") {
		t.Errorf("no fallback note for cursor:\n%s", out)
	}
}

func TestApprovalsStatus_EnvironmentAsks(t *testing.T) {
	approvalsHome(t, "version: 1\ntargets: [claude]\napprovals:\n  mode: yolo\n")
	t.Setenv(config.EnvApprovals, "ask")

	claude := toolStatus(t, approvalsStatusJSON(t), "claude")

	if claude.Effective != config.ApprovalsAsk || claude.From != config.EnvApprovals {
		t.Errorf("claude: %+v", claude)
	}
}

func TestApprovalsStatus_ProjectFileAsks(t *testing.T) {
	approvalsHome(t, "version: 1\ntargets: [claude]\napprovals:\n  mode: yolo\n")
	mustWriteFile(t, config.ConfigFileName, "version: 1\napprovals:\n  mode: ask\n")

	claude := toolStatus(t, approvalsStatusJSON(t), "claude")

	if claude.Effective != config.ApprovalsAsk || !strings.HasSuffix(claude.From, config.ConfigFileName) {
		t.Errorf("claude: %+v", claude)
	}
}

func TestApprovalsStatus_ProjectFileCannotLoosen(t *testing.T) {
	approvalsHome(t, "version: 1\ntargets: [claude]\n")
	mustWriteFile(t, config.ConfigFileName, "version: 1\napprovals:\n  mode: yolo\n")

	claude := toolStatus(t, approvalsStatusJSON(t), "claude")

	if claude.Effective != config.ApprovalsAsk {
		t.Errorf("claude: %+v", claude)
	}
}

func TestApprovalsAsk_WritesTheLocalHomeConfig(t *testing.T) {
	source := approvalsHome(t, "version: 1\ntargets: [claude, codex]\napprovals:\n  mode: yolo\n  tools:\n    codex: review\n")
	local := filepath.Join(source, "local", config.ConfigFileName)
	mustWriteFile(t, local, "# mine\nmodels:\n  fast:\n    claude: haiku\napprovals:\n  tools:\n    codex: review\n  projects:\n    /tmp/x: yolo\n  always-ask:\n    - \"Bash(make deploy*)\"\n")

	out, err := runCLI(t, "approvals", "ask")
	if err != nil {
		t.Fatalf("approvals ask: %v\n%s", err, out)
	}

	got := approvalsStatusJSON(t)
	for _, tool := range []string{"claude", "codex"} {
		if s := toolStatus(t, got, tool); s.Effective != config.ApprovalsAsk {
			t.Errorf("%s: %+v", tool, s)
		}
	}
	data, err := os.ReadFile(local)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# mine", "claude: haiku", "mode: ask", "Bash(make deploy*)"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("local config lost %q:\n%s", want, data)
		}
	}
	if strings.Contains(string(data), "/tmp/x") {
		t.Errorf("local config kept a loosening project:\n%s", data)
	}
	if !strings.Contains(out, "review") {
		t.Errorf("no note that a native reviewer keeps a running session's mode:\n%s", out)
	}
}

func TestApprovalsAsk_CreatesTheLocalHomeConfig(t *testing.T) {
	source := approvalsHome(t, "")

	if out, err := runCLI(t, "approvals", "ask"); err != nil {
		t.Fatalf("approvals ask: %v\n%s", err, out)
	}

	data, err := os.ReadFile(filepath.Join(source, "local", config.ConfigFileName))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "approvals:\n  mode: ask\n" {
		t.Errorf("got:\n%s", data)
	}
}

func TestGlobalConfig_ReadsApprovalsWithoutWarning(t *testing.T) {
	source := t.TempDir()
	mustWriteFile(t, filepath.Join(source, config.ConfigFileName), "version: 1\ntargets: [claude]\napprovals:\n  mode: yolo\n")
	var warn strings.Builder

	if _, err := loadGlobalTargets(source, &warn); err != nil {
		t.Fatal(err)
	}

	if warn.Len() != 0 {
		t.Errorf("warned: %s", warn.String())
	}
}

func approvalsProject(t *testing.T, cfg string) {
	t.Helper()
	approvalsHome(t, "")
	mustWriteFile(t, config.ConfigFileName, cfg)
	mustWriteFile(t, ".agnostic-ai/skills/review/SKILL.md", "---\nname: review\ndescription: Review code.\n---\nReview.\n")
}

func TestValidate_RejectsLooseningApprovalsInAProjectFile(t *testing.T) {
	for _, mode := range []string{"review", "yolo"} {
		t.Run(mode, func(t *testing.T) {
			approvalsProject(t, "version: 1\ntargets: [claude]\napprovals:\n  mode: "+mode+"\n")

			out, err := runCLI(t, "validate")

			if err == nil || !strings.Contains(out, "approvals.mode: "+mode) {
				t.Fatalf("validate passed or did not name the key: %v\n%s", err, out)
			}
			out, err = runCLI(t, "lint")
			if err == nil || !strings.Contains(out, "LINT064") {
				t.Fatalf("lint passed or did not report LINT064: %v\n%s", err, out)
			}
		})
	}
}

func TestValidate_AcceptsAskInAProjectFile(t *testing.T) {
	approvalsProject(t, "version: 1\ntargets: [claude]\napprovals:\n  mode: ask\n")

	if out, err := runCLI(t, "validate"); err != nil {
		t.Fatalf("validate: %v\n%s", err, out)
	}
}

func TestValidate_SkipsApprovalsCheckInTheHomeConfig(t *testing.T) {
	source := approvalsHome(t, "version: 1\ntargets: [claude]\napprovals:\n  mode: yolo\n")
	mustWriteFile(t, filepath.Join(source, ".agnostic-ai/skills/review/SKILL.md"), "---\nname: review\ndescription: Review code.\n---\nReview.\n")
	testutil.Chdir(t, source)

	if out, err := runCLI(t, "validate"); err != nil {
		t.Fatalf("validate in the home folder: %v\n%s", err, out)
	}
}
