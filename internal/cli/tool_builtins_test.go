package cli

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

const nativeRTKHook = "command -v rtk >/dev/null 2>&1 || exit 0; rtk hook claude"

func toolBuiltinConfig(t *testing.T, dir, names string) {
	t.Helper()
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude, codex]\nbuiltins: ["+names+"]\n")
}

func toolBuiltinSync(t *testing.T, args ...string) {
	t.Helper()
	if out, err := runBuiltinCLI(t, append([]string{"sync", "--gitignore=off"}, args...)...); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
}

func TestToolBuiltins_DefaultOffAndIndependent(t *testing.T) {
	for _, names := range []string{"", "rtk", "caveman", "rtk, caveman"} {
		t.Run(names, func(t *testing.T) {
			dir := builtinProject(t, names)
			_, bundle, err := loadProject(dir)
			if err != nil {
				t.Fatal(err)
			}
			wantHook, wantSkill := 0, 0
			if strings.Contains(names, "rtk") {
				wantHook = 1
			}
			if strings.Contains(names, "caveman") {
				wantSkill = 1
			}
			if len(bundle.Hooks) != wantHook || len(bundle.Skills) != wantSkill {
				t.Fatalf("%q loaded hooks %d skills %d", names, len(bundle.Hooks), len(bundle.Skills))
			}
			toolBuiltinSync(t)
			toolBuiltinSync(t, "--check")
		})
	}
	t.Run("omitted", func(t *testing.T) {
		dir := builtinProject(t, "")
		writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\n")
		_, bundle, err := loadProject(dir)
		if err != nil || len(bundle.Hooks) != 0 || len(bundle.Skills) != 0 {
			t.Fatalf("omitted config: %+v, %v", bundle, err)
		}
	})
	if config := renderConfig("", []string{"claude"}, false, "dev"); strings.Contains(config, "rtk") || strings.Contains(config, "caveman") {
		t.Errorf("init enables tools: %s", config)
	}
}

func TestToolBuiltins_RemovalPreservesOtherOwners(t *testing.T) {
	dir := builtinProject(t, "rtk, caveman")
	settings := filepath.Join(dir, ".claude", "settings.json")
	writeFile(t, settings, `{"env":{"KEEP":"sentinel"},"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"./guard.sh"},{"type":"command","command":"rtk hook claude"}]}]}}`)
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "skills", "other.md"), "---\nname: other\ndescription: Other owner\n---\nKeep me.\n")
	for _, names := range []string{"rtk, caveman", "caveman", "rtk, caveman", "rtk", ""} {
		toolBuiltinConfig(t, dir, names)
		toolBuiltinSync(t)
		data, err := os.ReadFile(settings)
		if err != nil {
			t.Fatal(err)
		}
		if !json.Valid(data) || !bytes.Contains(data, []byte(`"sentinel"`)) || !bytes.Contains(data, []byte("./guard.sh")) || !bytes.Contains(data, []byte(`"command": "rtk hook claude"`)) {
			t.Errorf("unrelated settings or native owner lost for %q: %s", names, data)
		}
		if got := bytes.Count(data, []byte(nativeRTKHook)); got != boolCount(strings.Contains(names, "rtk")) {
			t.Errorf("%q hook count %d: %s", names, got, data)
		}
		for _, target := range []string{".claude", ".agents"} {
			_, err := os.Stat(filepath.Join(dir, target, "skills", "caveman", "SKILL.md"))
			if strings.Contains(names, "caveman") && err != nil || !strings.Contains(names, "caveman") && !os.IsNotExist(err) {
				t.Errorf("%q %s skill: %v", names, target, err)
			}
			if _, err := os.Stat(filepath.Join(dir, target, "skills", "other", "SKILL.md")); err != nil {
				t.Errorf("other skill removed: %v", err)
			}
		}
		toolBuiltinSync(t, "--check")
	}
}

func boolCount(value bool) int {
	if value {
		return 1
	}
	return 0
}

func TestToolBuiltins_AdoptsAnIdenticalNativeHook(t *testing.T) {
	dir := builtinProject(t, "rtk")
	settings := filepath.Join(dir, ".claude", "settings.json")
	writeFile(t, settings, `{"env":{"KEEP":"sentinel"},"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"`+nativeRTKHook+`"}]}]}}`)
	toolBuiltinSync(t)
	data, err := os.ReadFile(settings)
	if err != nil {
		t.Fatal(err)
	}
	if got := bytes.Count(data, []byte(nativeRTKHook)); got != 1 {
		t.Errorf("identical handler count %d: %s", got, data)
	}
	toolBuiltinConfig(t, dir, "")
	toolBuiltinSync(t)
	data, err = os.ReadFile(settings)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte(nativeRTKHook)) || !bytes.Contains(data, []byte("sentinel")) {
		t.Errorf("adopted hook removal changed unrelated settings: %s", data)
	}
	toolBuiltinSync(t, "--check")
}

func TestToolBuiltins_ProvenanceAndNativeKinds(t *testing.T) {
	builtinProject(t, "rtk, caveman")
	toolBuiltinSync(t)
	for _, args := range [][]string{{"list"}, {"status"}, {"explain", "builtin:rtk"}, {"explain", "builtin:caveman"}, {"render", "builtin:rtk"}, {"render", "builtin:caveman"}, {"lint"}} {
		out, err := runBuiltinCLI(t, args...)
		if err != nil {
			t.Errorf("%v: %v\n%s", args, err, out)
		}
		if strings.Contains(out, "agnostic-ai/builtins/") || strings.Contains(out, "agnostic-ai-builtin-") {
			t.Errorf("%v leaks materialization path: %s", args, out)
		}
		if args[0] == "list" && (!strings.Contains(out, "hook\trtk-shell-output\tbuiltin\tbuiltin:rtk") || !strings.Contains(out, "skill\tcaveman\tbuiltin\tbuiltin:caveman")) {
			t.Errorf("list lacks native kinds and builtin provenance: %s", out)
		}
		if args[0] == "explain" && args[1] == "builtin:rtk" && strings.Contains(out, ".codex/hooks.json") {
			t.Errorf("RTK explanation claims a Codex output: %s", out)
		}
	}
}

func TestToolBuiltins_RemovalKeepsProjectOverrides(t *testing.T) {
	dir := builtinProject(t, "rtk, caveman")
	hookSource := filepath.Join(dir, ".agnostic-ai", "hooks", "rtk.yaml")
	skillSource := filepath.Join(dir, ".agnostic-ai", "skills", "caveman.md")
	hookBody := "name: rtk-shell-output\ntarget: claude\nevent: PreToolUse\nmatcher: Bash\ncommand: ./local-rtk.sh\n"
	skillBody := "---\nname: caveman\ndescription: Project response skill\n---\nProject skill body.\n"
	writeFile(t, hookSource, hookBody)
	writeFile(t, skillSource, skillBody)
	for _, names := range []string{"rtk, caveman", ""} {
		toolBuiltinConfig(t, dir, names)
		toolBuiltinSync(t)
		settings, err := os.ReadFile(filepath.Join(dir, ".claude", "settings.json"))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(settings, []byte("./local-rtk.sh")) || bytes.Contains(settings, []byte(nativeRTKHook)) {
			t.Errorf("%q lost project hook precedence: %s", names, settings)
		}
		for _, target := range []string{".claude", ".agents"} {
			skill, err := os.ReadFile(filepath.Join(dir, target, "skills", "caveman", "SKILL.md"))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(skill, []byte("Project skill body.")) {
				t.Errorf("%q lost project skill: %s", names, skill)
			}
		}
		toolBuiltinSync(t, "--check")
	}
	for path, body := range map[string]string{hookSource: hookBody, skillSource: skillBody} {
		data, err := os.ReadFile(path)
		if err != nil || string(data) != body {
			t.Errorf("source %s changed: %v %s", path, err, data)
		}
	}
}

func TestToolBuiltins_DeterministicGenerationAndMissingRTK(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("native hook uses a POSIX shell")
	}
	dir := builtinProject(t, "rtk, caveman")
	emptyPath := t.TempDir()
	t.Setenv("PATH", emptyPath)
	toolBuiltinSync(t)
	settings := filepath.Join(dir, ".claude", "settings.json")
	absent, err := os.ReadFile(settings)
	if err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(dir, "binary-ran")
	writeFile(t, filepath.Join(emptyPath, "rtk"), "#!/bin/sh\nprintf executed > '"+marker+"'\nexit 1\n")
	if err := os.Chmod(filepath.Join(emptyPath, "rtk"), 0o755); err != nil {
		t.Fatal(err)
	}
	toolBuiltinSync(t)
	present, err := os.ReadFile(settings)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(absent, present) {
		t.Error("RTK installation changes generated bytes")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("sync executed RTK: %v", err)
	}
	payloadMarker := filepath.Join(dir, "payload-ran")
	payload := "printf executed > '" + payloadMarker + "'"
	negative := exec.Command("/bin/sh", "-c", payload)
	negative.Env = []string{"PATH=" + t.TempDir()}
	if out, err := negative.CombinedOutput(); err != nil {
		t.Fatalf("negative control: %v %s", err, out)
	}
	if _, err := os.Stat(payloadMarker); err != nil {
		t.Fatal("negative control did not execute payload")
	}
	if err := os.Remove(payloadMarker); err != nil {
		t.Fatal(err)
	}
	request, err := json.Marshal(map[string]any{"tool_name": "Bash", "tool_input": map[string]any{"command": payload}})
	if err != nil {
		t.Fatal(err)
	}
	hook := exec.Command("/bin/sh", "-c", nativeRTKHook)
	hook.Env = []string{"PATH=" + t.TempDir()}
	hook.Stdin = bytes.NewReader(request)
	if out, err := hook.CombinedOutput(); err != nil || len(out) != 0 {
		t.Errorf("missing RTK: %v %q", err, out)
	}
	if _, err := os.Stat(payloadMarker); !os.IsNotExist(err) {
		t.Errorf("missing RTK executed payload: %v", err)
	}
}
