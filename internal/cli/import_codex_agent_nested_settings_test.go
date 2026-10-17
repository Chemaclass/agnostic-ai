package cli

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/BurntSushi/toml"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestImportFromCodex_AgentNestedSettingsSurviveSync(t *testing.T) {
	testutil.TempCwd(t)
	silence(t)
	captureLogOut(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [codex]\n")
	path := filepath.Join(".codex", "agents", "limited.toml")
	original := `name = "limited"
description = "Limited reviewer"
developer_instructions = "Review changes."
future_strings = { alpha = "one", beta = "two" }

[features]
shell_tool = false
memories = true

[[skills.config]]
path = "/skills/first/SKILL.md"
enabled = false

[[skills.config]]
path = "/skills/second/SKILL.md"
enabled = true
`
	writeFile(t, path, original)
	for _, args := range [][]string{{"import", "codex"}, {"sync"}, {"sync", "--check"}, {"lint"}} {
		root := NewRootCmd("test")
		root.SetArgs(args)
		if err := root.Execute(); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
	}
	var want, got map[string]any
	if _, err := toml.Decode(original, &want); err != nil {
		t.Fatal(err)
	}
	body := readFile(t, path)
	if _, err := toml.Decode(body, &got); err != nil {
		t.Fatalf("synced agent TOML is invalid: %v\n%s", err, body)
	}
	for _, key := range []string{"features", "skills", "future_strings"} {
		if !reflect.DeepEqual(got[key], want[key]) {
			t.Errorf("%s did not survive import/sync: got %#v, want %#v\n%s", key, got[key], want[key], body)
		}
	}
}
