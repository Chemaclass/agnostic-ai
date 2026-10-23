package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestQwen_GlobalOrdinaryPortableHooksProduceNativeBlockingReplies(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("executes POSIX hook commands")
	}
	for _, on := range []string{"before-tool", "prompt-submit", "after-tool", "after-edit", "stop"} {
		for _, command := range []string{`printf '{}' >&2; exit 2`, `exit 2`} {
			t.Run(on+"/"+command, func(t *testing.T) {
				home, source := globalAgentTestHome(t)
				mustWriteGlobalTest(t, filepath.Join(source, "hooks/guard.yaml"), "name: guard\non: "+on+"\ncommand: "+jsonString(command)+"\n")
				if _, warnings, err := runGlobalAgentTest("--only", "qwen"); err != nil {
					t.Fatalf("sync: %v\n%s", err, warnings)
				}
				event := map[string]string{"before-tool": "PreToolUse", "prompt-submit": "UserPromptSubmit", "after-tool": "PostToolUse", "after-edit": "PostToolUse", "stop": "Stop"}[on]
				handler := firstGlobalHandler(t, readGlobalJSON(t, filepath.Join(home, ".qwen/settings.json")), event)
				cmd := exec.Command("bash", "-c", handler["command"].(string))
				var stderr strings.Builder
				cmd.Stderr = &stderr
				stdout, err := cmd.Output()
				exit, ok := err.(*exec.ExitError)
				if !ok || exit.ExitCode() != 2 || len(stdout) != 0 {
					t.Fatalf("exit=%v stdout=%q", err, stdout)
				}
				var reply map[string]any
				if err := json.Unmarshal([]byte(stderr.String()), &reply); err != nil {
					t.Fatalf("no native blocking reply: %v, stderr=%q", err, stderr.String())
				}
				if on == "after-tool" || on == "after-edit" {
					if reply["continue"] != false {
						t.Errorf("PostToolUse did not stop: %#v", reply)
					}
				} else if reply["decision"] != "deny" {
					t.Errorf("native denial missing: %#v", reply)
				}
			})
		}
	}
}

func TestQwen_NativeSkillAndSettingsImportIntoFreshOutputs(t *testing.T) {
	dir := testutil.TempCwd(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [qwen]\n")
	writeFile(t, filepath.Join(dir, ".qwen/settings.json"), `{"disableAllHooks":true,"hooks":{"PreToolUse":[{"hooks":[{"type":"command","command":"native-guard"}]}]}}`)
	writeFile(t, filepath.Join(dir, ".qwen/skills/check/SKILL.md"), "---\nname: check\ndescription: Check\nmodel: custom-model\n---\nCheck carefully.\n")
	if out, err := runCLI(t, "import", "qwen"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}
	if err := os.Remove(filepath.Join(dir, ".qwen/settings.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, ".qwen/skills/check/SKILL.md")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, ".agnostic-ai/hooks/generated.yaml"), "event: PreToolUse\ncommand: echo generated\n")
	for _, args := range [][]string{{"sync"}, {"sync", "--check"}, {"lint"}} {
		if out, err := runCLI(t, args...); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}
	native := readGlobalJSON(t, filepath.Join(dir, ".qwen/settings.json"))
	if native["disableAllHooks"] != true {
		t.Errorf("top-level disableAllHooks lost: %#v", native)
	}
	groups := native["hooks"].(map[string]any)["PreToolUse"].([]any)
	if len(groups) != 2 {
		t.Errorf("native and generated guards must both survive fresh sync: %#v", groups)
	}
	skill := readGlobalTest(t, filepath.Join(dir, ".qwen/skills/check/SKILL.md"))
	if !strings.Contains(skill, "model: custom-model") {
		t.Errorf("native skill model lost: %s", skill)
	}
}

func TestQwen_GlobalNativeSkillModelIsPreserved(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "skills/check/SKILL.md"), "---\nname: check\ndescription: Check\nx-qwen:\n  model: custom-model\n---\nCheck carefully.\n")
	out, warnings, err := runGlobalAgentTest("--only", "qwen")
	if err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}
	native := readGlobalTest(t, filepath.Join(home, ".qwen/skills/check/SKILL.md"))
	if !strings.Contains(native, "model: custom-model") {
		t.Errorf("native skill model lost: %s", native)
	}
	if strings.Contains(out+warnings, "`model` on") {
		t.Errorf("preserved native model reported unsupported: %s\n%s", out, warnings)
	}
}
