package integration

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const approvalsAllowReply = `{"hookSpecificOutput":{"hookEventName":"PermissionRequest","decision":{"behavior":"allow"}}}`

// approvalsHookCommand reads the one PermissionRequest command in a
// Claude-shaped hooks file, or "" when there is none.
func approvalsHookCommand(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return ""
	}
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		Hooks map[string][]struct {
			Hooks []struct {
				Command string `json:"command"`
				Timeout int    `json:"timeout"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	groups := file.Hooks["PermissionRequest"]
	if len(groups) == 0 {
		return ""
	}
	if len(groups) != 1 || len(groups[0].Hooks) != 1 || groups[0].Hooks[0].Timeout != 10 {
		t.Fatalf("%s: PermissionRequest = %+v, want one handler with timeout 10", path, groups)
	}
	return groups[0].Hooks[0].Command
}

func TestBuiltinApprovals_GlobalSyncWritesTheHookAndProjectSyncDoesNot(t *testing.T) {
	skipEndToEndInShortMode(t)
	if runtime.GOOS == "windows" {
		t.Skip("the approvals hook has no Windows command; approvals fall back to ask there")
	}
	binary := builtBinary(t)
	base := t.TempDir()
	home := filepath.Join(base, "home")
	source := filepath.Join(home, ".agnostic-ai")
	codexHome := filepath.Join(base, "codex")
	project := filepath.Join(base, "project")
	bin := filepath.Join(base, "bin")
	for _, dir := range []string{codexHome, filepath.Join(project, ".git"), bin} {
		must(t, os.MkdirAll(dir, 0o755))
	}
	data, err := os.ReadFile(binary)
	must(t, err)
	must(t, os.WriteFile(filepath.Join(bin, "agnostic-ai"), data, 0o755))
	env := []string{
		"PATH=" + bin + string(os.PathListSeparator) + "/usr/bin" + string(os.PathListSeparator) + "/bin",
		"HOME=" + home, "USERPROFILE=" + home, "AGNOSTIC_AI_HOME=" + source, "CODEX_HOME=" + codexHome,
		"AGNOSTIC_AI_NO_UPDATE_CHECK=1", "TMPDIR=" + os.TempDir(),
	}
	run := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command(binary, args...)
		cmd.Dir, cmd.Env = dir, env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}
	must(t, os.MkdirAll(source, 0o755))
	must(t, os.WriteFile(filepath.Join(source, "agnostic-ai.yaml"), []byte("version: 1\ntargets: [claude, codex]\nbuiltins: [approvals]\napprovals:\n  mode: yolo\n"), 0o644))
	run(base, "sync", "--global")

	files := map[string]string{
		"claude": filepath.Join(home, ".claude", "settings.json"),
		"codex":  filepath.Join(codexHome, "hooks.json"),
	}
	for target, path := range files {
		command := approvalsHookCommand(t, path)
		if !strings.Contains(command, "agnostic-ai hook approve --target "+target) {
			t.Fatalf("%s: PermissionRequest command = %q", target, command)
		}
		hook := func(c string) (stdout, stderr string, code int) {
			t.Helper()
			payload, err := json.Marshal(map[string]any{
				"session_id": "s-" + target, "cwd": project, "permission_mode": "default", "hook_event_name": "PermissionRequest",
				"tool_name": "Bash", "tool_input": map[string]any{"command": c, "description": "Run it"},
			})
			must(t, err)
			cmd := exec.Command("sh", "-c", command)
			cmd.Dir, cmd.Env = project, env
			cmd.Stdin = bytes.NewReader(payload)
			var out, errOut bytes.Buffer
			cmd.Stdout, cmd.Stderr = &out, &errOut
			err = cmd.Run()
			if exit, ok := err.(*exec.ExitError); ok {
				return out.String(), errOut.String(), exit.ExitCode()
			}
			must(t, err)
			return out.String(), errOut.String(), 0
		}
		if out, errOut, code := hook("go test ./..."); out != approvalsAllowReply || errOut != "" || code != 0 {
			t.Errorf("%s go test: stdout %q stderr %q exit %d; want the allow reply, no stderr, exit 0", target, out, errOut, code)
		}
		if out, errOut, code := hook("git push"); out != "" || errOut != "" || code != 0 {
			t.Errorf("%s git push: stdout %q stderr %q exit %d; want nothing and exit 0", target, out, errOut, code)
		}
	}

	must(t, os.WriteFile(filepath.Join(project, "agnostic-ai.yaml"), []byte("version: 1\ntargets: [claude, codex]\nbuiltins: [approvals]\n"), 0o644))
	run(project, "sync", "--gitignore=off")
	for _, rel := range []string{".claude/settings.json", ".codex/hooks.json"} {
		if command := approvalsHookCommand(t, filepath.Join(project, rel)); command != "" {
			t.Errorf("project sync wrote a PermissionRequest hook to %s: %q", rel, command)
		}
	}
}
