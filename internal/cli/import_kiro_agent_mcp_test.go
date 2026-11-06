package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestImportKiro_AgentServersFromProjectFileRoundTripAsNames(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [kiro]\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai/agents/rev.md"), "---\nname: rev\ndescription: Reviews\nmcpServers: [db]\n---\n\nReview code.\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai/mcps/db.yaml"), "name: db\ncommand: db-mcp\nargs: [--port, \"5432\"]\n")
	execCLI(t, "sync", "-t", "kiro")
	first := snapshotEmitted(t, dir)
	if !strings.Contains(first[".kiro/agents/rev.md"], "mcpServers:") {
		t.Fatalf("first sync wrote no agent servers:\n%s", first[".kiro/agents/rev.md"])
	}

	if err := os.Remove(filepath.Join(dir, ".agnostic-ai/agents/rev.md")); err != nil {
		t.Fatal(err)
	}
	execCLI(t, "import", "kiro")
	if got := readFile(t, filepath.Join(dir, ".agnostic-ai/agents/rev.md")); !strings.Contains(got, "mcpServers: [db]") || strings.Contains(got, "command:") {
		t.Errorf("imported agent does not name the db spec:\n%s", got)
	}

	execCLI(t, "sync", "-t", "kiro")
	assertEmittedEqual(t, first, snapshotEmitted(t, dir))
}

func TestImportKiro_AgentServersMissingFromProjectFileMoveUnderXKiro(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [kiro]\n")
	writeFile(t, filepath.Join(dir, kiroMCPFile), `{"mcpServers":{"db":{"command":"db-mcp"}}}`)
	writeFile(t, filepath.Join(dir, kiroAgentsDir, "rev.md"), "---\ndescription: Reviews\nmcpServers:\n  db:\n    command: db-mcp\n  local:\n    command: local-mcp\n    timeout: 5000\n---\n\nReview code.\n")

	execCLI(t, "import", "kiro")
	got := readFile(t, filepath.Join(dir, ".agnostic-ai/agents/rev.md"))
	if !strings.Contains(got, "x-kiro:\n  mcpServers:\n") || strings.HasPrefix(got, "---\ndescription: Reviews\nmcpServers:") {
		t.Errorf("imported agent does not keep the inline servers under x-kiro:\n%s", got)
	}

	execCLI(t, "sync", "-t", "kiro")
	agent := readFile(t, filepath.Join(dir, kiroAgentsDir, "rev.md"))
	for _, want := range []string{"mcpServers:", "command: db-mcp", "command: local-mcp", "timeout: 5000"} {
		if !strings.Contains(agent, want) {
			t.Errorf("synced agent lost %q:\n%s", want, agent)
		}
	}
}

func TestImportKiroAgentDocument_MovedServersBeforeNameKeepDisplayName(t *testing.T) {
	in := "---\nmcpServers:\n  local:\n    command: local-mcp\nname: Display\ndescription: Reviews\n---\n\nReview code.\n"
	got, err := importKiroAgentDocument([]byte(in), "rev", nil)
	if err != nil {
		t.Fatal(err)
	}
	out := string(got)
	for _, want := range []string{"description: Reviews", "x-kiro:", "name: Display", "command: local-mcp"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "\nname: Display") {
		t.Errorf("display name left at top level:\n%s", out)
	}
}

func TestImportKiro_AgentServerTimeoutsKeepTheMapUnderXKiro(t *testing.T) {
	for _, key := range []string{"timeout", "requestTimeout"} {
		t.Run(key, func(t *testing.T) {
			dir := t.TempDir()
			testutil.Chdir(t, dir)
			silence(t)
			writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [kiro]\n")
			writeFile(t, filepath.Join(dir, kiroMCPFile), `{"mcpServers":{"db":{"command":"db-mcp","`+key+`":5000}}}`)
			writeFile(t, filepath.Join(dir, kiroAgentsDir, "rev.md"), "---\ndescription: Reviews\nmcpServers:\n  db:\n    command: db-mcp\n    "+key+": 5000\n---\n\nReview code.\n")

			execCLI(t, "import", "kiro")
			execCLI(t, "sync", "-t", "kiro")
			if agent := readFile(t, filepath.Join(dir, kiroAgentsDir, "rev.md")); !strings.Contains(agent, key+": 5000") {
				t.Errorf("synced agent lost %s:\n%s", key, agent)
			}
		})
	}
}

func TestImportKiro_AgentServersScopedInTheSpecStayAMap(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [kiro]\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai/agents/rev.md"), "---\nname: rev\ndescription: Reviews\nx-kiro:\n  mcpServers:\n    db:\n      command: db-mcp\n---\n\nReview code.\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai/mcps/db.yaml"), "name: db\ncommand: db-mcp\n")
	execCLI(t, "sync", "-t", "kiro")
	native := filepath.Join(dir, kiroAgentsDir, "rev.md")
	writeFile(t, native, strings.Replace(readFile(t, native), "description: Reviews", "description: Changed", 1))

	execCLI(t, "import", "kiro")
	if got := readFile(t, filepath.Join(dir, ".agnostic-ai/agents/rev.md")); !strings.Contains(got, "x-kiro:\n  mcpServers:\n    db:\n      command: db-mcp\n") {
		t.Errorf("imported agent lost its scoped server map:\n%s", got)
	}

	execCLI(t, "sync", "-t", "kiro")
	if agent := readFile(t, native); !strings.Contains(agent, "mcpServers:\n  db:\n    command: db-mcp\n") {
		t.Errorf("synced agent lost its servers:\n%s", agent)
	}
}

func TestImportKiro_DeletedAgentServersLeaveTheSpec(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [kiro]\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai/agents/rev.md"), "---\nname: rev\ndescription: Reviews\nmcpServers: [db]\n---\n\nReview code.\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai/mcps/db.yaml"), "name: db\ncommand: db-mcp\n")
	execCLI(t, "sync", "-t", "kiro")
	writeFile(t, filepath.Join(dir, kiroAgentsDir, "rev.md"), "---\ndescription: Reviews\n---\n\nReview code.\n")

	execCLI(t, "import", "kiro")
	if got := readFile(t, filepath.Join(dir, ".agnostic-ai/agents/rev.md")); strings.Contains(got, "mcpServers") {
		t.Errorf("imported agent kept the deleted servers:\n%s", got)
	}
}

func TestImportKiro_MovedAgentServerEnvLiteralBecomesReference(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	log := captureLog(t)
	const secret = "ghp_AAAAbbbbCCCC7777"
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [kiro]\n")
	writeFile(t, filepath.Join(dir, kiroAgentsDir, "rev.md"), "---\ndescription: Reviews\nmcpServers:\n  gh:\n    command: gh-mcp\n    env:\n      GITHUB_TOKEN: "+secret+"\n---\n\nReview code.\n")

	execCLI(t, "import", "kiro")
	got := readFile(t, filepath.Join(dir, ".agnostic-ai/agents/rev.md"))
	if strings.Contains(got, secret) || !strings.Contains(got, "GITHUB_TOKEN: ${GITHUB_TOKEN}") {
		t.Errorf("imported agent does not reference the env value:\n%s", got)
	}
	if out := log.String(); strings.Contains(out, secret) || !strings.Contains(out, "MCP server gh: env GITHUB_TOKEN now reads ${GITHUB_TOKEN}; set GITHUB_TOKEN") {
		t.Errorf("import output lacks the reference note or prints the value:\n%s", out)
	}

	execCLI(t, "sync", "-t", "kiro")
	if agent := readFile(t, filepath.Join(dir, kiroAgentsDir, "rev.md")); !strings.Contains(agent, "GITHUB_TOKEN: ${GITHUB_TOKEN}") {
		t.Errorf("synced agent does not write the reference:\n%s", agent)
	}
}

func TestImportKiro_MovedAgentServerWithOAuthSecretIsLeftOut(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	log := captureLog(t)
	const secret = "ghp_AAAAbbbbCCCC8888"
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [kiro]\n")
	writeFile(t, filepath.Join(dir, kiroAgentsDir, "rev.md"), "---\ndescription: Reviews\nmcpServers:\n  gh:\n    url: https://gh.example/mcp\n    oauth:\n      clientId: app\n      clientSecret: "+secret+"\n  local:\n    command: local-mcp\n---\n\nReview code.\n")

	execCLI(t, "import", "kiro")
	got := readFile(t, filepath.Join(dir, ".agnostic-ai/agents/rev.md"))
	if strings.Contains(got, secret) || strings.Contains(got, "gh:") || !strings.Contains(got, "command: local-mcp") {
		t.Errorf("imported agent keeps the OAuth secret or loses the other server:\n%s", got)
	}
	if out := log.String(); strings.Contains(out, secret) || !strings.Contains(out, "MCP server gh: left out; oauth.clientSecret holds a credential import cannot rewrite") {
		t.Errorf("import output lacks the left-out note or prints the value:\n%s", out)
	}
}
