package emit

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestRewriteHookPath_RootRunsFromNestedProject(t *testing.T) {
	for _, variable := range []string{"$CLAUDE_PROJECT_DIR", "${CLAUDE_PROJECT_DIR}"} {
		t.Run(variable, func(t *testing.T) {
			root := t.TempDir()
			git := exec.Command("git", "init", "-q", root)
			if out, err := git.CombinedOutput(); err != nil {
				t.Fatalf("git init: %v %s", err, out)
			}
			project := filepath.Join(root, "packages", "app $special 'name'")
			if err := os.MkdirAll(filepath.Join(project, "child"), 0o755); err != nil {
				t.Fatal(err)
			}
			testutil.Chdir(t, project)
			command := `printf '%s' "` + variable + `/custom scripts/guard.sh"`
			rewritten := RewriteHookPath(command, "codex")
			run := exec.Command("sh", "-c", rewritten)
			run.Dir = filepath.Join(project, "child")
			run.Env = []string{"PATH=" + os.Getenv("PATH")}
			out, err := run.CombinedOutput()
			canonical, canonicalErr := filepath.EvalSymlinks(project)
			if canonicalErr != nil {
				t.Fatal(canonicalErr)
			}
			want := filepath.ToSlash(filepath.Join(canonical, "custom scripts", "guard.sh"))
			if err != nil || string(out) != want {
				t.Errorf("command=%s result=%q err=%v want=%q", rewritten, out, err, want)
			}
		})
	}
}

func TestRewriteHookPath_RootUsesNativeVariables(t *testing.T) {
	for _, tc := range []struct{ target, want string }{
		{"claude", "CLAUDE_PROJECT_DIR"}, {"cursor", "CLAUDE_PROJECT_DIR"}, {"trae", "CLAUDE_PROJECT_DIR"},
		{"gemini", "GEMINI_PROJECT_DIR"}, {"qoder", "QODER_PROJECT_DIR"}, {"factory", "FACTORY_PROJECT_DIR"},
	} {
		t.Run(tc.target, func(t *testing.T) {
			got := RewriteHookPath(`"${CLAUDE_PROJECT_DIR}/custom/guard.sh"`, tc.target)
			if !strings.Contains(got, "${"+tc.want+"}") {
				t.Errorf("rewritten=%s want native %s", got, tc.want)
			}
		})
	}
}

func TestRewriteHookPath_RootLeavesLiteralVariables(t *testing.T) {
	for _, command := range []string{`echo '$CLAUDE_PROJECT_DIR'`, `echo \$CLAUDE_PROJECT_DIR`, `echo "$CLAUDE_PROJECT_DIR_EXTRA"`} {
		if got := RewriteHookPath(command, "codex"); got != command {
			t.Errorf("rewrote literal: %q -> %q", command, got)
		}
	}
}

func TestReportHookProjectRoot_NamesUnsupportedHookAndHonorsPolicy(t *testing.T) {
	testutil.TempCwd(t)
	hook := spec.Entry{Kind: spec.KindHook, Name: "guard", Path: ".agnostic-ai/hooks/guard.yaml", Meta: map[string]any{"command": `"${CLAUDE_PROJECT_DIR:-/tmp}/guard.sh"`}}
	b := spec.NewBundle([]spec.Entry{hook})
	c := Capabilities{Target: "codex", Supports: []spec.Kind{spec.KindHook}}
	ResetCoverageNotes()
	t.Cleanup(ResetCoverageNotes)
	out := swapWarnerForNotes(t)
	if err := ReportUnsupported(c, b, "warn"); err != nil {
		t.Fatal(err)
	}
	FlushCoverageNotes()
	for _, want := range []string{"guard", "CLAUDE_PROJECT_DIR", "codex"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("note misses %s: %s", want, out.String())
		}
	}
	if err := ReportUnsupported(c, b, "error"); err == nil || !strings.Contains(err.Error(), "guard") {
		t.Errorf("error=%v, want named unsupported hook", err)
	}
}

func TestReportHookProjectRoot_NoGitAndExecArgsHonorPolicy(t *testing.T) {
	testutil.TempCwd(t)
	for _, meta := range []map[string]any{
		{"command": `sh "$CLAUDE_PROJECT_DIR/custom/guard.sh"`},
		{"command": "sh", "args": []any{"$CLAUDE_PROJECT_DIR/custom/guard.sh", "arg with space"}},
		{"command": `"$(printf %s "$CLAUDE_PROJECT_DIR")/guard.sh"`},
	} {
		hook := spec.Entry{Kind: spec.KindHook, Name: "guard", Meta: meta}
		if err := ReportHookProjectRoot("codex", []spec.Entry{hook}, "error", false); err == nil || !strings.Contains(err.Error(), "guard") {
			t.Errorf("meta=%v error=%v, want named hook error", meta, err)
		}
		ResetCoverageNotes()
		if err := ReportHookProjectRoot("codex", []spec.Entry{hook}, "silent", false); err != nil {
			t.Error(err)
		}
		if PendingCoverageNotesCount() != 0 {
			t.Error("silent root check buffered a note")
		}
	}
}

func TestRewriteHookPath_ExecArgsStayLiteral(t *testing.T) {
	args := []string{"arg with space", "'$literal'"}
	command := "$CLAUDE_PROJECT_DIR/.claude/hooks/guard.sh"
	got := RewriteHookPath(command, "codex", map[string]any{"args": []any{args[0], args[1]}})
	want := "$CLAUDE_PROJECT_DIR/.codex/hooks/guard.sh"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
	folded := ExecFormCommand(got, args)
	if !strings.HasPrefix(folded, "'$CLAUDE_PROJECT_DIR/") || !strings.Contains(folded, "'arg with space'") {
		t.Errorf("exec words lost literal semantics: %s", folded)
	}
}

func TestReportHookProjectRoot_NamesNonPOSIXReferences(t *testing.T) {
	for _, command := range []string{`%CLAUDE_PROJECT_DIR%/guard.cmd`, `$env:CLAUDE_PROJECT_DIR/guard.ps1`, `${env:CLAUDE_PROJECT_DIR}/guard.ps1`} {
		hook := spec.Entry{Kind: spec.KindHook, Name: "windows-guard", Meta: map[string]any{"command": command}}
		if err := ReportHookProjectRoot("codex", []spec.Entry{hook}, "error", false); err == nil || !strings.Contains(err.Error(), "windows-guard") {
			t.Errorf("command=%s error=%v, want named unsupported syntax", command, err)
		}
	}
}

func TestReportHookProjectRoot_WindowsOverrideAndLengthHonorPolicy(t *testing.T) {
	testutil.TempCwd(t)
	for _, meta := range []map[string]any{
		{"command": "guard.sh", "commandWindows": `$env:CLAUDE_PROJECT_DIR/guard.ps1`},
		{"command": `echo ${#CLAUDE_PROJECT_DIR}`},
	} {
		hook := spec.Entry{Kind: spec.KindHook, Name: "guard", Path: "hooks/guard.yaml", Meta: meta}
		for _, global := range []bool{false, true} {
			ResetCoverageNotes()
			out := swapWarnerForNotes(t)
			if err := ReportHookProjectRoot("codex", []spec.Entry{hook}, "warn", global); err != nil {
				t.Error(err)
			}
			FlushCoverageNotes()
			if !strings.Contains(out.String(), "guard") || !strings.Contains(out.String(), "CLAUDE_PROJECT_DIR") {
				t.Errorf("missing named warning: %s", out.String())
			}
			if err := ReportHookProjectRoot("codex", []spec.Entry{hook}, "error", global); err == nil || !strings.Contains(err.Error(), "guard") {
				t.Errorf("meta=%v global=%v error=%v", meta, global, err)
			}
		}
	}
	t.Cleanup(ResetCoverageNotes)
}
