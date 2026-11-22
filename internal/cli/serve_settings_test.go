package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/approvals"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

var settingsTestSecret = companionSecret(strings.Repeat("k", minCompanionSecret))

// settingsFixture is a user-level settings handler on a temp home, with
// the companion's secret set.
func settingsFixture(t *testing.T, shared string) (*settingsHandler, string) {
	t.Helper()
	home, source := userAgentTestHome(t)
	t.Setenv(config.EnvApprovals, "")
	if shared != "" {
		writeFile(t, filepath.Join(source, config.ConfigFileName), shared)
	}
	marks := filepath.Join(home, ".config", "agnostic-ai", "conversation-marks.json")
	h := newSettingsHandler(true, newApprovalsReader(), func() string { return marks }, settingsTestSecret, "test")
	companion := filepath.Dir(marks)
	h.companionDir = func() (string, error) { return companion, nil }
	return h, source
}

func withSecret(secret companionSecret) func(*http.Request) {
	return func(r *http.Request) { r.Header.Set(companionSecretHeader, string(secret)) }
}

func getSettings(t *testing.T, h http.Handler, held string) (*httptest.ResponseRecorder, settingsView) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4310"+settingsViewPath, nil)
	if held != "" {
		req.Header.Set("If-None-Match", held)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var doc settingsView
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
			t.Fatalf("settings: %v\n%s", err, rec.Body)
		}
	}
	return rec, doc
}

func postSetting(t *testing.T, h http.Handler, body string, mutate func(*http.Request)) (*httptest.ResponseRecorder, settingsWriteReply) {
	t.Helper()
	rec := postEdit(t, h, settingsWritePath, body, mutate)
	var reply settingsWriteReply
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &reply); err != nil {
			t.Fatalf("write reply: %v\n%s", err, rec.Body)
		}
	}
	return rec, reply
}

func TestSettings_DocumentForAnEmptyHome(t *testing.T) {
	h, source := settingsFixture(t, "")
	rec, doc := getSettings(t, h, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET = %d %s", rec.Code, rec.Body)
	}
	if doc.SchemaVersion != settingsSchema || doc.ConfigRoot != source || doc.Approvals.Mode != config.ApprovalsAsk || doc.Approvals.ModeFile != "" || doc.Approvals.Companion {
		t.Errorf("document = %s", rec.Body)
	}
	if doc.Mailbox == nil || !doc.Mailbox.ApproveAgentSends || doc.Sync.Needed || doc.CanLoosen {
		t.Errorf("document = %s", rec.Body)
	}
	if strings.Contains(rec.Body.String(), string(settingsTestSecret)) {
		t.Error("the document carries the companion secret")
	}
	tag := rec.Header().Get("ETag")
	if again, _ := getSettings(t, h, tag); again.Code != http.StatusNotModified {
		t.Errorf("GET with the tag = %d", again.Code)
	}
}

func TestSettings_TagChangesWithEachFile(t *testing.T) {
	h, source := settingsFixture(t, "version: 1\n")
	rec, _ := getSettings(t, h, "")
	tag := rec.Header().Get("ETag")
	writeFile(t, filepath.Join(source, config.ConfigFileName), "version: 1\napprovals:\n  companion: true\n")
	rec, doc := getSettings(t, h, tag)
	if rec.Code != http.StatusOK || !doc.Approvals.Companion || doc.Approvals.ModeFile != config.ConfigFileName {
		t.Fatalf("after a home config change = %d %s", rec.Code, rec.Body)
	}
	tag = rec.Header().Get("ETag")
	store, _ := h.mailbox()
	writeFile(t, store.SettingsPath(), `{"approveAgentSends":false}`)
	rec, doc = getSettings(t, h, tag)
	if rec.Code != http.StatusOK || doc.Mailbox.ApproveAgentSends {
		t.Fatalf("after a mailbox change = %d %s", rec.Code, rec.Body)
	}
}

func TestSettings_ProjectServerAnswers404(t *testing.T) {
	h := newSettingsHandler(false, newApprovalsReader(), nil, settingsTestSecret, "test")
	for _, path := range []string{settingsViewPath, settingsWritePath, settingsSyncPath} {
		rec := postEdit(t, h, path, `{}`, withSecret(settingsTestSecret))
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s = %d", path, rec.Code)
		}
	}
}

func TestSettings_GuardTable(t *testing.T) {
	foreign := func(r *http.Request) {
		r.Header.Set("Origin", "https://example.com")
		withSecret(settingsTestSecret)(r)
	}
	wrong := withSecret(companionSecret(strings.Repeat("x", minCompanionSecret)))
	cases := []struct {
		name   string
		body   string
		mutate func(*http.Request)
		want   int
	}{
		{"yolo without proof", `{"key":"approvals.mode","value":"yolo"}`, nil, http.StatusForbidden},
		{"review with a wrong secret", `{"key":"approvals.mode","value":"review"}`, wrong, http.StatusForbidden},
		{"yolo from another origin", `{"key":"approvals.mode","value":"yolo"}`, foreign, http.StatusForbidden},
		{"companion on without proof", `{"key":"approvals.companion","value":true}`, nil, http.StatusForbidden},
		{"mailbox off without proof", `{"key":"mailbox.approveAgentSends","value":false}`, nil, http.StatusForbidden},
		{"ask without proof", `{"key":"approvals.mode","value":"ask"}`, nil, http.StatusOK},
		{"companion off without proof", `{"key":"approvals.companion","value":false}`, nil, http.StatusOK},
		{"mailbox on without proof", `{"key":"mailbox.approveAgentSends","value":true}`, nil, http.StatusOK},
		{"ask from another origin", `{"key":"approvals.mode","value":"ask"}`, foreign, http.StatusForbidden},
		{"unknown key", `{"key":"approvals.tools","value":{}}`, withSecret(settingsTestSecret), http.StatusBadRequest},
		{"bad mode", `{"key":"approvals.mode","value":"yollo"}`, withSecret(settingsTestSecret), http.StatusBadRequest},
		{"yolo with the secret", `{"key":"approvals.mode","value":"yolo"}`, withSecret(settingsTestSecret), http.StatusOK},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h, source := settingsFixture(t, "version: 1\n")
			rec, _ := postSetting(t, h, c.body, c.mutate)
			if rec.Code != c.want {
				t.Fatalf("POST %s = %d %s, want %d", c.body, rec.Code, rec.Body, c.want)
			}
			if c.want == http.StatusOK {
				return
			}
			if _, err := os.Stat(filepath.Join(source, "local")); !os.IsNotExist(err) {
				t.Errorf("a refused write created the local config: %v", err)
			}
			if got := readFile(t, filepath.Join(source, config.ConfigFileName)); got != "version: 1\n" {
				t.Errorf("a refused write changed the shared config: %q", got)
			}
		})
	}
}

func TestSettings_YoloOnACleanHomeAddsTheBuiltinAndAsksForASync(t *testing.T) {
	h, source := settingsFixture(t, "version: 1\ntargets: [claude]\n")
	rec, reply := postSetting(t, h, `{"key":"approvals.mode","value":"yolo","expected":"ask"}`, withSecret(settingsTestSecret))
	if rec.Code != http.StatusOK {
		t.Fatalf("POST yolo = %d %s", rec.Code, rec.Body)
	}
	local := filepath.Join(source, "local", config.ConfigFileName)
	data := readFile(t, local)
	if !strings.Contains(data, "mode: yolo") || !strings.Contains(data, "builtins: [approvals]") {
		t.Fatalf("local config = %q", data)
	}
	if len(reply.Changed) != 1 || reply.Changed[0] != "local/agnostic-ai.yaml" {
		t.Errorf("changed = %v", reply.Changed)
	}
	if reply.Settings.Approvals.Mode != config.ApprovalsYolo || !reply.Settings.Sync.Needed || !reply.Settings.CanLoosen {
		t.Errorf("document = %s", rec.Body)
	}
	if !strings.Contains(strings.Join(reply.Settings.Sync.Reasons, "\n"), "claude: the hook is not installed") {
		t.Errorf("reasons = %v", reply.Settings.Sync.Reasons)
	}
	status := approvalsStatusJSON(t)
	for _, tool := range status.Tools {
		if tool.Mode != config.ApprovalsYolo {
			t.Errorf("approvals status: %s asks %s", tool.Tool, tool.Mode)
		}
	}
	entries, err := approvals.ReadLog(approvals.LogPath(source))
	if err != nil || len(entries) != 1 || entries[0].Decision != approvals.Changed || entries[0].By != approvals.ByCompanion || entries[0].Detail != "approvals.mode: yolo" {
		t.Errorf("decision log = %+v, %v", entries, err)
	}
}

func TestSettings_CompanionOnWithoutTheHookAsksForASync(t *testing.T) {
	h, _ := settingsFixture(t, "version: 1\ntargets: [claude]\n")
	rec, reply := postSetting(t, h, `{"key":"approvals.companion","value":true,"expected":false}`, withSecret(settingsTestSecret))
	if rec.Code != http.StatusOK {
		t.Fatalf("POST companion = %d %s", rec.Code, rec.Body)
	}
	if reply.Settings.Approvals.Mode != config.ApprovalsAsk || !reply.Settings.Sync.Needed || !strings.Contains(strings.Join(reply.Settings.Sync.Reasons, "\n"), "claude: the hook is not installed") {
		t.Errorf("document = %s", rec.Body)
	}
}

func TestSettings_BuiltinAddedOnlyWhenMissing(t *testing.T) {
	const shared = "version: 1\ntargets: [claude]\nbuiltins: [approvals]\n"
	h, source := settingsFixture(t, shared)
	if rec, _ := postSetting(t, h, `{"key":"approvals.companion","value":true}`, withSecret(settingsTestSecret)); rec.Code != http.StatusOK {
		t.Fatalf("POST = %d %s", rec.Code, rec.Body)
	}
	if got := readFile(t, filepath.Join(source, "local", config.ConfigFileName)); got != "approvals:\n  companion: true\n" {
		t.Errorf("local config = %q", got)
	}
	if got := readFile(t, filepath.Join(source, config.ConfigFileName)); got != shared {
		t.Errorf("shared config = %q", got)
	}
}

func TestSettings_WriteLandsInTheBlockInEffect(t *testing.T) {
	const shared = "version: 1\nbuiltins: [approvals]\n# my approvals\napprovals:\n  mode: ask # for now\n  tools:\n    codex: review\n"
	h, source := settingsFixture(t, shared)
	if rec, _ := postSetting(t, h, `{"key":"approvals.mode","value":"yolo"}`, withSecret(settingsTestSecret)); rec.Code != http.StatusOK {
		t.Fatalf("POST = %d %s", rec.Code, rec.Body)
	}
	got := readFile(t, filepath.Join(source, config.ConfigFileName))
	for _, want := range []string{"# my approvals", "mode: yolo # for now", "codex: review", "builtins: [approvals]"} {
		if !strings.Contains(got, want) {
			t.Errorf("shared config lost %q:\n%s", want, got)
		}
	}
	if _, err := os.Stat(filepath.Join(source, "local", config.ConfigFileName)); !os.IsNotExist(err) {
		t.Errorf("the write created a local block: %v", err)
	}
	home, err := config.LoadHomeApprovals(source)
	if err != nil || home.Mode != config.ApprovalsYolo || home.Tools["codex"] != config.ApprovalsReview {
		t.Errorf("approvals = %+v, %v", home, err)
	}

	writeFile(t, filepath.Join(source, "local", config.ConfigFileName), "approvals:\n  mode: ask\n")
	rec, reply := postSetting(t, h, `{"key":"approvals.mode","value":"review"}`, withSecret(settingsTestSecret))
	if rec.Code != http.StatusOK {
		t.Fatalf("POST = %d %s", rec.Code, rec.Body)
	}
	if reply.Settings.Approvals.ModeFile != "local/agnostic-ai.yaml" || strings.Join(reply.Settings.Approvals.Shadowed, ",") != "approvals.mode,approvals.tools" {
		t.Errorf("document = %s", rec.Body)
	}
	if got := readFile(t, filepath.Join(source, "local", config.ConfigFileName)); got != "approvals:\n  mode: review\n" {
		t.Errorf("local config = %q", got)
	}
}

func TestSettings_AskMatchesTheChip(t *testing.T) {
	const shared = "version: 1\napprovals:\n  mode: yolo\n  always-ask: [\"Bash(make deploy*)\"]\n"
	h, source := settingsFixture(t, shared)
	local := filepath.Join(source, "local", config.ConfigFileName)
	if rec := postEdit(t, approvalsHandler{reader: newApprovalsReader()}, approvalsAskPath, `{"mode":"ask"}`, nil); rec.Code != http.StatusOK {
		t.Fatalf("chip = %d %s", rec.Code, rec.Body)
	}
	chip := readFile(t, local)
	if err := os.Remove(local); err != nil {
		t.Fatal(err)
	}
	if rec, _ := postSetting(t, h, `{"key":"approvals.mode","value":"ask"}`, nil); rec.Code != http.StatusOK {
		t.Fatalf("menu = %d %s", rec.Code, rec.Body)
	}
	if menu := readFile(t, local); menu != chip {
		t.Errorf("menu wrote %q, chip wrote %q", menu, chip)
	}
}

func TestSettings_ExpectedMismatchAnswersTheCurrentDocument(t *testing.T) {
	h, source := settingsFixture(t, "version: 1\napprovals:\n  mode: review\n")
	rec := postEdit(t, h, settingsWritePath, `{"key":"approvals.mode","value":"ask","expected":"yolo"}`, nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("POST = %d %s", rec.Code, rec.Body)
	}
	var refusal settingsRefusal
	if err := json.Unmarshal(rec.Body.Bytes(), &refusal); err != nil || refusal.Settings == nil || refusal.Settings.Approvals.Mode != config.ApprovalsReview {
		t.Errorf("refusal = %s", rec.Body)
	}
	if _, err := os.Stat(filepath.Join(source, "local", config.ConfigFileName)); !os.IsNotExist(err) {
		t.Errorf("a refused write changed a file: %v", err)
	}
}

func TestSettings_RefusesFilesItCannotEditSafely(t *testing.T) {
	for name, shared := range map[string]string{
		"two documents": "approvals:\n  mode: ask\n---\nother: 1\n",
		"a merge key":   "base: &b\n  mode: ask\napprovals:\n  <<: *b\n",
	} {
		t.Run(name, func(t *testing.T) {
			h, source := settingsFixture(t, shared)
			rec, _ := postSetting(t, h, `{"key":"approvals.companion","value":true}`, withSecret(settingsTestSecret))
			if rec.Code != http.StatusConflict {
				t.Fatalf("POST = %d %s", rec.Code, rec.Body)
			}
			if got := readFile(t, filepath.Join(source, config.ConfigFileName)); got != shared {
				t.Errorf("shared config = %q", got)
			}
		})
	}
}

func TestApprovalsSetYAML(t *testing.T) {
	cases := map[string]struct {
		in, key, want string
		value         any
	}{
		"empty file":       {in: "", key: "mode", value: "yolo", want: "approvals:\n  mode: yolo\n"},
		"no block":         {in: "# mine\ntargets: [claude]", key: "companion", value: true, want: "# mine\ntargets: [claude]\napprovals:\n  companion: true\n"},
		"empty block":      {in: "approvals:\n", key: "mode", value: "review", want: "approvals:\n  mode: review\n"},
		"keeps other keys": {in: "approvals:\n  mode: ask\n  tools: {codex: yolo}\n", key: "companion", value: false, want: "approvals:\n  mode: ask\n  tools: {codex: yolo}\n  companion: false\n"},
		"flow mapping":     {in: "{targets: [claude]}\n", key: "mode", value: "yolo", want: "{targets: [claude], approvals: {mode: yolo}}\n"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := approvalsSetYAML([]byte(c.in), c.key, c.value)
			if err != nil || string(got) != c.want {
				t.Fatalf("got %q, %v; want %q", got, err, c.want)
			}
		})
	}
	if _, err := approvalsSetYAML([]byte("approvals: &a {mode: ask}\n"), "mode", "yolo"); err == nil || !strings.Contains(err.Error(), "set approvals.mode: yolo by hand") {
		t.Errorf("anchor: %v", err)
	}
}

func TestConversationsList_TagCoversMailboxAndCompanionSettings(t *testing.T) {
	h, _ := mailboxHTTPFixture(t)
	tag := listTagOf(t, h, "")
	store, _ := h.mailboxStore()
	writeFile(t, store.SettingsPath(), `{"approveAgentSends":false}`)
	next := listTagOf(t, h, "")
	if next == tag {
		t.Error("the list tag missed a mailbox settings change")
	}
	writeFile(t, filepath.Join(filepath.Dir(h.scanner.MarksPath), companionSettingsFile), `{"smallCompanion":true}`)
	if listTagOf(t, h, "") == next {
		t.Error("the list tag missed a companion.json change")
	}
}

func TestSettings_TightensABlockThatDoesNotLoad(t *testing.T) {
	h, source := settingsFixture(t, "version: 1\napprovals:\n  mode: yolo\n  companion: true\n  nonsense: 1\n")
	if rec, _ := postSetting(t, h, `{"key":"approvals.companion","value":false}`, withSecret(settingsTestSecret)); rec.Code != http.StatusOK {
		t.Fatalf("POST = %d %s", rec.Code, rec.Body)
	}
	if got := readFile(t, filepath.Join(source, config.ConfigFileName)); !strings.Contains(got, "companion: false") {
		t.Errorf("shared config = %q", got)
	}
}

func TestSettings_RefusesAFileWithADocumentEndMarker(t *testing.T) {
	const shared = "version: 1\n...\n"
	h, source := settingsFixture(t, shared)
	if rec := postEdit(t, h, settingsWritePath, `{"key":"approvals.mode","value":"ask"}`, nil); rec.Code != http.StatusOK {
		t.Fatalf("ask = %d %s", rec.Code, rec.Body)
	}
	writeFile(t, filepath.Join(source, "local", config.ConfigFileName), shared)
	if rec := postEdit(t, h, settingsWritePath, `{"key":"approvals.mode","value":"ask"}`, nil); rec.Code != http.StatusConflict {
		t.Fatalf("ask into a file with an end marker = %d %s", rec.Code, rec.Body)
	}
	if got := readFile(t, filepath.Join(source, "local", config.ConfigFileName)); got != shared {
		t.Errorf("local config = %q", got)
	}
}

func TestSettings_UndoesTheChangeWhenTheBuiltinCannotBeAdded(t *testing.T) {
	const shared = "version: 1\ntargets: [claude]\nbuiltins: [no-such-builtin]\n"
	h, source := settingsFixture(t, shared)
	rec, _ := postSetting(t, h, `{"key":"approvals.companion","value":true}`, withSecret(settingsTestSecret))
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "was not changed") {
		t.Fatalf("POST = %d %s", rec.Code, rec.Body)
	}
	if _, err := os.Stat(filepath.Join(source, "local", config.ConfigFileName)); !os.IsNotExist(err) {
		t.Errorf("the local config was kept: %v", err)
	}
	if got := readFile(t, filepath.Join(source, config.ConfigFileName)); got != shared {
		t.Errorf("shared config = %q", got)
	}
	if entries, _ := approvals.ReadLog(approvals.LogPath(source)); len(entries) != 0 {
		t.Errorf("an undone change was logged: %+v", entries)
	}
}

func TestSettings_RepairingABrokenYoloBlockNeedsProof(t *testing.T) {
	const shared = "version: 1\napprovals:\n  mode: yolo\n  companion: maybe\n"
	h, source := settingsFixture(t, shared)
	path := filepath.Join(source, config.ConfigFileName)
	rec, _ := postSetting(t, h, `{"key":"approvals.companion","value":false}`, nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("POST without proof = %d %s", rec.Code, rec.Body)
	}
	if got := readFile(t, path); got != shared {
		t.Errorf("a refused repair changed the file:\n%s", got)
	}
	if !strings.Contains(rec.Body.String(), "Choose Ask me") {
		t.Errorf("refusal = %s", rec.Body)
	}
	if rec, _ := postSetting(t, h, `{"key":"approvals.mode","value":"ask"}`, nil); rec.Code != http.StatusOK {
		t.Errorf("Ask me without proof = %d %s", rec.Code, rec.Body)
	}
}

func TestSettings_RepairingABrokenYoloBlockWithProof(t *testing.T) {
	h, _ := settingsFixture(t, "version: 1\napprovals:\n  mode: yolo\n  companion: maybe\n")
	rec, reply := postSetting(t, h, `{"key":"approvals.companion","value":false}`, withSecret(settingsTestSecret))
	if rec.Code != http.StatusOK || reply.Settings.Approvals.Mode != config.ApprovalsYolo {
		t.Fatalf("POST with proof = %d %s", rec.Code, rec.Body)
	}
}

func TestSettings_WaitsForTheApprovalsLock(t *testing.T) {
	h, source := settingsFixture(t, "version: 1\n")
	approvalsLockTries = 2
	t.Cleanup(func() { approvalsLockTries = 50 })
	release, err := lockApprovalsWrites(source)
	if err != nil {
		t.Fatal(err)
	}
	held, err := os.OpenFile(filepath.Join(filepath.Dir(approvals.LogPath(source)), "write.lock"), os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = held.Close() })
	release()
	if err := tryLockProjectFile(held); err != nil {
		t.Skipf("no file locks here: %v", err)
	}
	rec, _ := postSetting(t, h, `{"key":"approvals.companion","value":false}`, nil)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "another agnostic-ai process") {
		t.Fatalf("POST while another process holds the lock = %d %s", rec.Code, rec.Body)
	}
	if _, err := os.Stat(filepath.Join(source, "local", config.ConfigFileName)); !os.IsNotExist(err) {
		t.Errorf("the write went ahead without the lock: %v", err)
	}
	if _, err := writeApprovalsAskLocked(source, true); err == nil {
		t.Error("Ask me went ahead without the lock")
	}
}

func TestRestoreTo_LeavesAnOutsideEditAlone(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agnostic-ai.yaml")
	writeFile(t, path, "before\n")
	undo := restoreTo(path, []byte("before\n"), false)
	writeFile(t, path, "someone else\n")
	if err := undo([]byte("ours\n")); err == nil {
		t.Error("undo replaced a file another program changed")
	}
	if got := readFile(t, path); got != "someone else\n" {
		t.Errorf("file = %q", got)
	}
	writeFile(t, path, "ours\n")
	if err := undo([]byte("ours\n")); err != nil || readFile(t, path) != "before\n" {
		t.Errorf("undo of our own write = %v, file %q", err, readFile(t, path))
	}
}

// TestSettings_AskKeepsALocalCompanionOff: Ask me without proof keeps a
// local companion: false that hides a shared true.
func TestSettings_AskKeepsALocalCompanionOff(t *testing.T) {
	h, source := settingsFixture(t, "version: 1\ntargets: [claude]\napprovals:\n  mode: yolo\n  companion: true\n")
	writeFile(t, filepath.Join(source, "local", config.ConfigFileName), "approvals:\n  mode: yolo\n  companion: no\n")
	if rec, _ := postSetting(t, h, `{"key":"approvals.mode","value":"ask"}`, nil); rec.Code != http.StatusOK {
		t.Fatalf("Ask me = %d %s", rec.Code, rec.Body)
	}
	home, err := config.LoadHomeApprovals(source)
	if err != nil || home.Mode != config.ApprovalsAsk || home.Companion {
		t.Errorf("after Ask me = %+v, %v; want ask with the buttons off", home, err)
	}
	approvalsHandlerAsk := approvalsHandler{reader: h.reader}
	if rec := postEdit(t, approvalsHandlerAsk, approvalsAskPath, `{"mode":"ask"}`, nil); rec.Code != http.StatusOK {
		t.Fatalf("legacy Ask me = %d %s", rec.Code, rec.Body)
	}
	if home, err := config.LoadHomeApprovals(source); err != nil || home.Companion {
		t.Errorf("after the legacy Ask me = %+v, %v", home, err)
	}
}

// TestSettings_AskRepairsABrokenLocalBlock: without proof, Ask me still
// repairs a local block that does not load, with the buttons off.
func TestSettings_AskRepairsABrokenLocalBlock(t *testing.T) {
	for _, local := range []string{"approvals:\n  mode: bogus\n", "approvals:\n  mode: yolo\n  companion: maybe\n"} {
		h, source := settingsFixture(t, "version: 1\ntargets: [claude]\napprovals:\n  mode: ask\n  companion: true\n")
		writeFile(t, filepath.Join(source, "local", config.ConfigFileName), local)
		if rec, _ := postSetting(t, h, `{"key":"approvals.mode","value":"ask"}`, nil); rec.Code != http.StatusOK {
			t.Fatalf("Ask me on %q = %d %s", local, rec.Code, rec.Body)
		}
		home, err := config.LoadHomeApprovals(source)
		if err != nil || home.Mode != config.ApprovalsAsk || home.Companion {
			t.Errorf("after Ask me on %q = %+v, %v", local, home, err)
		}
	}
}

func TestSettings_KeepsAnEditMadeWhileTheChangeWasChecked(t *testing.T) {
	cases := []struct {
		name, body, file string
	}{
		{"companion", `{"key":"approvals.companion","value":false}`, config.ConfigFileName},
		{"ask me", `{"key":"approvals.mode","value":"ask"}`, filepath.Join("local", config.ConfigFileName)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, source := settingsFixture(t, "version: 1\napprovals:\n  mode: review\n")
			path := filepath.Join(source, tc.file)
			const other = "version: 1\ntargets: [claude]\napprovals:\n  mode: review\n"
			approvalsWritePause = func() { writeFile(t, path, other) }
			t.Cleanup(func() { approvalsWritePause = func() {} })
			rec, _ := postSetting(t, h, tc.body, withSecret(settingsTestSecret))
			if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "changed while this change was checked") {
				t.Fatalf("POST = %d %s", rec.Code, rec.Body)
			}
			if got := readFile(t, path); got != other {
				t.Errorf("the other edit was lost:\n%s", got)
			}
		})
	}
}

// TestSettings_ATargetsEditWaitsForAnApprovalsChange: a targets edit that
// starts while an approvals change holds the lock waits for it, so both
// edits stay in the home config.
func TestSettings_ATargetsEditWaitsForAnApprovalsChange(t *testing.T) {
	silence(t)
	h, source := settingsFixture(t, "version: 1\ntargets: [claude]\napprovals:\n  mode: review\n")
	edits := newEditHandler(source, true)
	release, err := lockApprovalsWrites(source)
	if err != nil {
		t.Fatal(err)
	}
	held, err := os.OpenFile(filepath.Join(filepath.Dir(approvals.LogPath(source)), "write.lock"), os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	release()
	if err := tryLockProjectFile(held); err != nil {
		_ = held.Close()
		t.Skipf("no file locks here: %v", err)
	}
	_ = held.Close()
	done := make(chan *httptest.ResponseRecorder, 1)
	approvalsWritePause = func() {
		go func() {
			done <- postEdit(t, edits, editTargetsPath, `{"audience":"shared","name":"codex","enabled":true,"expected":false}`, nil)
		}()
		time.Sleep(200 * time.Millisecond)
	}
	t.Cleanup(func() { approvalsWritePause = func() {} })
	if rec, _ := postSetting(t, h, `{"key":"approvals.companion","value":false}`, withSecret(settingsTestSecret)); rec.Code != http.StatusOK {
		t.Fatalf("approvals change = %d %s", rec.Code, rec.Body)
	}
	if rec := <-done; rec.Code != http.StatusOK {
		t.Fatalf("targets edit = %d %s", rec.Code, rec.Body)
	}
	got := readFile(t, filepath.Join(source, config.ConfigFileName))
	if !strings.Contains(got, "companion: false") || !strings.Contains(got, "codex") {
		t.Errorf("an edit was lost:\n%s", got)
	}
}

// TestSettings_ChecksExpectedAgainstTheReadItWrites: the value the page
// expected is checked against the same read the change is built from, so
// a hand edit landing after that read stops the change.
func TestSettings_ChecksExpectedAgainstTheReadItWrites(t *testing.T) {
	h, source := settingsFixture(t, "version: 1\napprovals:\n  mode: review\n")
	path := filepath.Join(source, config.ConfigFileName)
	const hand = "version: 1\napprovals:\n  mode: ask\n"
	approvalsWritePause = func() { writeFile(t, path, hand) }
	t.Cleanup(func() { approvalsWritePause = func() {} })
	rec, _ := postSetting(t, h, `{"key":"approvals.mode","value":"yolo","expected":"review"}`, withSecret(settingsTestSecret))
	if rec.Code != http.StatusConflict {
		t.Fatalf("POST = %d %s", rec.Code, rec.Body)
	}
	if got := readFile(t, path); got != hand {
		t.Errorf("the hand edit was lost:\n%s", got)
	}
}
