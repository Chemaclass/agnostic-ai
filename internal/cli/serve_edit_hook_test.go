package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type hookEditTestReply struct {
	OK          bool   `json:"ok"`
	File        string `json:"file"`
	Spec        string `json:"spec"`
	SyncCommand string `json:"syncCommand"`
	Expected    string `json:"expected"`
	Error       string `json:"error"`
}

func hookEditRequest(t *testing.T, h http.Handler, request map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	return postEdit(t, h, editHookPath, string(body), nil)
}

func hookEditBody(name, on, command string) map[string]any {
	return map[string]any{"action": "preview", "audience": "shared", "name": name, "on": on, "command": command}
}

func hookEditPreview(t *testing.T, h http.Handler, request map[string]any) hookEditTestReply {
	t.Helper()
	rec := hookEditRequest(t, h, request)
	if rec.Code != http.StatusOK {
		t.Fatalf("preview = %d %s", rec.Code, rec.Body)
	}
	var reply hookEditTestReply
	if err := json.Unmarshal(rec.Body.Bytes(), &reply); err != nil {
		t.Fatal(err)
	}
	if reply.Expected == "" {
		t.Fatal("preview lacks its expected revision")
	}
	return reply
}

func hookEditApply(t *testing.T, h http.Handler, request map[string]any, preview hookEditTestReply) hookEditTestReply {
	t.Helper()
	request["action"], request["expected"] = "apply", preview.Expected
	rec := hookEditRequest(t, h, request)
	if rec.Code != http.StatusOK {
		t.Fatalf("apply = %d %s", rec.Code, rec.Body)
	}
	var reply hookEditTestReply
	if err := json.Unmarshal(rec.Body.Bytes(), &reply); err != nil {
		t.Fatal(err)
	}
	if !reply.OK {
		t.Errorf("apply lacks ok: %s", rec.Body)
	}
	return reply
}

func TestServeEditHook_PreviewThenApplyWritesTheCanonicalSpec(t *testing.T) {
	project := scopeProject(t)
	h := newEditHandler(project, false)
	request := hookEditBody("format-on-save", "after-edit", "npm run format -- $FILE")
	request["description"] = "Format the edited file."
	request["timeout"] = 30
	preview := hookEditPreview(t, h, request)
	want := "name: format-on-save\ndescription: Format the edited file.\non: after-edit\ncommand: npm run format -- $FILE\ntimeout: 30\n"
	if preview.File != ".agnostic-ai/hooks/format-on-save.yaml" || preview.Spec != want || preview.SyncCommand != "agnostic-ai sync" {
		t.Errorf("preview = %#v\nwant spec:\n%s", preview, want)
	}
	if _, err := os.Stat(filepath.Join(project, preview.File)); !os.IsNotExist(err) {
		t.Fatalf("preview wrote the hook: %v", err)
	}
	reply := hookEditApply(t, h, request, preview)
	if reply.File != preview.File {
		t.Errorf("apply file = %q", reply.File)
	}
	if got := readOptional(t, filepath.Join(project, preview.File)); got != want {
		t.Errorf("written spec = %q, want %q", got, want)
	}
	scope, err := loadCheckScope(false)
	if err != nil {
		t.Fatal(err)
	}
	if issues := scopeValidationIssues(scope); len(issues) > 0 {
		t.Errorf("validate reports %v", issues)
	}
	found := false
	for _, e := range scope.bundle.Hooks {
		if e.Name == "format-on-save" && e.Meta["on"] == "after-edit" {
			found = true
		}
	}
	if !found {
		t.Errorf("reloaded hooks = %v", scope.bundle.Hooks)
	}
}

func TestServeEditHook_KeepsAMultilineCommandAndTheMatch(t *testing.T) {
	project := scopeProject(t)
	h := newEditHandler(project, false)
	request := hookEditBody("guard-shell", "before-tool", "echo 'a: b' # not a comment\nexit 2")
	request["match"] = "shell"
	preview := hookEditPreview(t, h, request)
	if !strings.Contains(preview.Spec, "on: before-tool\nmatch: shell\ncommand: |-\n") {
		t.Errorf("preview spec = %s", preview.Spec)
	}
	hookEditApply(t, h, request, preview)
	scope, err := loadCheckScope(false)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range scope.bundle.Hooks {
		if e.Name == "guard-shell" {
			if e.Meta["command"] != "echo 'a: b' # not a comment\nexit 2" || e.Meta["match"] != "shell" {
				t.Errorf("reloaded meta = %v", e.Meta)
			}
			return
		}
	}
	t.Error("hook not loaded")
}

func TestServeEditHook_UsesTheProjectsConfiguredHookSource(t *testing.T) {
	project := scopeProject(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\nsources:\n  hooks: shared/hooks\n")
	h := newEditHandler(project, false)
	request := hookEditBody("lint-on-stop", "stop", "make lint")
	preview := hookEditPreview(t, h, request)
	if preview.File != "shared/hooks/lint-on-stop.yaml" {
		t.Errorf("destination = %q", preview.File)
	}
	hookEditApply(t, h, request, preview)
	if got := readOptional(t, "shared/hooks/lint-on-stop.yaml"); got != preview.Spec {
		t.Errorf("spec = %q", got)
	}
}

func TestServeEditHook_GlobalSavesUnderTheGlobalSource(t *testing.T) {
	silence(t)
	source := editHome(t)
	h := newEditHandler(source, true)
	request := hookEditBody("note-start", "session-start", "echo hi")
	preview := hookEditPreview(t, h, request)
	if preview.File != "hooks/note-start.yaml" || preview.SyncCommand != "agnostic-ai sync --global" {
		t.Errorf("global preview = %#v", preview)
	}
	hookEditApply(t, h, request, preview)
	if got := readOptional(t, filepath.Join(source, "hooks", "note-start.yaml")); got != preview.Spec {
		t.Errorf("global spec = %q", got)
	}
}

func TestServeEditHook_LocalSaveStaysOutOfGit(t *testing.T) {
	project := gitScopeProject(t)
	h := newEditHandler(project, false)
	request := hookEditBody("my-format", "after-edit", "gofmt -w .")
	request["audience"] = "personal"
	preview := hookEditPreview(t, h, request)
	if preview.File != ".agnostic-ai/local/hooks/my-format.yaml" {
		t.Errorf("local destination = %q", preview.File)
	}
	hookEditApply(t, h, request, preview)
	assertGitHides(t, project, preview.File)
	scope, err := loadCheckScope(false)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range scope.bundle.Hooks {
		if e.Name == "my-format" && e.Layer == layerNameProjectUser {
			return
		}
	}
	t.Errorf("local hook not loaded: %v", scope.bundle.Hooks)
}

func TestServeEditHook_RefusesInvalidRequests(t *testing.T) {
	project := scopeProject(t)
	h := newEditHandler(project, false)
	for _, tc := range []struct {
		name   string
		change func(map[string]any)
		want   string
	}{
		{"bad action", func(r map[string]any) { r["action"] = "write" }, "send action preview or apply"},
		{"missing name", func(r map[string]any) { r["name"] = "" }, "send action preview or apply"},
		{"name with a slash", func(r map[string]any) { r["name"] = "../escape" }, "lowercase letters, digits, and hyphens"},
		{"uppercase name", func(r map[string]any) { r["name"] = "Format" }, "lowercase letters, digits, and hyphens"},
		{"git folder name", func(r map[string]any) { r["name"] = ".git" }, "lowercase letters, digits, and hyphens"},
		{"unknown stage", func(r map[string]any) { r["on"] = "PostToolUse" }, "on must be one of"},
		{"blank command", func(r map[string]any) { r["command"] = "  \n" }, "command is required"},
		{"match on after-edit", func(r map[string]any) { r["match"] = "shell" }, "match applies only to before-tool and after-tool"},
		{"unknown match", func(r map[string]any) { r["on"], r["match"] = "before-tool", "files" }, "match must be one of"},
		{"zero timeout", func(r map[string]any) { r["timeout"] = 0 }, "timeout must be a whole number of seconds above zero"},
		{"negative timeout", func(r map[string]any) { r["timeout"] = -5 }, "timeout must be a whole number of seconds above zero"},
		{"fractional timeout", func(r map[string]any) { r["timeout"] = 1.5 }, "send action preview or apply"},
		{"bad audience", func(r map[string]any) { r["audience"] = "team" }, "audience"},
		{"bearer token in command", func(r map[string]any) {
			r["command"] = `curl -H "Authorization: Bearer ghp_abcdefghijklmnopqrstuvwxyz0123456789" https://example.com`
		}, "command contains a credential"},
		{"token assignment in command", func(r map[string]any) {
			r["command"] = "GITHUB_TOKEN=ghp_abcdefghijklmnopqrstuvwxyz0123456789 ./notify.sh"
		}, "command contains a credential"},
		{"api key header in command", func(r map[string]any) {
			r["command"] = `curl -H "X-Api-Key: abc123def456" https://example.com`
		}, "command contains a credential"},
		{"password assignment in command", func(r map[string]any) { r["command"] = "PASSWORD=hunter2hunter2 ./deploy.sh" }, "command contains a credential"},
		{"bearer token inside a substitution", func(r map[string]any) {
			r["command"] = `status=$(curl -s -H "Authorization: Bearer ghp_abcdefghijklmnopqrstuvwxyz0123456789" https://api.github.com/user)`
		}, "command contains a credential"},
		{"api key inside a substitution", func(r map[string]any) {
			r["command"] = `echo $(curl -H "X-Api-Key: abc123def456" https://example.com)`
		}, "command contains a credential"},
		{"token echoed in a substitution", func(r map[string]any) {
			r["command"] = "TOKEN=$(echo ghp_abcdefghijklmnopqrstuvwxyz0123456789) ./notify.sh"
		}, "command contains a credential"},
		{"password assignment inside a substitution", func(r map[string]any) { r["command"] = "out=$(PASSWORD=hunter2hunter2 ./deploy.sh)" }, "command contains a credential"},
		{"token inside backticks", func(r map[string]any) {
			r["command"] = "status=`curl -H \"Authorization: Bearer ghp_abcdefghijklmnopqrstuvwxyz0123456789\" https://x`"
		}, "command contains a credential"},
		{"bearer value in description", func(r map[string]any) { r["description"] = "Authorization: Bearer abc123def456ghi789jklmn" }, "description contains a credential"},
		{"password in description", func(r map[string]any) { r["description"] = "password=hunter2hunter2" }, "description contains a credential"},
		{"url password in description", func(r map[string]any) { r["description"] = "https://user:hunter2hunter2@example.com" }, "description contains a credential"},
		{"openai key as name", func(r map[string]any) { r["name"] = "sk-abcdefghij0123456789" }, "name contains a credential"},
		{"gitlab token as name", func(r map[string]any) { r["name"] = "glpat-abcdefgh12345678" }, "name contains a credential"},
		{"slack token as name", func(r map[string]any) { r["name"] = "xoxb-1234-5678-abcdefgh" }, "name contains a credential"},
		{"token in description", func(r map[string]any) {
			r["description"] = "uses ghp_abcdefghijklmnopqrstuvwxyz0123456789"
		}, "description contains a credential"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := hookEditBody("format-on-save", "after-edit", "npm run format")
			tc.change(request)
			rec := hookEditRequest(t, h, request)
			if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), tc.want) {
				t.Errorf("refusal = %d %s, want 400 with %q", rec.Code, rec.Body, tc.want)
			}
		})
	}
	if _, err := os.Stat(filepath.Join(project, ".agnostic-ai", "hooks")); !os.IsNotExist(err) {
		t.Errorf("a refused request wrote hooks: %v", err)
	}
}

func TestServeEditHook_RefusesAMatchAConfiguredToolLacks(t *testing.T) {
	project := scopeProject(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, codex]\n")
	h := newEditHandler(project, false)
	request := hookEditBody("read-guard", "before-tool", "exit 0")
	request["match"] = "read"
	rec := hookEditRequest(t, h, request)
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "codex") {
		t.Errorf("unsupported match = %d %s", rec.Code, rec.Body)
	}
	if _, err := os.Stat(filepath.Join(project, ".agnostic-ai", "hooks", "read-guard.yaml")); !os.IsNotExist(err) {
		t.Errorf("refused hook was written: %v", err)
	}
}

func TestServeEditHook_RefusesAnExistingName(t *testing.T) {
	for _, tc := range []struct{ name, file, body string }{
		{"same file", ".agnostic-ai/hooks/format-on-save.yaml", "name: format-on-save\non: stop\ncommand: echo old\n"},
		{"other case", ".agnostic-ai/hooks/Format-On-Save.yaml", "on: stop\ncommand: echo old\n"},
		{"other file, same name", ".agnostic-ai/hooks/older.yaml", "name: format-on-save\non: stop\ncommand: echo old\n"},
		{"local layer", ".agnostic-ai/local/hooks/format-on-save.yaml", "on: stop\ncommand: echo old\n"},
		{"yml file", ".agnostic-ai/hooks/format-on-save.yml", "on: stop\ncommand: echo old\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			project := scopeProject(t)
			writeFile(t, tc.file, tc.body)
			h := newEditHandler(project, false)
			rec := hookEditRequest(t, h, hookEditBody("format-on-save", "after-edit", "npm run format"))
			if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "A hook named format-on-save already exists") {
				t.Errorf("existing name = %d %s", rec.Code, rec.Body)
			}
			if got := readOptional(t, tc.file); got != tc.body {
				t.Errorf("existing hook changed: %q", got)
			}
		})
	}
}

func TestServeEditHook_RefusesAHookCreatedAfterPreview(t *testing.T) {
	project := scopeProject(t)
	h := newEditHandler(project, false)
	request := hookEditBody("format-on-save", "after-edit", "npm run format")
	preview := hookEditPreview(t, h, request)
	file := filepath.Join(project, preview.File)
	before := "on: stop\ncommand: echo other\n"
	writeFile(t, file, before)
	request["action"], request["expected"] = "apply", preview.Expected
	rec := hookEditRequest(t, h, request)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "already exists") {
		t.Errorf("created after preview = %d %s", rec.Code, rec.Body)
	}
	if got := readOptional(t, file); got != before {
		t.Errorf("concurrent hook changed: %q", got)
	}
}

func TestServeEditHook_RefusesAnApplyWithoutAMatchingPreview(t *testing.T) {
	project := scopeProject(t)
	h := newEditHandler(project, false)
	request := hookEditBody("format-on-save", "after-edit", "npm run format")
	request["action"] = "apply"
	if rec := hookEditRequest(t, h, request); rec.Code != http.StatusConflict {
		t.Errorf("apply without preview = %d %s, want 409", rec.Code, rec.Body)
	}
	request["action"] = "preview"
	preview := hookEditPreview(t, h, request)
	request["command"] = "rm -rf /"
	request["action"], request["expected"] = "apply", preview.Expected
	if rec := hookEditRequest(t, h, request); rec.Code != http.StatusConflict {
		t.Errorf("apply of a different hook = %d %s, want 409", rec.Code, rec.Body)
	}
	if _, err := os.Stat(filepath.Join(project, preview.File)); !os.IsNotExist(err) {
		t.Errorf("stale apply wrote the hook: %v", err)
	}
}

func TestServeEditHook_RefusesRequestsAnotherSiteCouldSend(t *testing.T) {
	project := scopeProject(t)
	h := newEditHandler(project, false)
	body, err := json.Marshal(hookEditBody("format-on-save", "after-edit", "npm run format"))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*http.Request)
		status int
	}{
		{"foreign origin", func(r *http.Request) { r.Header.Set("Origin", "https://other.example") }, http.StatusForbidden},
		{"form", func(r *http.Request) { r.Header.Set("Content-Type", "application/x-www-form-urlencoded") }, http.StatusUnsupportedMediaType},
		{"foreign host", func(r *http.Request) { r.Host = "other.example:4310" }, http.StatusMisdirectedRequest},
		{"GET", func(r *http.Request) { r.Method = http.MethodGet }, http.StatusMethodNotAllowed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if rec := postEdit(t, h, editHookPath, string(body), tc.mutate); rec.Code != tc.status {
				t.Errorf("guard = %d %s, want %d", rec.Code, rec.Body, tc.status)
			}
		})
	}
	if _, err := os.Stat(filepath.Join(project, ".agnostic-ai", "hooks")); !os.IsNotExist(err) {
		t.Errorf("refused request wrote hooks: %v", err)
	}
}

func TestServeEditHook_KeepsTheCommandByteForByte(t *testing.T) {
	for _, tc := range []struct{ name, command string }{
		{"escaped trailing space", `rm -- release\ `},
		{"trailing line continuation", "echo one \\\n"},
		{"trailing newline", "make lint\n"},
		{"trailing tab", "make lint\t"},
		{"leading space", "  make lint"},
		{"carriage return line ends", "make lint\r\nmake test"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			project := scopeProject(t)
			h := newEditHandler(project, false)
			request := hookEditBody("keep-bytes", "stop", tc.command)
			preview := hookEditPreview(t, h, request)
			hookEditApply(t, h, request, preview)
			if got := readOptional(t, filepath.Join(project, preview.File)); got != preview.Spec {
				t.Errorf("written spec = %q, preview = %q", got, preview.Spec)
			}
			scope, err := loadCheckScope(false)
			if err != nil {
				t.Fatal(err)
			}
			for _, e := range scope.bundle.Hooks {
				if e.Name == "keep-bytes" {
					if e.Meta["command"] != tc.command {
						t.Errorf("reloaded command = %q, want %q\n%s", e.Meta["command"], tc.command, preview.Spec)
					}
					return
				}
			}
			t.Error("hook not loaded")
		})
	}
}

func TestServeEditHook_RefusesTheNameOfABuiltinHookForAnotherTool(t *testing.T) {
	project := scopeProject(t)
	h := newEditHandler(project, false)
	rec := hookEditRequest(t, h, hookEditBody("memory-windsurf-session-start", "session-start", "echo mine"))
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "A hook named memory-windsurf-session-start already exists") {
		t.Errorf("built-in name = %d %s", rec.Code, rec.Body)
	}
}

func TestServeEditHook_PreviewRefusesWhatValidateWouldReport(t *testing.T) {
	project := scopeProject(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [aider]\n")
	h := newEditHandler(project, false)
	rec := hookEditRequest(t, h, hookEditBody("lint-on-stop", "stop", "make lint"))
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "lint-on-stop.yaml") {
		t.Errorf("preview with no hook tool = %d %s", rec.Code, rec.Body)
	}
}

func TestServeEditHook_RefusesMatchAny(t *testing.T) {
	project := scopeProject(t)
	request := hookEditBody("guard", "before-tool", "exit 0")
	request["match"] = "any"
	rec := hookEditRequest(t, newEditHandler(project, false), request)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "match must be one of shell, edit, read, web") {
		t.Errorf("match any = %d %s", rec.Code, rec.Body)
	}
}

func TestServeEditHook_LeavesOutTheToolsAStageLacks(t *testing.T) {
	project := scopeProject(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, cursor]\n")
	h := newEditHandler(project, false)
	if rec := hookEditRequest(t, h, hookEditBody("format-on-save", "after-edit", "npm run format")); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("without exclusion = %d %s, want 422", rec.Code, rec.Body)
	}
	request := hookEditBody("format-on-save", "after-edit", "npm run format")
	request["targetsExclude"] = []string{"cursor"}
	got := hookEditPreview(t, h, request)
	if !strings.Contains(got.Spec, "targets-exclude:\n  - cursor\n") {
		t.Errorf("spec = %q", got.Spec)
	}
	hookEditApply(t, h, request, got)
	request = hookEditBody("other", "after-edit", "npm run format")
	request["targetsExclude"] = []string{"../x"}
	if rec := hookEditRequest(t, h, request); rec.Code != http.StatusBadRequest {
		t.Errorf("bad tool name = %d %s, want 400", rec.Code, rec.Body)
	}
}

func TestServeEditHook_ChecksTheToolsItLeavesOut(t *testing.T) {
	scopeProject(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, cursor, aider]\n")
	h := newEditHandler(".", false)
	for _, tc := range []struct {
		exclude []string
		problem string
	}{
		{[]string{"cursor", "bogus"}, "bogus is not a configured tool"},
		{[]string{"cursor", "cursor"}, "cursor is listed twice"},
		{[]string{"claude", "cursor"}, "leaves the hook no tool to run on"},
	} {
		request := hookEditBody("note-start", "session-start", "echo hi")
		request["targetsExclude"] = tc.exclude
		if rec := hookEditRequest(t, h, request); rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), tc.problem) {
			t.Errorf("exclude %v = %d %s, want 422 with %q", tc.exclude, rec.Code, rec.Body, tc.problem)
		}
	}
	request := hookEditBody("note-start", "session-start", "echo hi")
	request["targetsExclude"] = []string{"aider"}
	if got := hookEditPreview(t, h, request); strings.Contains(got.Spec, "targets-exclude") {
		t.Errorf("a tool that writes no hooks was written out: %q", got.Spec)
	}
}

func TestServeEditHook_GlobalAcceptsAToolTheGlobalSetupWritesNoHooksFor(t *testing.T) {
	silence(t)
	source := editHome(t)
	writeFile(t, filepath.Join(source, "agnostic-ai.yaml"), "targets:\n  - claude\n  - windsurf\n")
	h := newEditHandler(source, true)
	request := hookEditBody("note-start", "session-start", "echo hi")
	request["targetsExclude"] = []string{"windsurf"}
	got := hookEditPreview(t, h, request)
	if strings.Contains(got.Spec, "targets-exclude") {
		t.Errorf("spec = %q, want no exclusion for a tool with no global hooks", got.Spec)
	}
}

func TestServeEditHook_KeepsACommandThatReadsTheCredentialAtRunTime(t *testing.T) {
	h := newEditHandler(scopeProject(t), false)
	for _, command := range []string{
		`curl -H "Authorization: Bearer $GITHUB_TOKEN" https://example.com`,
		`curl -H "Authorization: Bearer ${GITHUB_TOKEN}" https://example.com`,
		`curl -H "Authorization: Bearer $(gh auth token)" https://example.com`,
		`TOKEN=$(cat ~/.token) ./notify.sh`,
		`curl "https://hooks.example.com/notify?token=$HOOK_TOKEN"`,
		`PASSWORD=$1 ./notify.sh`,
		"curl -H \"Authorization: Bearer `gh auth token`\" https://example.com",
		`TOKEN=$(cat $(pwd)/t) ./notify.sh`,
		`TOKEN=$(jq -r .token <(cat f)) ./notify.sh`,
	} {
		got := hookEditPreview(t, h, hookEditBody("notify", "session-start", command))
		if !strings.Contains(got.Spec, "notify") {
			t.Errorf("spec = %q for %q", got.Spec, command)
		}
	}
}
