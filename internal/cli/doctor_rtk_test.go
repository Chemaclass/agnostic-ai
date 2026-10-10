package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestRTKDeclaredPermissions_ReportsChangedAskRule(t *testing.T) {
	sources := []rtkSettings{{Path: "project", Permissions: map[string][]string{
		"ask": {"Bash(git status)"}, "allow": {"Bash(rtk git status)"},
	}}}
	original := rtkDeclaredDecision("git status", sources)
	rewritten := rtkDeclaredDecision("rtk git status", sources)
	if original.Decision != "ask" || rewritten.Decision != "allow" {
		t.Fatalf("decisions: original=%+v rewritten=%+v", original, rewritten)
	}
}

func TestRTKDeclaredPermissions_KeepsUnknownAndDeny(t *testing.T) {
	cases := []struct{ command, rule, list, want string }{
		{"git status", "Bash(git:*)", "deny", "deny"},
		{"rtk git status", "Bash(rtk git:*)", "ask", "ask"},
		{"git status", "Bash(git * status)", "allow", "unknown"},
		{"git status && touch marker", "Bash(git:*)", "allow", "unknown"},
		{"git status", "Bash(rtk git:*)", "allow", "unknown"},
		{"git status", "Bash(git status *)", "deny", "deny"},
	}
	for _, c := range cases {
		t.Run(c.rule+"/"+c.command, func(t *testing.T) {
			got := rtkDeclaredDecision(c.command, []rtkSettings{{Permissions: map[string][]string{c.list: {c.rule}}}})
			if got.Decision != c.want {
				t.Errorf("got %s, want %s", got.Decision, c.want)
			}
		})
	}
}

func TestRTKDeclaredPermissions_UnsupportedToolGlobPreventsAllow(t *testing.T) {
	for _, rule := range []string{"*", "B*"} {
		got := rtkDeclaredDecision("git status", []rtkSettings{{Permissions: map[string][]string{
			"allow": {"Bash(git status)"}, "deny": {rule},
		}}})
		if got.Decision == "allow" {
			t.Errorf("deny %q hidden by allow: %+v", rule, got)
		}
	}
}

func TestRTKSettings_DecodesPermissionModesAndSiblingFields(t *testing.T) {
	got, err := decodeRTKSettings("settings.json", []byte(`{"permissions":{"defaultMode":"acceptEdits","disableBypassPermissionsMode":"disable","allow":["Bash(git status)"],"blockReadsOutsideWorkingDirectories":true}}`))
	if err != nil {
		t.Fatal(err)
	}
	if got.Permissions["allow"][0] != "Bash(git status)" {
		t.Fatalf("lost rule: %+v", got)
	}
}

func TestRTKDeclaredPermissions_EquivalentDecisions(t *testing.T) {
	for _, decision := range []string{"deny", "ask", "allow"} {
		sources := []rtkSettings{{Permissions: map[string][]string{decision: {"Bash(git status)", "Bash(rtk git status)"}}}}
		if original, rewritten := rtkDeclaredDecision("git status", sources), rtkDeclaredDecision("rtk git status", sources); original.Decision != decision || rewritten.Decision != decision {
			t.Fatalf("equivalent %s decisions: %+v / %+v", decision, original, rewritten)
		}
	}
}

func TestRTKDeclaredPermissions_UnsupportedDenyPreventsAllowClaim(t *testing.T) {
	got := rtkDeclaredDecision("git status", []rtkSettings{{Permissions: map[string][]string{
		"allow": {"Bash(git status)"}, "deny": {"Bash(git * status)"},
	}}})
	if got.Decision != "unknown" {
		t.Fatalf("unknown deny reported as %+v", got)
	}
}

func TestRTKDeclaredPermissions_WrappersAndParameterRulesStayUnknown(t *testing.T) {
	for _, command := range []string{"timeout 3 git status", "/usr/bin/timeout 3 git status", "time git status", "nice git status", "nohup git status", "stdbuf -oL git status", "command git status", "builtin git status", "noglob git status", "xargs git status"} {
		got := rtkDeclaredDecision(command, []rtkSettings{{Permissions: rtkPermissionRules{
			"allow": {"Bash(*)"}, "deny": {"Bash(git status)"},
		}}})
		if got.Decision != "unknown" {
			t.Errorf("wrapper %q reported as %+v", command, got)
		}
	}
	for _, rule := range []string{"Bash(run_in_background:true)", "Bash(run_in_background :true)", "Bash(timeout:1000)", "Bash(timeout:*)", "Bash(description:status)", "Bash(dangerouslyDisableSandbox:true)"} {
		got := rtkDeclaredDecision("git status", []rtkSettings{{Permissions: rtkPermissionRules{
			"allow": {"Bash(git status)"}, "deny": {rule},
		}}})
		if got.Decision != "unknown" {
			t.Errorf("parameter rule %q reported as %+v", rule, got)
		}
	}
}

func TestRTKDiagnostic_BoundsDescendantHeldOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX processor fixture")
	}
	processor := filepath.Join(t.TempDir(), "rtk")
	if err := os.WriteFile(processor, []byte("#!/bin/sh\nsleep 3 &\nprintf 'rtk fixture\\n'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	_, err := runRTKDiagnostic(processor, "--version")
	if err == nil {
		t.Fatal("expected a failure for inherited output that stays open")
	}
	if elapsed := time.Since(start); elapsed > 1500*time.Millisecond {
		t.Fatalf("descendant kept the diagnostic waiting for %s", elapsed)
	}
}

func TestDoctorRTK_HelpDocumentsReadOnlyScope(t *testing.T) {
	var out bytes.Buffer
	cmd := NewRootCmd("test")
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"doctor", "rtk", "--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"--command", "--json", "does not execute", "declared"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("help missing %q: %s", want, out.String())
		}
	}
}

func TestRTKReport_TextAndJSONPreserveUnknown(t *testing.T) {
	report := rtkReport{Command: "git status", Status: "missing", Runtime: "unknown", Original: rtkDecision{Decision: "unknown"}, Rewritten: rtkDecision{Decision: "unknown"}, Warnings: []string{"RTK is not installed"}}
	var text, data bytes.Buffer
	if err := printRTKReport(&text, report, false); err != nil {
		t.Fatal(err)
	}
	if err := printRTKReport(&data, report, true); err != nil {
		t.Fatal(err)
	}
	var decoded rtkReport
	if err := json.Unmarshal(data.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Status != report.Status || decoded.Runtime != "unknown" || !strings.Contains(text.String(), "unknown") || strings.Contains(text.String(), "safe") {
		t.Fatalf("reports disagree: %s / %s", text.String(), data.String())
	}
}

func TestDoctorRTK_MissingProcessorKeepsApprovalUnknown(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)
	t.Setenv("PATH", t.TempDir())
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	mustWrite(t, "agnostic-ai.yaml", "targets: [claude]\n")
	var out bytes.Buffer
	cmd := NewRootCmd("test")
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"doctor", "rtk", "--command", "git status", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var got rtkReport
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Status != "missing" || got.Runtime != "unknown" || got.Rewritten.Decision != "unknown" {
		t.Fatalf("missing processor: %+v", got)
	}
}

func TestDoctorRTK_ProcessesDataWithoutExecutingPayload(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX fake processor; matching/report tests run on every OS")
	}
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	bin := t.TempDir()
	processor := filepath.Join(bin, "rtk")
	if err := os.WriteFile(processor, []byte("#!/bin/sh\ncase \"$1\" in\n--version) printf 'rtk fixture\\n';;\nrewrite) printf '%s\\n' \"$2\";;\nesac\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	mustWrite(t, "agnostic-ai.yaml", "targets: [claude]\n")
	var out bytes.Buffer
	cmd := NewRootCmd("test")
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"doctor", "rtk", "--command", "git status; touch marker", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat("marker"); !os.IsNotExist(err) {
		t.Fatalf("payload executed: %v", err)
	}
	var got rtkReport
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Original.Decision != "unknown" || got.Rewritten.Decision != "unknown" || got.Runtime != "unknown" {
		t.Fatalf("compound command reported known: %+v", got)
	}
	if got.Status != "unchanged" {
		t.Fatalf("unchanged processor output classified as %s", got.Status)
	}
	out.Reset()
	cmd = NewRootCmd("test")
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"doctor", "rtk", "--command", "rtk git status", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Status != "unchanged" || got.Changed {
		t.Fatalf("already-prefixed: %+v", got)
	}
}

func TestDoctorRTK_HandlesProcessorAskAndDeny(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX fake processor; classification tests run on every OS")
	}
	for _, code := range []string{"2", "3"} {
		t.Run(code, func(t *testing.T) {
			dir := setupFixture(t)
			testutil.Chdir(t, dir)
			silence(t)
			t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
			bin := t.TempDir()
			script := "#!/bin/sh\nif [ \"$1\" = --version ]; then printf 'rtk fixture\\n'; exit 0; fi\n"
			if code == "3" {
				script += "printf 'rtk git status\\n'\n"
			}
			script += "exit " + code + "\n"
			if err := os.WriteFile(filepath.Join(bin, "rtk"), []byte(script), 0755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", bin)
			mustWrite(t, "agnostic-ai.yaml", "targets: [claude]\n")
			var out bytes.Buffer
			cmd := NewRootCmd("test")
			cmd.SetOut(&out)
			cmd.SetArgs([]string{"doctor", "rtk", "--command", "git status", "--json"})
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			var got rtkReport
			if err := json.Unmarshal(out.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if got.Runtime != "unknown" {
				t.Fatalf("processor claimed host outcome: %+v", got)
			}
			if code == "2" && (got.Status != "processor-denied" || got.Replacement != "") {
				t.Fatalf("deny: %+v", got)
			}
			if code == "3" && (got.Status != "rewritten" || got.Replacement != "rtk git status") {
				t.Fatalf("ask: %+v", got)
			}
		})
	}
}

func TestRTKHandlers_DoNotClassifyEchoAsProcessor(t *testing.T) {
	for _, command := range []string{"echo rtk rewrite", "printf '%s' 'rtk hook claude'"} {
		if rtkDirectHandler(command) {
			t.Fatalf("data classified as RTK processor: %s", command)
		}
	}
}

func TestDoctorRTK_ReportsDeclaredChangeAndDuplicateOwners(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX fake processor; matching/report tests run on every OS")
	}
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)
	user := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", user)
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "rtk"), []byte("#!/bin/sh\ncase \"$1\" in\n--version) printf 'rtk fixture\\n';;\nrewrite) printf 'rtk git status\\n';;\nesac\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	mustWrite(t, "agnostic-ai.yaml", "targets: [claude]\nbuiltins: [rtk]\n")
	mustWrite(t, ".agnostic-ai/settings/approval.yaml", "permissions:\n  ask: ['Bash(git status)']\n  allow: ['Bash(rtk git status)']\n")
	mustWrite(t, filepath.Join(user, "settings.json"), `{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"rtk hook claude"}]}]}}`)
	var out bytes.Buffer
	cmd := NewRootCmd("test")
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"doctor", "rtk", "--command", "git status", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var report rtkReport
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if !report.Changed || report.Original.Decision != "ask" || report.Rewritten.Decision != "allow" || len(report.Hooks) != 2 {
		t.Fatalf("wrong diagnosis: %+v", report)
	}
	if report.Runtime != "unknown" || len(report.Warnings) != 2 {
		t.Fatalf("missing warning or runtime limit: %+v", report)
	}
	if _, err := os.Stat(".claude/settings.json"); !os.IsNotExist(err) {
		t.Fatalf("diagnostic wrote settings: %v", err)
	}
}

func TestRTKDiagnosticOutput_RejectsCopyBeyondLimit(t *testing.T) {
	var output rtkDiagnosticOutput
	reader := io.LimitReader(strings.NewReader(strings.Repeat("x", 70000)), 70000)
	_, err := io.Copy(&output, reader)
	if err == nil || !strings.Contains(err.Error(), "exceeds 64 KiB") {
		t.Errorf("copy overflow error = %v", err)
	}
}

func TestDoctorRTK_RejectsOversizedProcessorReplacement(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX processor fixture")
	}
	for _, exitCode := range []int{0, 3} {
		t.Run(fmt.Sprintf("exit_%d", exitCode), func(t *testing.T) {
			dir := setupFixture(t)
			testutil.Chdir(t, dir)
			silence(t)
			t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
			bin := t.TempDir()
			script := fmt.Sprintf("#!/bin/sh\nif [ \"$1\" = --version ]; then printf 'rtk fixture\\n'; exit 0; fi\nprintf '%%s' '%s'\nexit %d\n", strings.Repeat("x", 70000), exitCode)
			if err := os.WriteFile(filepath.Join(bin, "rtk"), []byte(script), 0o755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", bin)
			mustWrite(t, "agnostic-ai.yaml", "targets: [claude]\n")
			var out bytes.Buffer
			cmd := NewRootCmd("test")
			cmd.SetOut(&out)
			cmd.SetArgs([]string{"doctor", "rtk", "--command", "git status", "--json"})
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), "exceeds 64 KiB") {
				t.Errorf("oversized processor exit %d: error = %v, report bytes = %d", exitCode, err, out.Len())
			}
			if out.Len() != 0 {
				t.Errorf("oversized processor produced a report of %d bytes", out.Len())
			}
		})
	}
}

func TestRTKDiagnostic_AcceptsReplacementAtOutputLimit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX processor fixture")
	}
	replacement := "rtk " + strings.Repeat("x", 65532)
	processor := filepath.Join(t.TempDir(), "rtk")
	script := fmt.Sprintf("#!/bin/sh\nprintf '%%s' '%s'\n", replacement)
	if err := os.WriteFile(processor, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	output, err := runRTKDiagnostic(processor, "rewrite", "git status")
	if err != nil {
		t.Fatal(err)
	}
	if string(output) != replacement {
		t.Errorf("replacement differs at output limit: got %d bytes, want %d", len(output), len(replacement))
	}
}

func TestDoctorRTK_UsesNativeSettingsOnlyWithoutPlannedSettings(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX processor fixture")
	}
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	bin := t.TempDir()
	script := "#!/bin/sh\ncase \"$1\" in\n--version) printf 'rtk fixture\\n';;\nrewrite) printf 'rtk git status\\n';;\nesac\n"
	if err := os.WriteFile(filepath.Join(bin, "rtk"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	mustWrite(t, "agnostic-ai.yaml", "targets: [claude]\n")
	mustWrite(t, ".claude/settings.json", `{"permissions":{"ask":["Bash(git status)"],"allow":["Bash(rtk git status)"]},"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"rtk hook claude"},{"type":"command","command":"rtk hook claude"}]}]}}`)
	run := func() rtkReport {
		t.Helper()
		var out bytes.Buffer
		cmd := NewRootCmd("test")
		cmd.SetOut(&out)
		cmd.SetArgs([]string{"doctor", "rtk", "--command", "git status", "--json"})
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
		var report rtkReport
		if err := json.Unmarshal(out.Bytes(), &report); err != nil {
			t.Fatal(err)
		}
		return report
	}
	native := run()
	if native.Original.Decision != "ask" || native.Rewritten.Decision != "allow" || !native.Changed {
		t.Errorf("native declared decisions = %s -> %s, changed = %t", native.Original.Decision, native.Rewritten.Decision, native.Changed)
	}
	if len(native.Hooks) != 2 || !strings.Contains(strings.Join(native.Warnings, "\n"), "multiple known RTK handlers") {
		t.Errorf("native handlers = %v, warnings = %v", native.Hooks, native.Warnings)
	}
	mustWrite(t, ".agnostic-ai/settings/approval.yaml", "permissions:\n  deny: ['Bash(git status)']\n  ask: ['Bash(rtk git status)']\n")
	planned := run()
	if planned.Original.Decision != "deny" {
		t.Errorf("planned original deny decision = %s", planned.Original.Decision)
	}
	if len(planned.Hooks) != 2 {
		t.Errorf("emitted handlers were counted again through native fallback: %v", planned.Hooks)
	}
}

func TestRTKDiagnosticOutput_PreservesCopyFailureAndCompletion(t *testing.T) {
	readError := errors.New("processor output ended before completion")
	var incomplete rtkDiagnosticOutput
	reader := io.MultiReader(strings.NewReader("rtk git"), iotest.ErrReader(readError))
	_, err := io.Copy(&incomplete, reader)
	if !errors.Is(err, readError) {
		t.Errorf("copy error = %v", err)
	}
	if !errors.Is(incomplete.err, readError) {
		t.Errorf("recorded copy error = %v", incomplete.err)
	}
	if incomplete.buffer.String() != "rtk git" {
		t.Errorf("incomplete bytes = %q", incomplete.buffer.String())
	}
	_, stickyErr := io.Copy(&incomplete, io.LimitReader(strings.NewReader("ignored"), 7))
	if !errors.Is(stickyErr, readError) || incomplete.buffer.String() != "rtk git" {
		t.Errorf("copy failure was not sticky: error = %v, output = %q", stickyErr, incomplete.buffer.String())
	}
	var complete rtkDiagnosticOutput
	replacement := "rtk git status"
	_, err = io.Copy(&complete, io.LimitReader(strings.NewReader(replacement), int64(len(replacement))))
	if err != nil || complete.err != nil || complete.buffer.String() != replacement {
		t.Errorf("complete copy: error = %v, recorded error = %v, output = %q", err, complete.err, complete.buffer.String())
	}
}

func TestDoctorRTK_RejectsIncompleteAskReplacement(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX processor fixture")
	}
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	bin := t.TempDir()
	script := "#!/bin/sh\nif [ \"$1\" = --version ]; then printf 'rtk fixture\\n'; exit 0; fi\nprintf 'rtk git'\n(/bin/sleep 1; printf ' status') &\nexit 3\n"
	if err := os.WriteFile(filepath.Join(bin, "rtk"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	mustWrite(t, "agnostic-ai.yaml", "targets: [claude]\n")
	var out bytes.Buffer
	cmd := NewRootCmd("test")
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"doctor", "rtk", "--command", "git status", "--json"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "RTK diagnostic rewrite") {
		t.Errorf("unfinished processor output: error = %v, report = %s", err, out.String())
	}
	if out.Len() != 0 {
		t.Errorf("unfinished processor output produced %d report bytes", out.Len())
	}
}

func TestDoctorRTK_DeduplicatesAliasedSettings(t *testing.T) {
	for _, planned := range []bool{false, true} {
		for _, absolute := range []bool{false, true} {
			t.Run(fmt.Sprintf("planned=%t/absolute=%t", planned, absolute), func(t *testing.T) {
				dir := setupFixture(t)
				testutil.Chdir(t, dir)
				silence(t)
				t.Setenv("PATH", t.TempDir())
				userDir := ".claude"
				if absolute {
					userDir = filepath.Join(dir, ".claude")
				}
				t.Setenv("CLAUDE_CONFIG_DIR", userDir)
				mustWrite(t, "agnostic-ai.yaml", "targets: [claude]\n")
				mustWrite(t, ".claude/settings.json", `{"permissions":{"ask":["Bash(git status)"]},"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"rtk hook claude"}]}]}}`)
				wantDecision := "ask"
				if planned {
					mustWrite(t, ".agnostic-ai/settings/approval.yaml", "permissions:\n  deny: ['Bash(git status)']\n")
					wantDecision = "deny"
				}
				report, err := collectRTKReport("git status")
				if err != nil {
					t.Fatal(err)
				}
				if report.Original.Decision != wantDecision || len(report.Original.Rules) != 1 || len(report.Hooks) != 1 {
					t.Errorf("settings alias duplicated or changed authority: decision=%+v hooks=%v", report.Original, report.Hooks)
				}
				if strings.Contains(strings.Join(report.Warnings, "\n"), "multiple known RTK handlers") {
					t.Errorf("one handler reported as multiple owners: %v", report.Warnings)
				}
			})
		}
	}
}
