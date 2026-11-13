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
		".vibe/skills/review/SKILL.md":      "---\nname: review\ndescription: Review patches\ndisable-model-invocation: true\nallowed-tools: [read_file, ask_user_question]\n---\nRead reference.txt.\n",
		".vibe/skills/review/reference.txt": "Check errors.\n",
		".vibe/config.toml":                 "active_model='devstral'\nenable_mouse_capture=false\n[[mcp_servers]]\nname='docs.api'\ntransport='streamable-http'\nurl='https://example.test/mcp'\n[mcp_servers.auth]\ntype='static'\napi_key_env='DOCS_TOKEN'\n",
		".vibe/hooks.toml":                  "[[hooks]]\nname='guard'\ntype='post_agent'\ncommand=\"echo '{}'\"\nstrict=false\n",
	}
	for _, event := range []string{"pre_agent", "notification", "session_start", "session_end"} {
		native[".vibe/hooks.toml"] += fmt.Sprintf("[[hooks]]\nname=%q\ntype=%q\ncommand=\"echo done\"\nmatch='main'\nstrict=%t\n", strings.ReplaceAll(event, "_", "-"), event, event == "pre_agent")
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
	run("lint")
	run("validate")
	for path, wants := range map[string][]string{
		"AGENTS.md":                         {"Use small changes."},
		"backend/AGENTS.md":                 {"Return contextual errors."},
		".vibe/agents/reviewer.toml":        {`agent_type = "subagent"`, `enabled_tools = ["read_file"]`},
		".vibe/skills/review/SKILL.md":      {"disable-model-invocation: true", "read_file", "ask_user_question", "Read reference.txt."},
		".vibe/skills/review/reference.txt": {"Check errors.\n"},
		".vibe/config.toml":                 {`name = "docs.api"`, `active_model = "devstral"`, `enable_mouse_capture = false`, `api_key_env = "DOCS_TOKEN"`},
		".vibe/hooks.toml":                  {`type = "post_agent"`, `type = "pre_agent"`, `type = "notification"`, `type = "session_start"`, `type = "session_end"`, `match = "main"`, `strict = true`, `strict = false`},
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
