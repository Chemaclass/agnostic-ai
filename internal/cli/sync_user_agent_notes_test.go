package cli

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncUser_NotesAgentFieldsTheTargetDoesNotWrite(t *testing.T) {
	_, source := userAgentTestHome(t)
	writeFile(t, filepath.Join(source, "agents", "reviewer.md"), "---\nname: reviewer\ndescription: Review code\nmaxTurns: 5\nbackground: true\n---\nReview carefully.\n")

	out, warnings, err := runUserAgentTest("--only", "codex", "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	all := out + warnings
	for _, field := range []string{"`maxTurns`", "`background`"} {
		if n := strings.Count(all, field+" on 1 agent has no effect on codex"); n != 1 {
			t.Errorf("note for %s printed %d times, want once:\n%s", field, n, all)
		}
	}
}

func TestSyncUser_PrintsQwensOwnAgentNoteOnce(t *testing.T) {
	_, source := userAgentTestHome(t)
	writeFile(t, filepath.Join(source, "agents", "reviewer.md"), "---\nname: reviewer\ndescription: Review code\npermissionMode: plan\neffort: high\n---\nReview carefully.\n")

	out, warnings, err := runUserAgentTest("--only", "qwen", "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	all := out + warnings
	for _, field := range []string{"`permissionMode`", "`effort`"} {
		if n := strings.Count(all, field+" on 1 agent has no effect on qwen"); n != 1 {
			t.Errorf("note for %s printed %d times, want once:\n%s", field, n, all)
		}
	}
}

func TestSyncUser_NotesAgentServersAnInlineTargetCannotWrite(t *testing.T) {
	for _, target := range []string{"kiro", "openhands"} {
		t.Run(target, func(t *testing.T) {
			_, source := userAgentTestHome(t)
			writeFile(t, filepath.Join(source, "agents", "reviewer.md"), "---\nname: reviewer\ndescription: Review code\nmcpServers: [db]\n---\nReview carefully.\n")
			writeFile(t, filepath.Join(source, "mcps", "db.yaml"), "name: db\ncommand: db-mcp\n")

			out, warnings, err := runUserAgentTest("--only", target, "--dry-run")
			if err != nil {
				t.Fatal(err)
			}
			if all := out + warnings; strings.Count(all, "`mcpServers` on 1 agent has no effect on "+target) != 1 {
				t.Errorf("want one mcpServers note:\n%s", all)
			}
		})
	}
}

func TestSyncUser_NoAgentServerNoteWhenOpenHandsServersAreSetNatively(t *testing.T) {
	_, source := userAgentTestHome(t)
	writeFile(t, filepath.Join(source, "agents", "reviewer.md"), "---\nname: reviewer\ndescription: Review code\nmcpServers: [db]\nx-openhands:\n  mcp_servers:\n    db:\n      command: db-mcp\n---\nReview carefully.\n")
	writeFile(t, filepath.Join(source, "mcps", "db.yaml"), "name: db\ncommand: db-mcp\n")

	out, warnings, err := runUserAgentTest("--only", "openhands", "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	if all := out + warnings; strings.Contains(all, "`mcpServers` on 1 agent") {
		t.Errorf("x-openhands.mcp_servers writes the servers, so no note is due:\n%s", all)
	}
}
