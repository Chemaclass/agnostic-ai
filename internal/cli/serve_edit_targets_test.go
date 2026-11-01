package cli

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func targetsProject(t *testing.T, body string) (string, http.Handler) {
	t.Helper()
	silence(t)
	root := t.TempDir()
	testutil.Chdir(t, root)
	mustWrite(t, "agnostic-ai.yaml", body)
	return root, newEditHandler(root, false)
}

func TestEditTargets_SwitchesTheSharedListAndKeepsComments(t *testing.T) {
	for _, tc := range []struct {
		name, config, body, want string
		status                   int
	}{
		{"turn off keeps the comment", "# Keep me.\nversion: 1\ntargets:\n  - claude # main\n  - codex\n", `{"audience":"shared","name":"codex","enabled":false,"expected":true}`, "# Keep me.\nversion: 1\ntargets:\n  - claude # main\n", http.StatusOK},
		{"turn on appends a block item", "version: 1\ntargets:\n  - claude\n", `{"audience":"shared","name":"cursor","enabled":true,"expected":false}`, "version: 1\ntargets:\n  - claude\n  - cursor\n", http.StatusOK},
		{"flow list", "version: 1\ntargets: [claude, codex]\n", `{"audience":"shared","name":"cursor","enabled":true,"expected":false}`, "version: 1\ntargets: [claude, codex, cursor]\n", http.StatusOK},
		{"last tool stays on", "version: 1\ntargets: [claude]\n", `{"audience":"shared","name":"claude","enabled":false,"expected":true}`, "version: 1\ntargets: [claude]\n", http.StatusBadRequest},
		{"unknown name", "version: 1\ntargets: [claude]\n", `{"audience":"shared","name":"cursr","enabled":true,"expected":false}`, "version: 1\ntargets: [claude]\n", http.StatusBadRequest},
		{"stale state", "version: 1\ntargets: [claude]\n", `{"audience":"shared","name":"claude","enabled":true,"expected":false}`, "version: 1\ntargets: [claude]\n", http.StatusConflict},
		{"external name is kept", "version: 1\ntargets: [claude, mytool]\n", `{"audience":"shared","name":"codex","enabled":true,"expected":false}`, "version: 1\ntargets: [claude, mytool, codex]\n", http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, h := targetsProject(t, tc.config)
			rec := postEdit(t, h, editTargetsPath, tc.body, nil)
			if rec.Code != tc.status {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tc.status, rec.Body)
			}
			if got := readOptional(t, "agnostic-ai.yaml"); got != tc.want {
				t.Errorf("config:\n%s\nwant:\n%s", got, tc.want)
			}
		})
	}
}

func TestEditTargets_NoKeyWritesTheDefaultListWithTheSwitchApplied(t *testing.T) {
	_, h := targetsProject(t, "# Head.\nversion: 1\n")
	rec := postEdit(t, h, editTargetsPath, `{"audience":"shared","name":"cursor","enabled":false,"expected":true}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	got := readOptional(t, "agnostic-ai.yaml")
	if !strings.HasPrefix(got, "# Head.\nversion: 1\n") || strings.Contains(got, "cursor") {
		t.Errorf("config:\n%s", got)
	}
	for _, name := range config.DefaultTargets() {
		if name != "cursor" && !strings.Contains(got, "- "+name+"\n") {
			t.Errorf("default %s missing from:\n%s", name, got)
		}
	}
	rec = postEdit(t, h, editTargetsPath, `{"audience":"shared","name":"amp","enabled":false,"expected":false}`, nil)
	if rec.Code != http.StatusOK {
		t.Errorf("turning off a tool that is already off = %d: %s", rec.Code, rec.Body)
	}
}

func TestEditTargets_PersonalSeedsReplacesAndResets(t *testing.T) {
	_, h := targetsProject(t, "version: 1\ntargets: [claude, codex]\n")
	if rec := postEdit(t, h, editTargetsPath, `{"audience":"personal","name":"codex","enabled":false,"expected":true}`, nil); rec.Code != http.StatusOK {
		t.Fatalf("personal off = %d %s", rec.Code, rec.Body)
	}
	local := readOptional(t, "agnostic-ai.local.yaml")
	if !strings.Contains(local, "targets:") || strings.Contains(local, "codex") || !strings.Contains(local, "claude") {
		t.Errorf("personal list:\n%s", local)
	}
	if got := readOptional(t, "agnostic-ai.yaml"); got != "version: 1\ntargets: [claude, codex]\n" {
		t.Errorf("shared changed:\n%s", got)
	}
	if rec := postEdit(t, h, editTargetsPath, `{"audience":"shared","name":"cursor","enabled":true,"expected":false}`, nil); rec.Code != http.StatusBadRequest {
		t.Errorf("shared while personal = %d, want 400: %s", rec.Code, rec.Body)
	}
	if rec := postEdit(t, h, editTargetsPath, `{"audience":"personal","name":"codex","enabled":true,"expected":false}`, nil); rec.Code != http.StatusOK {
		t.Fatalf("personal on = %d %s", rec.Code, rec.Body)
	}
	if strings.Contains(readOptional(t, "agnostic-ai.local.yaml"), "targets") {
		t.Errorf("personal targets list kept:\n%s", readOptional(t, "agnostic-ai.local.yaml"))
	}
	if rec := postEdit(t, h, editTargetsPath, `{"audience":"personal","reset":true,"expected":["claude","codex"]}`, nil); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("reset without a list = %d, want 422: %s", rec.Code, rec.Body)
	}
}

func TestEditTargets_PersonalResetDropsTheList(t *testing.T) {
	_, h := targetsProject(t, "version: 1\ntargets: [claude, codex]\n")
	if rec := postEdit(t, h, editTargetsPath, `{"audience":"personal","name":"codex","enabled":false,"expected":true}`, nil); rec.Code != http.StatusOK {
		t.Fatalf("personal off = %d %s", rec.Code, rec.Body)
	}
	if rec := postEdit(t, h, editTargetsPath, `{"audience":"personal","reset":true,"expected":["claude","codex"]}`, nil); rec.Code != http.StatusConflict {
		t.Errorf("stale reset = %d, want 409: %s", rec.Code, rec.Body)
	}
	if rec := postEdit(t, h, editTargetsPath, `{"audience":"personal","reset":true,"expected":["claude"]}`, nil); rec.Code != http.StatusOK {
		t.Fatalf("reset = %d %s", rec.Code, rec.Body)
	}
	if strings.Contains(readOptional(t, "agnostic-ai.local.yaml"), "targets") {
		t.Errorf("personal list kept:\n%s", readOptional(t, "agnostic-ai.local.yaml"))
	}
}

func TestEditTargets_RefusesBadRequests(t *testing.T) {
	_, h := targetsProject(t, "version: 1\ntargets: [claude]\n")
	for _, body := range []string{
		`{"audience":"shared"}`,
		`{"audience":"shared","name":"codex"}`,
		`{"audience":"shared","reset":true,"enabled":true}`,
		`{"audience":"shared","reset":true,"expected":["claude"]}`,
	} {
		if rec := postEdit(t, h, editTargetsPath, body, nil); rec.Code != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400: %s", body, rec.Code, rec.Body)
		}
	}
	if got := readOptional(t, "agnostic-ai.yaml"); got != "version: 1\ntargets: [claude]\n" {
		t.Errorf("config changed:\n%s", got)
	}
	if _, err := os.Stat("agnostic-ai.local.yaml"); !os.IsNotExist(err) {
		t.Errorf("personal config written: %v", err)
	}
}

func TestEditTargets_GlobalWritesTheHomeConfigAndRefusesProjectOnlyTools(t *testing.T) {
	silence(t)
	source := editHome(t)
	h := newEditHandler(source, true)
	if rec := postEdit(t, h, editTargetsPath, `{"audience":"shared","name":"codex","enabled":true,"expected":false}`, nil); rec.Code != http.StatusOK {
		t.Fatalf("enable = %d %s", rec.Code, rec.Body)
	}
	data, _ := os.ReadFile(source + "/agnostic-ai.yaml")
	if !strings.Contains(string(data), "# Global targets.") || !strings.Contains(string(data), "- codex") {
		t.Errorf("after enabling:\n%s", data)
	}
	if rec := postEdit(t, h, editTargetsPath, `{"audience":"shared","name":"ellipsis","enabled":true,"expected":false}`, nil); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "sync --global cannot write") {
		t.Errorf("project-only tool = %d %s", rec.Code, rec.Body)
	}
}

func TestEditTargets_LastSupportedToolStaysOn(t *testing.T) {
	_, h := targetsProject(t, "version: 1\ntargets: [claude, mytool]\n")
	rec := postEdit(t, h, editTargetsPath, `{"audience":"shared","name":"claude","enabled":false,"expected":true}`, nil)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "keep at least one coding tool on") {
		t.Errorf("project = %d, want 400 naming the rule: %s", rec.Code, rec.Body)
	}
	if got := readOptional(t, "agnostic-ai.yaml"); got != "version: 1\ntargets: [claude, mytool]\n" {
		t.Errorf("config changed:\n%s", got)
	}

	silence(t)
	source := editHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "agnostic-ai.yaml"), "targets: [claude, aider]\n")
	rec = postEdit(t, newEditHandler(source, true), editTargetsPath, `{"audience":"shared","name":"claude","enabled":false,"expected":true}`, nil)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "keep at least one coding tool on") {
		t.Errorf("global = %d, want 400 naming the rule: %s", rec.Code, rec.Body)
	}
}

func TestEditTargets_ResetIgnoresNamesThePanelDoesNotShow(t *testing.T) {
	_, h := targetsProject(t, "version: 1\ntargets: [claude, codex]\n")
	mustWrite(t, "agnostic-ai.local.yaml", "targets: [claude, mytool]\n")
	rec := postEdit(t, h, editTargetsPath, `{"audience":"personal","reset":true,"expected":["claude"]}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("reset = %d %s", rec.Code, rec.Body)
	}
	if strings.Contains(readOptional(t, "agnostic-ai.local.yaml"), "targets") {
		t.Errorf("personal list kept:\n%s", readOptional(t, "agnostic-ai.local.yaml"))
	}
}

func TestEditTargets_OffThenOnRestoresAConfigWithNoTargetsKey(t *testing.T) {
	const original = "# Head.\nversion: 1\n"
	_, h := targetsProject(t, original)
	if rec := postEdit(t, h, editTargetsPath, `{"audience":"shared","name":"cursor","enabled":false,"expected":true}`, nil); rec.Code != http.StatusOK {
		t.Fatalf("off = %d %s", rec.Code, rec.Body)
	}
	if !strings.Contains(readOptional(t, "agnostic-ai.yaml"), "targets:") {
		t.Fatalf("turning a default tool off wrote no list:\n%s", readOptional(t, "agnostic-ai.yaml"))
	}
	if rec := postEdit(t, h, editTargetsPath, `{"audience":"shared","name":"cursor","enabled":true,"expected":false}`, nil); rec.Code != http.StatusOK {
		t.Fatalf("on = %d %s", rec.Code, rec.Body)
	}
	if got := readOptional(t, "agnostic-ai.yaml"); got != original {
		t.Errorf("config after off and on:\n%s\nwant the original:\n%s", got, original)
	}
}
