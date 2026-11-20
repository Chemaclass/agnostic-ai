package integration

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestVibeRoundTrip_NativeProject(t *testing.T) {
	binary := builtBinary(t)
	dir := testutil.TempCwd(t)
	native := map[string]string{
		"AGENTS.md":                         "# Project\n\nUse small changes.\n",
		"backend/AGENTS.md":                 "Return contextual errors.\n",
		".vibe/agents/reviewer.toml":        "agent_type='subagent'\ndescription='Review patches'\ninstructions='Review changes.'\nenabled_tools=['read_file']\n",
		".vibe/skills/review/SKILL.md":      "---\nname: review\ndescription: " + strings.Repeat("\U000000E9", 600) + "\ndisable-model-invocation: true\nallowed-tools: [read_file, ask_user_question]\n---\nRead reference.txt.\n",
		".vibe/skills/review/reference.txt": "Check errors.\n",
		".vibe/config.toml":                 "active_model='devstral'\nenable_mouse_capture=false\n[[mcp_servers]]\nname='docs.api'\ntransport='streamable-http'\nurl='https://example.test/mcp'\n[mcp_servers.auth]\ntype='static'\napi_key_env='DOCS_TOKEN'\n",
		".vibe/hooks.toml":                  "[[hooks]]\nname='guard'\ntype='post_agent'\ncommand=\"echo '{}'\"\nstrict=false\n",
	}
	strictByEvent := map[string]bool{
		"pre_tool": false, "post_tool": false, "pre_agent": true, "post_agent": false,
		"notification": false, "session_start": false, "session_end": false,
		"pre_llm": true, "post_llm": true,
	}
	for event, strict := range strictByEvent {
		native[".vibe/hooks.toml"] += fmt.Sprintf("[[hooks]]\nname=%q\ntype=%q\ncommand=\"echo done\"\nmatch='main'\nstrict=%t\n", strings.ReplaceAll(event, "_", "-"), event, strict)
	}
	must(t, os.WriteFile("agnostic-ai.yaml", []byte("version: 1\ntargets: [vibe]\n"), 0o644))
	for path, body := range native {
		must(t, os.MkdirAll(filepath.Dir(path), 0o755))
		must(t, os.WriteFile(path, []byte(body), 0o644))
	}
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(binary, args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}
	run("import", "vibe")
	for path := range native {
		must(t, os.Remove(path))
	}
	run("sync", "-t", "vibe")
	run("sync", "--check", "-t", "vibe")
	run("lint", "--strict")
	run("validate")
	var hooks struct {
		Hooks []struct {
			Name, Type, Match, Command string
			Strict                     bool
		}
	}
	_, err := toml.DecodeFile(".vibe/hooks.toml", &hooks)
	must(t, err)
	wantHooks := len(strictByEvent) + 1
	for _, hook := range hooks.Hooks {
		if hook.Name == "guard" {
			if hook.Type != "post_agent" || hook.Strict || !strings.Contains(hook.Command, "echo '{}'") {
				t.Errorf("unexpected native guard: %+v", hook)
			}
			continue
		}
		strict, expected := strictByEvent[hook.Type]
		if !expected || hook.Match != "main" || hook.Strict != strict || !strings.Contains(hook.Command, "echo done") {
			t.Errorf("unexpected native hook: %+v", hook)
		}
		delete(strictByEvent, hook.Type)
	}
	if len(strictByEvent) > 0 || len(hooks.Hooks) != wantHooks {
		t.Errorf("got %d native hooks, want %d. Missing hook types: %v", len(hooks.Hooks), wantHooks, strictByEvent)
	}
	for path, wants := range map[string][]string{
		"AGENTS.md":                         {"Use small changes."},
		"backend/AGENTS.md":                 {"Return contextual errors."},
		".vibe/agents/reviewer.toml":        {`agent_type = "subagent"`, `enabled_tools = ["read_file"]`},
		".vibe/skills/review/SKILL.md":      {strings.Repeat("\U000000E9", 600), "disable-model-invocation: true", "read_file", "ask_user_question", "Read reference.txt."},
		".vibe/skills/review/reference.txt": {"Check errors.\n"},
		".vibe/config.toml":                 {`name = "docs.api"`, `active_model = "devstral"`, `enable_mouse_capture = false`, `api_key_env = "DOCS_TOKEN"`},
	} {
		data, err := os.ReadFile(path)
		must(t, err)
		for _, want := range wants {
			if !strings.Contains(string(data), want) {
				t.Errorf("%s missing %q: %s", path, want, data)
			}
		}
		if filepath.Ext(path) == ".toml" {
			_, err := toml.Decode(string(data), new(map[string]any))
			must(t, err)
		}
	}
}
