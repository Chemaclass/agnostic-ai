package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/approvals"
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
		writeFile(t, filepath.Join(source, config.ConfigFileName), shared)
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

// fakeApprovalRoutes gives claude a yolo hook and codex a review route,
// whatever the table holds.
func fakeApprovalRoutes(t *testing.T) {
	t.Helper()
	saved, savedGOOS := approvalRouteFor, approvalsGOOS
	t.Cleanup(func() { approvalRouteFor, approvalsGOOS = saved, savedGOOS })
	approvalsGOOS = "linux"
	approvalRouteFor = func(tool string) approvals.Route {
		switch tool {
		case "claude":
			return approvals.Route{Target: tool, Reason: "not built yet", Yolo: &approvals.Mechanism{Name: "Claude yolo route"}}
		case "codex":
			return approvals.Route{Target: tool, Reason: "not built yet", Review: &approvals.Mechanism{Name: "Codex Auto-review"}}
		}
		return approvals.RouteFor(tool)
	}
}

func TestApprovalsStatus_RoutePerTool(t *testing.T) {
	fakeApprovalRoutes(t)
	source := approvalsHome(t, "version: 1\ntargets: [claude, codex, cursor]\napprovals:\n  mode: yolo\n  tools:\n    codex: review\n")
	writeCodexReviewerOn(t, source)

	got := approvalsStatusJSON(t)

	claude := toolStatus(t, got, "claude")
	if claude.Mode != config.ApprovalsYolo || claude.Effective != config.ApprovalsYolo || claude.Route != "Claude yolo route" || claude.From != "approvals.mode" || claude.Yolo != "Claude yolo route" || claude.Review != config.ApprovalsAsk {
		t.Errorf("claude: %+v", claude)
	}
	codex := toolStatus(t, got, "codex")
	if codex.Mode != config.ApprovalsReview || codex.Effective != config.ApprovalsReview || codex.From != "approvals.tools.codex" {
		t.Errorf("codex: %+v", codex)
	}
	cursor := toolStatus(t, got, "cursor")
	if cursor.Mode != config.ApprovalsYolo || cursor.Effective != config.ApprovalsAsk || cursor.Route != "" || cursor.Reason == "" || cursor.Yolo != config.ApprovalsAsk {
		t.Errorf("cursor: %+v", cursor)
	}

	out, err := runCLI(t, "approvals", "status")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "falls back to ask: "+cursor.Reason) {
		t.Errorf("no fallback reason for cursor:\n%s", out)
	}
}

func TestApprovalsStatus_JSONHasEveryRouteField(t *testing.T) {
	approvalsHome(t, "version: 1\ntargets: [claude, copilot]\napprovals:\n  mode: yolo\n")

	out, err := runCLI(t, "approvals", "status", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var raw struct {
		Tools []map[string]any `json:"tools"`
	}
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		t.Fatal(err)
	}
	for _, tool := range raw.Tools {
		for _, key := range []string{"tool", "mode", "effective", "from", "yolo", "review", "reason"} {
			if _, ok := tool[key]; !ok {
				t.Errorf("%v has no %s", tool["tool"], key)
			}
		}
	}
	copilot := toolStatus(t, approvalsStatusJSON(t), "copilot")
	if copilot.Reason != approvals.RouteFor("copilot").Reason {
		t.Errorf("copilot: %+v", copilot)
	}
}

func TestApprovalsStatus_WindowsFallsBack(t *testing.T) {
	saved := approvalsGOOS
	t.Cleanup(func() { approvalsGOOS = saved })
	approvalsGOOS = "windows"
	approvalsHome(t, "version: 1\ntargets: [claude, codex]\napprovals:\n  mode: yolo\n")

	out, err := runCLI(t, "approvals", "status")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(out, "falls back to ask: Windows") != 2 {
		t.Errorf("want both tools to fall back on Windows:\n%s", out)
	}
}

func TestApprovalsStatus_HookNotInstalled(t *testing.T) {
	approvalsHome(t, "version: 1\ntargets: [claude]\napprovals:\n  mode: yolo\n")
	t.Setenv("CLAUDE_CONFIG_DIR", "")

	claude := toolStatus(t, approvalsStatusJSON(t), "claude")
	if claude.Effective != config.ApprovalsAsk || !strings.Contains(claude.Reason, "sync --global") {
		t.Errorf("claude: %+v", claude)
	}
}

func TestApprovalsStatus_ClaudeHookInstalled(t *testing.T) {
	approvalsHome(t, "version: 1\ntargets: [claude]\napprovals:\n  mode: yolo\n")
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	home, _ := os.UserHomeDir()
	writeFile(t, filepath.Join(home, ".claude", "settings.json"), `{"hooks":{"PermissionRequest":[{"hooks":[{"type":"command","command":"agnostic-ai hook approve --target claude"}]}]}}`)
	saved := approvalsGOOS
	t.Cleanup(func() { approvalsGOOS = saved })
	approvalsGOOS = "linux"

	claude := toolStatus(t, approvalsStatusJSON(t), "claude")
	if claude.Effective != config.ApprovalsYolo || claude.Route != approvals.MechanismPermissionHook || claude.MinVersion == "" {
		t.Errorf("claude: %+v", claude)
	}
}

func TestApprovalsStatus_ClaudeHookOffOrElsewhere(t *testing.T) {
	saved := approvalsGOOS
	t.Cleanup(func() { approvalsGOOS = saved })
	approvalsGOOS = "linux"
	for name, body := range map[string]string{
		"all hooks off":         `{"disableAllHooks":true,"hooks":{"PermissionRequest":[{"hooks":[{"type":"command","command":"agnostic-ai hook approve --target claude"}]}]}}`,
		"under another event":   `{"hooks":{"PreToolUse":[{"hooks":[{"type":"command","command":"agnostic-ai hook approve --target claude"}]}]}}`,
		"settings do not parse": `{"hooks":`,
	} {
		t.Run(name, func(t *testing.T) {
			approvalsHome(t, "version: 1\ntargets: [claude]\napprovals:\n  mode: yolo\n")
			t.Setenv("CLAUDE_CONFIG_DIR", "")
			home, _ := os.UserHomeDir()
			writeFile(t, filepath.Join(home, ".claude", "settings.json"), body)
			claude := toolStatus(t, approvalsStatusJSON(t), "claude")
			if claude.Effective != config.ApprovalsAsk || claude.Reason == "" {
				t.Errorf("claude: %+v", claude)
			}
		})
	}
}

func TestApprovalsStatus_CodexHookNotTrusted(t *testing.T) {
	approvalsHome(t, "version: 1\ntargets: [codex]\napprovals:\n  mode: yolo\n")
	codexHome := t.TempDir()
	t.Setenv("CODEX_HOME", codexHome)
	writeFile(t, filepath.Join(codexHome, "hooks.json"), `{"hooks":{"PermissionRequest":[{"hooks":[{"type":"command","command":"agnostic-ai hook approve --target codex","timeout":10}]}]}}`)
	saved := approvalsGOOS
	t.Cleanup(func() { approvalsGOOS = saved })
	approvalsGOOS = "linux"

	codex := toolStatus(t, approvalsStatusJSON(t), "codex")
	if codex.Effective != config.ApprovalsAsk || !strings.Contains(codex.Reason, "/hooks") {
		t.Errorf("codex: %+v", codex)
	}
	out, err := runCLI(t, "approvals", "status")
	if err != nil || !strings.Contains(out, "Codex has not trusted the hook") {
		t.Errorf("status does not name the trust step: %v\n%s", err, out)
	}
}

func TestApprovals_RejectsUnknownArguments(t *testing.T) {
	approvalsHome(t, "")
	out, err := runCLI(t, "approvals", "yolo")
	if err == nil || !strings.Contains(out+err.Error(), "yolo") {
		t.Fatalf("approvals yolo = %v\n%s; want an error naming the argument", err, out)
	}
	if _, err := runCLI(t, "approvals"); err != nil {
		t.Errorf("approvals alone: %v", err)
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
	writeFile(t, config.ConfigFileName, "version: 1\napprovals:\n  mode: ask\n")

	claude := toolStatus(t, approvalsStatusJSON(t), "claude")

	if claude.Effective != config.ApprovalsAsk || !strings.HasSuffix(claude.From, config.ConfigFileName) {
		t.Errorf("claude: %+v", claude)
	}
}

func TestApprovalsStatus_ProjectFileCannotLoosen(t *testing.T) {
	approvalsHome(t, "version: 1\ntargets: [claude]\n")
	writeFile(t, config.ConfigFileName, "version: 1\napprovals:\n  mode: yolo\n")

	claude := toolStatus(t, approvalsStatusJSON(t), "claude")

	if claude.Effective != config.ApprovalsAsk {
		t.Errorf("claude: %+v", claude)
	}
}

func TestApprovalsAsk_WritesTheLocalHomeConfig(t *testing.T) {
	fakeApprovalRoutes(t)
	source := approvalsHome(t, "version: 1\ntargets: [claude, codex]\napprovals:\n  mode: yolo\n  tools:\n    codex: review\n")
	writeCodexReviewerOn(t, source)
	local := filepath.Join(source, "local", config.ConfigFileName)
	writeFile(t, local, "# mine\nmodels:\n  fast:\n    claude: haiku\napprovals:\n  tools:\n    codex: review\n  projects:\n    /tmp/x: yolo\n  always-ask:\n    - \"Bash(make deploy*)\"\n")

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
	writeFile(t, filepath.Join(source, config.ConfigFileName), "version: 1\ntargets: [claude]\napprovals:\n  mode: yolo\n")
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
	writeFile(t, config.ConfigFileName, cfg)
	writeFile(t, ".agnostic-ai/skills/review/SKILL.md", "---\nname: review\ndescription: Review code.\n---\nReview.\n")
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
	writeFile(t, filepath.Join(source, ".agnostic-ai/skills/review/SKILL.md"), "---\nname: review\ndescription: Review code.\n---\nReview.\n")
	testutil.Chdir(t, source)

	if out, err := runCLI(t, "validate"); err != nil {
		t.Fatalf("validate in the home folder: %v\n%s", err, out)
	}
}

func TestApprovalsAsk_WritesDespiteABrokenConfig(t *testing.T) {
	source := approvalsHome(t, "version: 1\napprovals:\n  mode: yollo\n")

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

func TestApprovalsAskYAML(t *testing.T) {
	cases := map[string]struct{ in, want, err string }{
		"comments only":  {in: "# mine\n", want: "# mine\napprovals:\n  mode: ask\n"},
		"document start": {in: "---\n", want: "---\napprovals:\n  mode: ask\n"},
		"no newline":     {in: "targets: [claude]", want: "targets: [claude]\napprovals:\n  mode: ask\n"},
		"flow mapping":   {in: "{targets: [claude]}\n", want: "{targets: [claude], approvals: {mode: ask}}\n"},
		"two documents":  {in: "targets: [claude]\n---\nother: 1\n", err: "more than one YAML document"},
		"an anchor":      {in: "approvals: &a {mode: yolo}\nmodels: {x: *a}\n", err: "anchor"},
		"a list":         {in: "- a\n", err: "not a mapping"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := approvalsAskYAML([]byte(c.in))
			if c.err != "" {
				if err == nil || !strings.Contains(err.Error(), c.err) {
					t.Fatalf("got %v, want an error with %q", err, c.err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != c.want {
				t.Fatalf("got:\n%s\nwant:\n%s", got, c.want)
			}
		})
	}
}

func TestValidateGlobal_ReportsABrokenApprovalsBlock(t *testing.T) {
	source := approvalsHome(t, "version: 1\ntargets: [claude]\napprovals:\n  mode: yollo\n")
	writeFile(t, filepath.Join(source, "rules", "style.md"), "Be brief.\n")

	out, err := runCLI(t, "validate", "--global")

	if err == nil || !strings.Contains(out, "yollo") {
		t.Fatalf("validate --global passed or did not name the value: %v\n%s", err, out)
	}
}

func TestApprovalRoutes_ReviewMatchesNativeReviewers(t *testing.T) {
	for _, r := range approvals.Routes() {
		reviewer, ok := nativeReviewers[r.Target]
		if (r.Review != nil) != ok {
			t.Errorf("%s: route table review %+v, native reviewer %v", r.Target, r.Review, ok)
			continue
		}
		if ok && r.Review.Name != reviewer.name {
			t.Errorf("%s: route names %q, native reviewer %q", r.Target, r.Review.Name, reviewer.name)
		}
	}
}

func TestApprovalsStatus_KiroHook(t *testing.T) {
	saved := approvalsGOOS
	t.Cleanup(func() { approvalsGOOS = saved })
	approvalsGOOS = "linux"
	t.Setenv("KIRO_HOME", "")
	const hook = `{"version":"v1","hooks":[{"name":"approve","trigger":"PermissionRequest","timeout":10,"action":{"type":"command","command":"agnostic-ai hook approve --target kiro"}}]}`
	for name, tc := range map[string]struct {
		hook, rules      string
		routed, override bool
	}{
		"installed":     {hook: hook, routed: true},
		"override":      {hook: hook, routed: true, override: true},
		"not installed": {},
		"disabled":      {hook: strings.Replace(hook, `"timeout":10`, `"timeout":10,"enabled":false`, 1)},
		"other trigger": {hook: strings.Replace(hook, "PermissionRequest", "PreToolUse", 1)},
		"ask rule":      {hook: hook, rules: "rules:\n  - capability: shell\n    effect: ask\n"},
	} {
		t.Run(name, func(t *testing.T) {
			approvalsHome(t, "version: 1\ntargets: [kiro]\napprovals:\n  mode: yolo\n")
			home, _ := os.UserHomeDir()
			root := filepath.Join(home, ".kiro")
			if tc.override {
				root = filepath.Join(t.TempDir(), "kiro[profile]")
				t.Setenv("KIRO_HOME", root)
			}
			if tc.hook != "" {
				writeFile(t, filepath.Join(root, "hooks", "approve-permission-request.json"), tc.hook)
			}
			if tc.rules != "" {
				writeFile(t, filepath.Join(root, "settings", "permissions.yaml"), tc.rules)
			}
			kiro := toolStatus(t, approvalsStatusJSON(t), "kiro")
			if tc.routed != (kiro.Effective == config.ApprovalsYolo) || tc.routed && kiro.MinVersion != "3.4.0" || !tc.routed && kiro.Reason == "" {
				t.Errorf("kiro: %+v", kiro)
			}
		})
	}
}
