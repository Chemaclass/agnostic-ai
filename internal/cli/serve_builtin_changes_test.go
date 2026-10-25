package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/builtins"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/gitquery"
)

func builtinChangeProject(t *testing.T) string {
	t.Helper()
	silence(t)
	root := t.TempDir()
	chdir(t, root)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "# Keep me.\nversion: 1\ntargets: [claude, codex]\nbuiltins: []\n")
	mustWriteFile(t, ".agnostic-ai/AGNOSTIC_AI.md", "Project instructions.\n")
	if _, err := runCLI(t, "sync"); err != nil {
		t.Fatal(err)
	}
	return root
}

func builtinProposal(t *testing.T, h http.Handler, enabled bool) builtinChangeView {
	t.Helper()
	return builtinNamedProposal(t, h, "caveman", enabled)
}

func builtinNamedProposal(t *testing.T, h http.Handler, name string, enabled bool) builtinChangeView {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"name": name, "enabled": enabled})
	rec := postEdit(t, h, builtinPreviewPath, string(body), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("preview: %d %s", rec.Code, rec.Body)
	}
	var result builtinChangeView
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func builtinAction(t *testing.T, h http.Handler, path, id string) builtinChangeView {
	t.Helper()
	rec := postEdit(t, h, path, `{"id":"`+id+`"}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("action %s: %d %s", path, rec.Code, rec.Body)
	}
	var result builtinChangeView
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestBuiltinChanges_PreviewApplyUndoRestoresProject(t *testing.T) {
	root := builtinChangeProject(t)
	before, err := snapshotBuiltinProject(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	h := newBuiltinChangeHandler(root, false)
	proposal := builtinProposal(t, h, true)
	if proposal.Status != "proposed" || len(proposal.Changes) < 3 {
		t.Fatalf("preview = %+v", proposal)
	}
	if err := before.check(root); err != nil {
		t.Fatalf("preview wrote project: %v", err)
	}
	for _, change := range proposal.Changes {
		if strings.Contains(change.Path, "skills/caveman/") && !strings.HasSuffix(change.Path, "/SKILL.md") {
			t.Errorf("preview lists %s, want only SKILL.md in the caveman skill", change.Path)
		}
	}
	applied := builtinAction(t, h, builtinApplyPath, proposal.ID)
	if applied.Status != "applied" || !applied.UndoAvailable {
		t.Fatalf("apply = %+v", applied)
	}
	for _, path := range []string{".claude/skills/caveman/SKILL.md", ".agents/skills/caveman/SKILL.md"} {
		if _, err := os.Stat(path); err != nil {
			t.Error(err)
		}
	}
	if output, err := runCLI(t, "sync", "--check"); err != nil {
		t.Fatalf("check: %v\n%s", err, output)
	}
	undone := builtinAction(t, h, builtinUndoPath, applied.ID)
	if undone.Status != "undone" || undone.UndoAvailable {
		t.Fatalf("undo = %+v", undone)
	}
	after, err := snapshotBuiltinProject(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !before.sameContent(after) {
		t.Error("undo did not restore original project bytes and modes")
	}
}

func TestBuiltinChanges_StaleSourceRefusesApply(t *testing.T) {
	root := builtinChangeProject(t)
	h := newBuiltinChangeHandler(root, false)
	proposal := builtinProposal(t, h, true)
	mustWriteFile(t, ".agnostic-ai/AGNOSTIC_AI.md", "An intervening edit.\n")
	rec := postEdit(t, h, builtinApplyPath, `{"id":"`+proposal.ID+`"}`, nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("apply: %d %s", rec.Code, rec.Body)
	}
	config, _ := os.ReadFile("agnostic-ai.yaml")
	if strings.Contains(string(config), "caveman") {
		t.Error("stale apply changed config")
	}
}

func TestBuiltinChanges_UndoKeepsInterveningOutputEdit(t *testing.T) {
	root := builtinChangeProject(t)
	h := newBuiltinChangeHandler(root, false)
	proposal := builtinProposal(t, h, true)
	applied := builtinAction(t, h, builtinApplyPath, proposal.ID)
	file := filepath.Join(root, ".claude/skills/caveman/SKILL.md")
	edit := []byte("A user's later edit.\n")
	if err := os.WriteFile(file, edit, 0o600); err != nil {
		t.Fatal(err)
	}
	rec := postEdit(t, h, builtinUndoPath, `{"id":"`+applied.ID+`"}`, nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("undo: %d %s", rec.Code, rec.Body)
	}
	now, _ := os.ReadFile(file)
	if !bytes.Equal(now, edit) {
		t.Error("undo overwrote user edit")
	}
}

func TestBuiltinChanges_EnableDisableMatchesOrdinarySync(t *testing.T) {
	for _, name := range builtins.Names() {
		t.Run(name, func(t *testing.T) {
			root := builtinChangeProject(t)
			h := newBuiltinChangeHandler(root, false)
			for _, enabled := range []bool{true, false} {
				proposal := builtinNamedProposal(t, h, name, enabled)
				builtinAction(t, h, builtinApplyPath, proposal.ID)
				before, err := snapshotBuiltinProject(root, nil)
				if err != nil {
					t.Fatal(err)
				}
				state := readStateFile(".")
				if out, err := runCLI(t, "sync"); err != nil {
					t.Fatalf("sync: %v\n%s", err, out)
				}
				after, err := snapshotBuiltinProject(root, nil)
				if err != nil {
					t.Fatal(err)
				}
				delete(before.files, filepath.ToSlash(stateFilePath(".")))
				delete(after.files, filepath.ToSlash(stateFilePath(".")))
				if !before.sameContent(after) {
					t.Errorf("enabled %v: ordinary sync changed planned native files", enabled)
					for path, a := range before.files {
						b := after.files[path]
						if !bytes.Equal(a.data, b.data) {
							t.Logf("content changed %s: before len %d after len %d", path, len(a.data), len(b.data))
						}
						if a.info != nil && b.info != nil && a.info.Mode() != b.info.Mode() {
							t.Logf("mode changed %s: %v -> %v", path, a.info.Mode(), b.info.Mode())
						}
					}
				}
				stateAfter := readStateFile(".")
				state.SyncedAt = stateAfter.SyncedAt
				state.FilesChanged = stateAfter.FilesChanged
				a, _ := json.Marshal(state)
				b, _ := json.Marshal(stateAfter)
				if !bytes.Equal(a, b) {
					t.Errorf("enabled %v: ledger differs from sync\nbefore: %s\nafter: %s", enabled, a, b)
				}
			}
		})
	}
}

func TestBuiltinChanges_UsesLocalConfigAndKeepsPrivateModes(t *testing.T) {
	root := builtinChangeProject(t)
	mustWriteFile(t, "agnostic-ai.local.yaml", "# Personal choice.\nbuiltins: []\n")
	if err := os.Chmod("agnostic-ai.local.yaml", 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := runCLI(t, "sync"); err != nil {
		t.Fatal(err)
	}
	originalInfo, err := os.Stat("agnostic-ai.local.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && originalInfo.Mode().Perm() != 0o600 {
		t.Fatalf("setup mode = %v", originalInfo.Mode())
	}
	original, _ := os.ReadFile("agnostic-ai.local.yaml")
	shared, _ := os.ReadFile("agnostic-ai.yaml")
	h := newBuiltinChangeHandler(root, false)
	proposal := builtinProposal(t, h, true)
	if proposal.Config != "agnostic-ai.local.yaml" {
		t.Fatalf("config = %s", proposal.Config)
	}
	applied := builtinAction(t, h, builtinApplyPath, proposal.ID)
	now, _ := os.ReadFile("agnostic-ai.yaml")
	if !bytes.Equal(now, shared) {
		t.Error("shared config changed")
	}
	info, _ := os.Stat("agnostic-ai.local.yaml")
	if info.Mode() != originalInfo.Mode() {
		t.Errorf("mode = %v, want original %v", info.Mode(), originalInfo.Mode())
	}
	builtinAction(t, h, builtinUndoPath, applied.ID)
	now, _ = os.ReadFile("agnostic-ai.local.yaml")
	if !bytes.Equal(now, original) {
		t.Error("undo changed local formatting")
	}
}

func TestBuiltinChanges_PartialWriteFailureRestoresFiles(t *testing.T) {
	root := builtinChangeProject(t)
	before, err := snapshotBuiltinProject(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	h := newBuiltinChangeHandler(root, false)
	proposal := builtinProposal(t, h, true)
	count := 0
	h.beforeWrite = func(string) error {
		count++
		if count == 3 {
			return fmt.Errorf("injected write failure")
		}
		return nil
	}
	rec := postEdit(t, h, builtinApplyPath, `{"id":"`+proposal.ID+`"}`, nil)
	if rec.Code == http.StatusOK || !strings.Contains(rec.Body.String(), "restored") {
		t.Fatalf("apply: %d %s", rec.Code, rec.Body)
	}
	after, err := snapshotBuiltinProject(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !before.sameContent(after) {
		t.Error("partial apply was not restored")
	}
}

func TestBuiltinChanges_RollbackKeepsAnInterveningEdit(t *testing.T) {
	root := builtinChangeProject(t)
	h := newBuiltinChangeHandler(root, false)
	proposal := builtinProposal(t, h, true)
	first := ""
	h.beforeWrite = func(path string) error {
		if first == "" {
			first = path
			return nil
		}
		if err := os.Chmod(filepath.Join(root, first), 0o600); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(root, first), []byte("Later edit.\n"), 0o600); err != nil {
			return err
		}
		return fmt.Errorf("injected write failure")
	}
	rec := postEdit(t, h, builtinApplyPath, `{"id":"`+proposal.ID+`"}`, nil)
	if rec.Code == http.StatusOK || !strings.Contains(rec.Body.String(), "could not restore") || !strings.Contains(rec.Body.String(), first) {
		t.Fatalf("apply: %d %s", rec.Code, rec.Body)
	}
	now, _ := os.ReadFile(filepath.Join(root, first))
	if string(now) != "Later edit.\n" {
		t.Error("rollback overwrote an intervening edit")
	}
}

func TestBuiltinChanges_RespectsCLIWriterLock(t *testing.T) {
	root := builtinChangeProject(t)
	h := newBuiltinChangeHandler(root, false)
	proposal := builtinProposal(t, h, true)
	lock, err := acquireProjectLock(root, "test writer")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.Close() }()
	rec := postEdit(t, h, builtinApplyPath, `{"id":"`+proposal.ID+`"}`, nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("apply: %d %s", rec.Code, rec.Body)
	}
}

func TestBuiltinChanges_NewSourceInvalidatesPreview(t *testing.T) {
	root := builtinChangeProject(t)
	h := newBuiltinChangeHandler(root, false)
	proposal := builtinProposal(t, h, true)
	mustWriteFile(t, ".agnostic-ai/rules/later.md", "---\nname: later\ndescription: New source.\n---\nNew rule.\n")
	rec := postEdit(t, h, builtinApplyPath, `{"id":"`+proposal.ID+`"}`, nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("apply: %d %s", rec.Code, rec.Body)
	}
}

func TestBuiltinChanges_InputChangedDuringApplyIsKept(t *testing.T) {
	root := builtinChangeProject(t)
	h := newBuiltinChangeHandler(root, false)
	proposal := builtinProposal(t, h, true)
	h.beforeWrite = func(string) error {
		mustWriteFile(t, ".agnostic-ai/rules/concurrent.md", "---\nname: concurrent\ndescription: Concurrent rule.\n---\nKeep this rule.\n")
		return nil
	}
	rec := postEdit(t, h, builtinApplyPath, `{"id":"`+proposal.ID+`"}`, nil)
	if rec.Code == http.StatusOK || !strings.Contains(rec.Body.String(), "restored") {
		t.Fatalf("apply: %d %s", rec.Code, rec.Body)
	}
	config, _ := os.ReadFile("agnostic-ai.yaml")
	if strings.Contains(string(config), "caveman") {
		t.Error("failed apply left config changed")
	}
	if _, err := os.Stat(".agnostic-ai/rules/concurrent.md"); err != nil {
		t.Error(err)
	}
}

func TestBuiltinChanges_PreviewDoesNotReplaceSuccessfulUndo(t *testing.T) {
	root := builtinChangeProject(t)
	h := newBuiltinChangeHandler(root, false)
	proposal := builtinProposal(t, h, true)
	applied := builtinAction(t, h, builtinApplyPath, proposal.ID)
	builtinProposal(t, h, false)
	builtinAction(t, h, builtinUndoPath, applied.ID)
	config, _ := os.ReadFile("agnostic-ai.yaml")
	if strings.Contains(string(config), "caveman") {
		t.Error("undo did not restore original config")
	}
}

func TestBuiltinChanges_ExpiredAndReplayedRequestsDoNotWrite(t *testing.T) {
	root := builtinChangeProject(t)
	h := newBuiltinChangeHandler(root, false)
	proposal := builtinProposal(t, h, true)
	applied := builtinAction(t, h, builtinApplyPath, proposal.ID)
	before, err := snapshotBuiltinProject(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, request := range []struct {
		handler http.Handler
		path    string
	}{
		{h, builtinApplyPath},
		{newBuiltinChangeHandler(root, false), builtinUndoPath},
	} {
		rec := postEdit(t, request.handler, request.path, `{"id":"`+applied.ID+`"}`, nil)
		if rec.Code != http.StatusConflict {
			t.Errorf("request: %d %s", rec.Code, rec.Body)
		}
	}
	if err := before.check(root); err != nil {
		t.Error(err)
	}
}

func TestBuiltinChanges_RejectsUnsafeProjectsBeforeWriting(t *testing.T) {
	for _, tc := range []struct{ name, config string }{
		{"external adapter", "version: 1\ntargets: [external-tool]\nbuiltins: []\n"},
		{"external output", "version: 1\ntargets: [claude]\nbuiltins: []\noutputs:\n  claude:\n    rules-file: ../outside.md\n"},
		{"linked skills", "version: 1\ntargets: [claude]\nbuiltins: []\nsync:\n  shared-skills: true\n"},
		{"unsupported skill", "version: 1\ntargets: [jules]\nbuiltins: []\non-unsupported: error\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := builtinChangeProject(t)
			mustWriteFile(t, "agnostic-ai.yaml", tc.config)
			before, err := snapshotBuiltinProject(root, nil)
			if err != nil {
				t.Fatal(err)
			}
			h := newBuiltinChangeHandler(root, false)
			rec := postEdit(t, h, builtinPreviewPath, `{"name":"caveman","enabled":true}`, nil)
			if rec.Code == http.StatusOK {
				t.Fatalf("unsafe preview accepted: %s", rec.Body)
			}
			if err := before.check(root); err != nil {
				t.Error(err)
			}
		})
	}
}

func TestBuiltinChanges_LocalScratchSymlinkDoesNotBlockApplyOrUndo(t *testing.T) {
	root := builtinChangeProject(t)
	outside := filepath.Join(t.TempDir(), "bashunit")
	mustWriteFile(t, outside, "Keep the tool.\n")
	link := filepath.Join(defaultProjectUser, "refactor-sync-guards", "tools", "bashunit")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	report := filepath.Join(defaultProjectUser, "refactor-sync-guards", "report.md")
	mustWriteFile(t, report, "Original notes.\n")
	h := newBuiltinChangeHandler(root, false)
	proposal := builtinProposal(t, h, true)
	mustWriteFile(t, report, "Later notes.\n")
	applied := builtinAction(t, h, builtinApplyPath, proposal.ID)
	builtinAction(t, h, builtinUndoPath, applied.ID)
	if target, err := os.Readlink(link); err != nil || target != outside {
		t.Errorf("scratch link changed: %q, %v", target, err)
	}
	for path, want := range map[string]string{outside: "Keep the tool.\n", report: "Later notes.\n"} {
		data, err := os.ReadFile(path)
		if err != nil || string(data) != want {
			t.Errorf("%s: got %q, %v", path, data, err)
		}
	}
	if out, err := runCLI(t, "sync", "--check"); err != nil {
		t.Fatalf("undo left drift: %v\n%s", err, out)
	}
}

func TestBuiltinChanges_NewLocalSourcesAndHookScriptsRefuseApply(t *testing.T) {
	for _, path := range []string{
		".agnostic-ai/local/rules/later.md",
		".agnostic-ai/local/AGNOSTIC_AI.md",
		".agnostic-ai/scripts/later.sh",
	} {
		t.Run(path, func(t *testing.T) {
			root := builtinChangeProject(t)
			h := newBuiltinChangeHandler(root, false)
			proposal := builtinProposal(t, h, true)
			mustWriteFile(t, path, "A later source.\n")
			rec := postEdit(t, h, builtinApplyPath, `{"id":"`+proposal.ID+`"}`, nil)
			if rec.Code != http.StatusConflict {
				t.Fatalf("apply: %d %s", rec.Code, rec.Body)
			}
			data, err := os.ReadFile("agnostic-ai.yaml")
			if err != nil || strings.Contains(string(data), "caveman") {
				t.Errorf("stale apply changed config: %q, %v", data, err)
			}
		})
	}
}

func TestBuiltinChanges_SymlinkInputRefused(t *testing.T) {
	for _, tc := range []struct {
		name, path, config string
	}{
		{"shared rule", ".agnostic-ai/rules/linked.md", ""},
		{"local rule", ".agnostic-ai/local/rules/linked.md", ""},
		{"custom source", ".agnostic-ai/local/custom-specs/linked.md", "version: 1\ntargets: [claude, codex]\nbuiltins: []\nsources:\n  rules: .agnostic-ai/local/custom-specs\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := builtinChangeProject(t)
			if tc.config != "" {
				mustWriteFile(t, "agnostic-ai.yaml", tc.config)
			}
			outside := filepath.Join(t.TempDir(), "rule.md")
			mustWriteFile(t, outside, "Keep me.\n")
			if err := os.MkdirAll(filepath.Dir(tc.path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, tc.path); err != nil {
				t.Fatal(err)
			}
			h := newBuiltinChangeHandler(root, false)
			rec := postEdit(t, h, builtinPreviewPath, `{"name":"caveman","enabled":true}`, nil)
			if rec.Code == http.StatusOK || !strings.Contains(rec.Body.String(), "not a regular file") {
				t.Fatalf("unsafe input not rejected: %d %s", rec.Code, rec.Body)
			}
			data, _ := os.ReadFile(outside)
			if string(data) != "Keep me.\n" {
				t.Error("external source changed")
			}
		})
	}
}

func TestBuiltinChanges_RequestBoundaries(t *testing.T) {
	h := newBuiltinChangeHandler(t.TempDir(), false)
	for _, tc := range []struct {
		name   string
		mutate func(*http.Request)
		status int
	}{
		{"foreign host", func(r *http.Request) { r.Host = "example.com" }, http.StatusMisdirectedRequest},
		{"foreign origin", func(r *http.Request) { r.Header.Set("Origin", "https://example.com") }, http.StatusForbidden},
		{"cross site", func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "cross-site") }, http.StatusForbidden},
		{"content type", func(r *http.Request) { r.Header.Set("Content-Type", "text/plain") }, http.StatusUnsupportedMediaType},
		{"method", func(r *http.Request) { r.Method = http.MethodGet }, http.StatusMethodNotAllowed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := postEdit(t, h, builtinPreviewPath, `{"name":"caveman","enabled":true}`, tc.mutate)
			if rec.Code != tc.status {
				t.Errorf("status = %d, want %d: %s", rec.Code, tc.status, rec.Body)
			}
		})
	}
	for _, body := range []string{`{"name":"caveman"}`, `{"name":"unknown","enabled":true}`, `{"name":"caveman","enabled":true,"unknown":1}`, `{"name":"caveman","enabled":true} {}`} {
		rec := postEdit(t, h, builtinPreviewPath, body, nil)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("invalid body = %d: %s", rec.Code, rec.Body)
		}
	}
	req := httptest.NewRequest(http.MethodGet, "http://example.com"+builtinStatusPath, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("foreign status = %d", rec.Code)
	}
	global := newBuiltinChangeHandler(t.TempDir(), true)
	rec = postEdit(t, global, builtinPreviewPath, `{"name":"caveman","enabled":true}`, nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("global mutation = %d", rec.Code)
	}
}

func TestBuiltinChanges_RedactsPreviewAndLeavesEnvironmentUntouched(t *testing.T) {
	root := builtinChangeProject(t)
	secret := "sk-" + strings.Repeat("a1", 24)
	config, _ := os.ReadFile("agnostic-ai.yaml")
	mustWriteFile(t, "agnostic-ai.yaml", strings.Replace(string(config), "builtins: []", "builtins: [] # https://example.test/?api_key="+secret, 1))
	home, err := snapshotBuiltinProject(os.Getenv("HOME"), nil)
	if err != nil {
		t.Fatal(err)
	}
	h := newBuiltinChangeHandler(root, false)
	rec := postEdit(t, h, builtinPreviewPath, `{"name":"caveman","enabled":true}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("preview: %d %s", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), secret) {
		t.Error("preview disclosed credential")
	}
	if err := home.check(os.Getenv("HOME")); err != nil {
		t.Error(err)
	}
}

func TestBuiltinChanges_DisableUndoRestoresDeletedFileMode(t *testing.T) {
	root := builtinChangeProject(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, codex]\nbuiltins: [caveman]\n")
	if _, err := runCLI(t, "sync"); err != nil {
		t.Fatal(err)
	}
	path := ".claude/skills/caveman/SKILL.md"
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := snapshotBuiltinProject(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	h := newBuiltinChangeHandler(root, false)
	proposal := builtinProposal(t, h, false)
	applied := builtinAction(t, h, builtinApplyPath, proposal.ID)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("disabled skill exists: %v", err)
	}
	builtinAction(t, h, builtinUndoPath, applied.ID)
	after, err := snapshotBuiltinProject(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !before.sameContent(after) {
		t.Error("undo did not restore deleted bytes and modes")
	}
}

func TestBuiltinChanges_ParentMovedOutsideIsNeverMutated(t *testing.T) {
	root := builtinChangeProject(t)
	h := newBuiltinChangeHandler(root, false)
	proposal := builtinProposal(t, h, true)
	builtinAction(t, h, builtinApplyPath, proposal.ID)
	proposal = builtinProposal(t, h, false)
	outside := filepath.Join(t.TempDir(), "moved")
	moved := false
	h.beforeWrite = func(path string) error {
		if moved || !strings.HasPrefix(path, ".agents/skills/caveman/") {
			return nil
		}
		moved = true
		parent := filepath.Join(root, ".agents/skills/caveman")
		if err := os.Rename(parent, outside); err != nil {
			return err
		}
		return os.Symlink(outside, parent)
	}
	rec := postEdit(t, h, builtinApplyPath, `{"id":"`+proposal.ID+`"}`, nil)
	if rec.Code == http.StatusOK {
		t.Fatalf("unsafe apply accepted: %s", rec.Body)
	}
	if !moved {
		t.Fatal("test did not move output parent")
	}
	if _, err := os.Stat(filepath.Join(outside, "SKILL.md")); err != nil {
		t.Errorf("apply mutated moved external output: %v", err)
	}
}

func TestBuiltinChanges_FailedLaterApplyKeepsSuccessfulUndo(t *testing.T) {
	root := builtinChangeProject(t)
	h := newBuiltinChangeHandler(root, false)
	first := builtinProposal(t, h, true)
	applied := builtinAction(t, h, builtinApplyPath, first.ID)
	second := builtinProposal(t, h, false)
	count := 0
	h.beforeWrite = func(string) error {
		count++
		if count == 2 {
			return fmt.Errorf("injected failure")
		}
		return nil
	}
	rec := postEdit(t, h, builtinApplyPath, `{"id":"`+second.ID+`"}`, nil)
	if rec.Code == http.StatusOK || !strings.Contains(rec.Body.String(), "restored") {
		t.Fatalf("apply: %d %s", rec.Code, rec.Body)
	}
	h.beforeWrite = nil
	builtinAction(t, h, builtinUndoPath, applied.ID)
}

func TestBuiltinChanges_FailedUndoCanRetryAfterCompleteRollback(t *testing.T) {
	root := builtinChangeProject(t)
	h := newBuiltinChangeHandler(root, false)
	first := builtinProposal(t, h, true)
	applied := builtinAction(t, h, builtinApplyPath, first.ID)
	count := 0
	h.beforeWrite = func(string) error {
		count++
		if count == 2 {
			return fmt.Errorf("injected failure")
		}
		return nil
	}
	rec := postEdit(t, h, builtinUndoPath, `{"id":"`+applied.ID+`"}`, nil)
	if rec.Code == http.StatusOK || !strings.Contains(rec.Body.String(), "restored") {
		t.Fatalf("undo: %d %s", rec.Code, rec.Body)
	}
	h.beforeWrite = nil
	builtinAction(t, h, builtinUndoPath, applied.ID)
}

func TestBuiltinChanges_DisablingUnsupportedSkillClearsWarnings(t *testing.T) {
	root := builtinChangeProject(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [aider]\nbuiltins: [caveman]\non-unsupported: warn\n")
	if _, err := runCLI(t, "sync"); err != nil {
		t.Fatal(err)
	}
	h := newBuiltinChangeHandler(root, false)
	proposal := builtinProposal(t, h, false)
	if len(proposal.Findings) != 0 {
		t.Errorf("disable findings = %v", proposal.Findings)
	}
	builtinAction(t, h, builtinApplyPath, proposal.ID)
	before := readStateFile(".")
	if _, err := runCLI(t, "sync"); err != nil {
		t.Fatal(err)
	}
	after := readStateFile(".")
	before.SyncedAt, before.FilesChanged = after.SyncedAt, after.FilesChanged
	a, _ := json.Marshal(before)
	b, _ := json.Marshal(after)
	if !bytes.Equal(a, b) {
		t.Error("disable ledger differs from ordinary sync")
	}
}

func TestBuiltinChanges_ParentMovedInsideProjectIsNeverMutated(t *testing.T) {
	for _, action := range []string{builtinApplyPath, builtinUndoPath} {
		t.Run(filepath.Base(action), func(t *testing.T) {
			root := builtinChangeProject(t)
			h := newBuiltinChangeHandler(root, false)
			proposal := builtinProposal(t, h, true)
			applied := builtinAction(t, h, builtinApplyPath, proposal.ID)
			id := applied.ID
			if action == builtinApplyPath {
				id = builtinProposal(t, h, false).ID
			}
			destination := filepath.Join(root, "moved")
			moved := false
			h.beforeWrite = func(path string) error {
				if moved || !strings.HasPrefix(path, ".agents/skills/caveman/") {
					return nil
				}
				parent := filepath.Join(root, ".agents/skills/caveman")
				if err := os.Rename(parent, destination); err != nil {
					return err
				}
				target, err := filepath.Rel(filepath.Dir(parent), destination)
				if err != nil {
					return err
				}
				if err := os.Symlink(target, parent); err != nil {
					return err
				}
				moved = true
				return nil
			}
			rec := postEdit(t, h, action, `{"id":"`+id+`"}`, nil)
			if rec.Code == http.StatusOK {
				t.Fatalf("unsafe operation accepted: %s", rec.Body)
			}
			if !moved {
				t.Fatalf("test did not create alias: %s", rec.Body)
			}
			if _, err := os.Stat(filepath.Join(destination, "SKILL.md")); err != nil {
				t.Errorf("operation mutated moved output: %v", err)
			}
		})
	}
}

func TestBuiltinChanges_DisableUndoKeepsReadOnlyAssets(t *testing.T) {
	root := builtinChangeProject(t)
	h := newBuiltinChangeHandler(root, false)
	proposal := builtinProposal(t, h, true)
	builtinAction(t, h, builtinApplyPath, proposal.ID)
	path := ".agents/skills/caveman/SKILL.md"
	if err := os.Chmod(path, 0o444); err != nil {
		t.Fatal(err)
	}
	before, err := snapshotEditFile(path)
	if err != nil {
		t.Fatal(err)
	}
	proposal = builtinProposal(t, h, false)
	applied := builtinAction(t, h, builtinApplyPath, proposal.ID)
	builtinAction(t, h, builtinUndoPath, applied.ID)
	after, err := snapshotEditFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if after.info == nil || after.info.Mode() != before.info.Mode() || !bytes.Equal(after.data, before.data) {
		t.Error("undo did not restore read-only asset bytes and mode")
	}
}

func TestBuiltinChanges_StaleProposalRecoveryRequiresFreshPreview(t *testing.T) {
	root := builtinChangeProject(t)
	h := newBuiltinChangeHandler(root, false)
	proposal := builtinProposal(t, h, true)
	applied := builtinAction(t, h, builtinApplyPath, proposal.ID)
	stale := builtinProposal(t, h, false)
	config, err := os.ReadFile("agnostic-ai.yaml")
	if err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, "agnostic-ai.yaml", string(config)+"# A later edit.\n")
	rec := postEdit(t, h, builtinApplyPath, `{"id":"`+stale.ID+`"}`, nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("apply: %d %s", rec.Code, rec.Body)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4310"+builtinStatusPath, nil))
	var recovered builtinChangeView
	if err := json.Unmarshal(rec.Body.Bytes(), &recovered); err != nil {
		t.Fatal(err)
	}
	if recovered.Status == "proposed" || recovered.ID == stale.ID {
		t.Errorf("recovery offers stale proposal: status %s, id %s", recovered.Status, recovered.ID)
	}
	if !recovered.UndoAvailable || recovered.UndoID != applied.ID {
		t.Error("stale proposal discarded previous successful undo")
	}
	fresh := builtinProposal(t, h, false)
	if fresh.ID == stale.ID {
		t.Error("fresh preview reused stale proposal")
	}
	builtinAction(t, h, builtinApplyPath, fresh.ID)
}

func TestBuiltinChanges_NativePathsUseRecordedSnapshots(t *testing.T) {
	root := builtinChangeProject(t)
	ignore := "nested" + string(filepath.Separator) + "." + string(filepath.Separator) + "generated.ignore"
	if err := os.Mkdir("nested", 0o755); err != nil {
		t.Fatal(err)
	}
	config, err := os.ReadFile("agnostic-ai.yaml")
	if err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, "agnostic-ai.yaml", string(config)+"gitignore:\n  enabled: true\n  path: '"+ignore+"'\n")
	if _, err := runCLI(t, "sync"); err != nil {
		t.Fatal(err)
	}
	before, err := snapshotBuiltinProject(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := planBuiltinChange(root, "caveman", true)
	if err != nil {
		t.Fatal(err)
	}
	ledgerFound := false
	for _, change := range plan.changes {
		canonical := filepath.ToSlash(filepath.Clean(change.Path))
		if change.Path != canonical {
			t.Fatalf("planned path %q does not match snapshot key %q", change.Path, canonical)
		}
		recorded, ok := plan.inputs.files[change.Path]
		if !ok || !sameBuiltinFile(change.before, recorded) {
			t.Fatalf("planned path %q lost its recorded snapshot", change.Path)
		}
		if change.Path == filepath.ToSlash(stateFilePath(".")) {
			ledgerFound = true
			if change.Action != "update" || change.before.info == nil {
				t.Fatal("existing ledger planned as a new file")
			}
		}
	}
	if !ledgerFound {
		t.Fatal("preview omitted ledger update")
	}
	h := newBuiltinChangeHandler(root, false)
	proposal := builtinProposal(t, h, true)
	applied := builtinAction(t, h, builtinApplyPath, proposal.ID)
	builtinAction(t, h, builtinUndoPath, applied.ID)
	after, err := snapshotBuiltinProject(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !before.sameContent(after) {
		t.Error("native-path apply/undo changed original bytes or modes")
	}
}

func TestBuiltinChanges_AnchoredSnapshotRejectsIdenticalReplacement(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join("nested", "file")
	absolute := filepath.Join(root, path)
	mustWriteFile(t, absolute, "Keep this content.\n")
	before, err := snapshotEditFile(absolute)
	if err != nil {
		t.Fatal(err)
	}
	anchored, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = anchored.Close() }()
	if err := checkBuiltinFile(anchored, path, before); err != nil {
		t.Fatalf("unchanged native path: %v", err)
	}
	replacement := filepath.Join(root, "replacement")
	if err := os.WriteFile(replacement, before.data, before.info.Mode().Perm()); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(replacement, before.info.ModTime(), before.info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, absolute); err != nil {
		t.Fatal(err)
	}
	if err := checkBuiltinFile(anchored, path, before); err == nil {
		t.Error("accepted replacement with different identity despite equal bytes, mode, and time")
	}
}

func TestBuiltinChanges_AllBuiltinsEnableDisableUndo(t *testing.T) {
	for _, name := range builtins.Names() {
		t.Run(name, func(t *testing.T) {
			root := builtinChangeProject(t)
			h := newBuiltinChangeHandler(root, false)
			for _, enabled := range []bool{true, false} {
				before, err := snapshotBuiltinProject(root, nil)
				if err != nil {
					t.Fatal(err)
				}
				proposal := builtinNamedProposal(t, h, name, enabled)
				if proposal.Name != name {
					t.Errorf("name = %q", proposal.Name)
				}
				applied := builtinAction(t, h, builtinApplyPath, proposal.ID)
				if applied.UndoName != name {
					t.Errorf("undo name = %q", applied.UndoName)
				}
				if out, err := runCLI(t, "sync", "--check"); err != nil {
					t.Fatalf("check: %v\n%s", err, out)
				}
				builtinAction(t, h, builtinUndoPath, applied.ID)
				after, err := snapshotBuiltinProject(root, nil)
				if err != nil {
					t.Fatal(err)
				}
				if !before.sameContent(after) {
					t.Fatal("undo changed original bytes/modes")
				}
				builtinAction(t, h, builtinApplyPath, builtinNamedProposal(t, h, name, enabled).ID)
			}
		})
	}
}

func TestBuiltinChanges_PendingHandoffIncludedAndUndoRestoresDrift(t *testing.T) {
	root := builtinChangeProject(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, codex]\nbuiltins: [handoff]\n")
	before, err := snapshotBuiltinProject(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	h := newBuiltinChangeHandler(root, false)
	proposal := builtinProposal(t, h, true)
	if !strings.Contains(strings.Join(proposal.Findings, " "), "pending source changes") {
		t.Error("missing pending update finding")
	}
	if err := before.check(root); err != nil {
		t.Fatalf("preview wrote: %v", err)
	}
	applied := builtinAction(t, h, builtinApplyPath, proposal.ID)
	if out, err := runCLI(t, "sync", "--check"); err != nil {
		t.Fatalf("check: %v\n%s", err, out)
	}
	builtinAction(t, h, builtinUndoPath, applied.ID)
	after, err := snapshotBuiltinProject(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !before.sameContent(after) {
		t.Error("undo did not restore pending drift")
	}
}

func TestBuiltinChanges_RefusesUnprovenOutputWrites(t *testing.T) {
	for _, kind := range []string{"edited", "missing checksum", "new collision", "edited manifest"} {
		t.Run(kind, func(t *testing.T) {
			root := builtinChangeProject(t)
			path := "AGENTS.md"
			switch kind {
			case "new collision":
				path = ".claude/skills/caveman/SKILL.md"
				mustWriteFile(t, path, "User file.\n")
			case "edited manifest":
				mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, codex]\nbuiltins: []\nsync:\n  output-manifest: true\n")
				if out, err := runCLI(t, "sync"); err != nil {
					t.Fatalf("sync: %v\n%s", err, out)
				}
				path = outputManifestPath
				mustWriteFile(t, path, "User file.\n")
			case "edited":
				mustWriteFile(t, path, "User file.\n")
			case "missing checksum":
				state := readStateFile(".")
				delete(state.OutputSums, path)
				data, err := json.Marshal(state)
				if err != nil {
					t.Fatal(err)
				}
				mustWriteFile(t, stateFilePath("."), string(data))
			}
			mustWriteFile(t, ".agnostic-ai/AGNOSTIC_AI.md", "Changed source.\n")
			before, err := snapshotBuiltinProject(root, nil)
			if err != nil {
				t.Fatal(err)
			}
			h := newBuiltinChangeHandler(root, false)
			rec := postEdit(t, h, builtinPreviewPath, `{"name":"caveman","enabled":true}`, nil)
			if rec.Code == http.StatusOK || !strings.Contains(rec.Body.String(), path) {
				t.Fatalf("unproven write: %d %s", rec.Code, rec.Body)
			}
			if err := before.check(root); err != nil {
				t.Error(err)
			}
		})
	}
}

func TestBuiltinChanges_HandoffThenCavemanKeepsOneUndoOwner(t *testing.T) {
	root := builtinChangeProject(t)
	h := newBuiltinChangeHandler(root, false)
	first := builtinAction(t, h, builtinApplyPath, builtinNamedProposal(t, h, "handoff", true).ID)
	before, err := snapshotBuiltinProject(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	second := builtinProposal(t, h, true)
	if second.Name != "caveman" || second.UndoName != "handoff" || second.UndoID != first.ID {
		t.Fatalf("proposal owners = %+v", second)
	}
	applied := builtinAction(t, h, builtinApplyPath, second.ID)
	if applied.UndoName != "caveman" || applied.UndoID != second.ID {
		t.Fatalf("applied owners = %+v", applied)
	}
	rec := postEdit(t, h, builtinUndoPath, `{"id":"`+first.ID+`"}`, nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("old undo: %d %s", rec.Code, rec.Body)
	}
	builtinAction(t, h, builtinUndoPath, applied.ID)
	after, err := snapshotBuiltinProject(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !before.sameContent(after) {
		t.Error("undo did not retain first built-in")
	}
}

func TestBuiltinChanges_LastHookReleasePreservesUserSettings(t *testing.T) {
	for _, name := range []string{"rtk", "handoff-hook"} {
		t.Run(name, func(t *testing.T) {
			root := builtinChangeProject(t)
			original := "{\n  \"theme\": \"dark\"\n}\n"
			mustWriteFile(t, ".claude/settings.json", original)
			mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, codex]\nbuiltins: ["+name+"]\n")
			if out, err := runCLI(t, "sync"); err != nil {
				t.Fatalf("sync: %v\n%s", err, out)
			}
			before, err := snapshotBuiltinProject(root, nil)
			if err != nil {
				t.Fatal(err)
			}
			h := newBuiltinChangeHandler(root, false)
			applied := builtinAction(t, h, builtinApplyPath, builtinNamedProposal(t, h, name, false).ID)
			data, err := os.ReadFile(".claude/settings.json")
			if err != nil {
				t.Fatal(err)
			}
			if string(data) != original {
				t.Errorf("user settings changed: %s", data)
			}
			if _, ok := readStateFile(".").Merged[".claude/settings.json"]; ok {
				t.Error("released settings still claimed")
			}
			if out, err := runCLI(t, "sync", "--check"); err != nil {
				t.Fatalf("check: %v\n%s", err, out)
			}
			builtinAction(t, h, builtinUndoPath, applied.ID)
			after, err := snapshotBuiltinProject(root, nil)
			if err != nil {
				t.Fatal(err)
			}
			if !before.sameContent(after) {
				t.Error("undo changed hook settings")
			}
		})
	}
}

func TestBuiltinChanges_MemoryMigrationRequiresCLIWithoutWriting(t *testing.T) {
	for _, kind := range []string{"native fact", "native fact BOM", "native index", "repo fact", "external store", "tracked settings"} {
		t.Run(kind, func(t *testing.T) {
			root := builtinChangeProject(t)
			switch kind {
			case "native fact", "native fact BOM", "native index":
				if kind == "native fact BOM" {
					mustWriteFile(t, ".claude/settings.local.json", "\xef\xbb\xbf{}\n")
					if out, err := runCLI(t, "sync"); err != nil {
						t.Fatalf("sync: %v\n%s", err, out)
					}
				}
				native, _, err := adapters.NativeMemoryDir("claude", root)
				if err != nil {
					t.Fatal(err)
				}
				if kind == "native fact" || kind == "native fact BOM" {
					mustWriteFile(t, filepath.Join(native, "fact.md"), "A saved fact.\n")
				} else {
					mustWriteFile(t, filepath.Join(native, "MEMORY.md"), "A saved note in the index.\n")
				}
			case "repo fact", "external store", "tracked settings":
				if _, err := gitquery.LayoutAt(root); err == nil {
					t.Fatal("fixture already belongs to a Git repository")
				}
				git(t, root, "init", "--quiet")
				gitquery.Reset()
				switch kind {
				case "repo fact":
					store, err := adapters.PersonalMemoryDir(&config.Config{Memory: config.MemoryConfig{Personal: config.PersonalMemoryRepo}}, root)
					if err != nil {
						t.Fatal(err)
					}
					mustWriteFile(t, filepath.Join(store, "fact.md"), "A saved repository fact.\n")
				case "external store":
					mustWriteFile(t, "agnostic-ai.local.yaml", "memory:\n  personal: repo\n")
				case "tracked settings":
					mustWriteFile(t, ".claude/settings.local.json", "{}\n")
					git(t, root, "add", "--force", ".claude/settings.local.json")
				}
			}
			before, err := snapshotBuiltinProject(root, nil)
			if err != nil {
				t.Fatal(err)
			}
			home, err := snapshotBuiltinProject(os.Getenv("HOME"), nil)
			if err != nil {
				t.Fatal(err)
			}
			h := newBuiltinChangeHandler(root, false)
			rec := postEdit(t, h, builtinPreviewPath, `{"name":"memory","enabled":true}`, nil)
			if rec.Code == http.StatusOK || !strings.Contains(rec.Body.String(), "CLI") {
				t.Fatalf("unsafe memory preview: %d %s", rec.Code, rec.Body)
			}
			if kind == "native fact BOM" && !strings.Contains(rec.Body.String(), "existing memory needs migration") {
				t.Fatalf("BOM settings bypassed memory migration check: %d %s", rec.Code, rec.Body)
			}
			if err := before.check(root); err != nil {
				t.Error(err)
			}
			if err := home.check(os.Getenv("HOME")); err != nil {
				t.Error(err)
			}
		})
	}
}

func TestBuiltinChanges_UnchangedUserMemoryDirectoryDoesNotRequireNativeMigration(t *testing.T) {
	root := builtinChangeProject(t)
	mustWriteFile(t, ".claude/settings.local.json", `{"autoMemoryDirectory":"/user/memory"}`+"\n")
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, codex]\nbuiltins: [memory]\n")
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	native, _, err := adapters.NativeMemoryDir("claude", root)
	if err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, filepath.Join(native, "fact.md"), "A saved native fact.\n")
	h := newBuiltinChangeHandler(root, false)
	builtinAction(t, h, builtinApplyPath, builtinProposal(t, h, true).ID)
	data, err := os.ReadFile(".claude/settings.local.json")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"autoMemoryDirectory":"/user/memory"}`+"\n" {
		t.Errorf("user memory setting changed: %s", data)
	}
}

func TestBuiltinChanges_MemoryAddsSyncGitExclusion(t *testing.T) {
	silence(t)
	root := t.TempDir()
	chdir(t, root)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
	git(t, root, "init", "--quiet")
	gitquery.Reset()
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, codex]\nbuiltins: [handoff, rtk]\n")
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	exclude := filepath.Join(root, ".git", "info", "exclude")
	original, err := os.ReadFile(exclude)
	if err != nil {
		t.Fatal(err)
	}
	// serve passes its working directory as the root.
	h := newBuiltinChangeHandler(".", false)
	proposal := builtinNamedProposal(t, h, "memory", true)
	var listed bool
	for _, change := range proposal.Changes {
		if change.Path == ".git/info/exclude" && strings.Contains(change.Diff, "+**/.claude/settings.local.json") {
			listed = true
		}
	}
	if !listed {
		t.Fatalf("preview does not list the Git exclusion: %+v", proposal.Changes)
	}
	applied := builtinAction(t, h, builtinApplyPath, proposal.ID)
	settings, err := os.ReadFile(".claude/settings.local.json")
	if err != nil || !strings.Contains(string(settings), "autoMemoryDirectory") {
		t.Fatalf("memory settings = %q, %v", settings, err)
	}
	if status := git(t, root, "status", "--porcelain", "--untracked-files=all"); strings.Contains(status, "settings.local.json") {
		t.Errorf("git status shows private settings:\n%s", status)
	}
	if out, err := runCLI(t, "sync", "--check"); err != nil {
		t.Fatalf("check: %v\n%s", err, out)
	}
	builtinAction(t, h, builtinUndoPath, applied.ID)
	if restored, err := os.ReadFile(exclude); err != nil || !bytes.Equal(restored, original) {
		t.Errorf("undo left info/exclude = %q, %v; want %q", restored, err, original)
	}
}

func memoryGitProject(t *testing.T, root, config string) {
	t.Helper()
	chdir(t, root)
	gitquery.Reset()
	mustWriteFile(t, "agnostic-ai.yaml", config)
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
}

func TestBuiltinChanges_MemoryExclusionWithoutManagedGitignore(t *testing.T) {
	silence(t)
	root := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
	git(t, root, "init", "--quiet")
	memoryGitProject(t, root, "version: 1\ntargets: [claude, codex]\nbuiltins: [handoff, rtk]\ngitignore:\n  enabled: false\n")
	h := newBuiltinChangeHandler(".", false)
	proposal := builtinNamedProposal(t, h, "memory", true)
	for _, change := range proposal.Changes {
		if change.Path == ".git/info/exclude" {
			if !strings.Contains(change.Diff, "\n+**/.claude/settings.local.json\n") {
				t.Fatalf("planned exclusion:\n%s", change.Diff)
			}
			return
		}
	}
	t.Fatalf("preview does not change .git/info/exclude: %+v", proposal.Changes)
}

func TestBuiltinChanges_MemoryExclusionOutsideLinkedWorktreeRequiresCLI(t *testing.T) {
	silence(t)
	isolateGit(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
	main := t.TempDir()
	git(t, main, "init", "--quiet")
	git(t, main, "commit", "--quiet", "--allow-empty", "-m", "start")
	linked := filepath.Join(t.TempDir(), "linked")
	git(t, main, "worktree", "add", "--quiet", "-b", "linked", linked)
	memoryGitProject(t, linked, "version: 1\ntargets: [claude, codex]\nbuiltins: [handoff, rtk]\n")
	exclude := filepath.Join(main, ".git", "info", "exclude")
	before, err := os.ReadFile(exclude)
	if err != nil {
		t.Fatal(err)
	}
	h := newBuiltinChangeHandler(".", false)
	rec := postEdit(t, h, builtinPreviewPath, `{"name":"memory","enabled":true}`, nil)
	if rec.Code == http.StatusOK || !strings.Contains(rec.Body.String(), "need a Git exclusion outside this project; use agnostic-ai sync in the CLI") {
		t.Fatalf("linked worktree preview: %d %s", rec.Code, rec.Body)
	}
	if after, err := os.ReadFile(exclude); err != nil || !bytes.Equal(after, before) {
		t.Errorf("preview changed info/exclude: %q, %v", after, err)
	}
}
