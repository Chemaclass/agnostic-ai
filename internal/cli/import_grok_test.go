package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func seedGrokNative(t *testing.T, dir string) {
	t.Helper()
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [grok]\ngitignore:\n  enabled: true\n  commit: [grok:instructions]\n")
	files := map[string]string{
		".grok/rules/style.md": "Keep functions short.\n", "backend/.grok/rules/database.md": "Use transactions.\n",
		".grok/agents/reviewer.md":     "---\nname: reviewer\ndescription: Review changes\ntools: [read_file]\neffort: high\nmaxTurns: 5\n---\n\nFind defects.\n",
		".grok/skills/review/SKILL.md": "---\nname: review\ndescription: Review work\ndisable-model-invocation: true\n---\n\nRead [guide](guide.txt).\n", ".grok/skills/review/guide.txt": "Read the tests.\n",
		".grok/commands/check.md": "---\ndescription: Check work\n---\n\nCheck $ARGUMENTS.\n",
		".grok/hooks/guard.json":  "{\"hooks\":{\"PreToolUse\":[{\"matcher\":\"Bash\",\"hooks\":[{\"type\":\"command\",\"command\":\"echo ok\",\"env\":{\"MODE\":\"safe\"},\"timeout\":7}]}]}}\n",
		".grok/config.toml":       "[mcp_servers.local]\ncommand = \"server\"\nargs = [\"--stdio\"]\nenabled = false\n[permission]\ndeny = [\"Bash(git push *)\"]\n",
	}
	for p, data := range files {
		writeFile(t, filepath.Join(dir, p), data)
	}
}
func TestImportGrok_NativeProjectRoundTrip(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	seedGrokNative(t, dir)
	if out, err := exec.Command("git", "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	execCLI(t, "import", "grok")
	execCLI(t, "sync", "-t", "grok")
	for path, want := range map[string]string{".grok/rules/style.md": "Keep functions short.", "backend/.grok/rules/database.md": "Use transactions.", ".grok/agents/reviewer.md": "read_file", ".grok/skills/review/guide.txt": "Read the tests.", ".grok/commands/check.md": "$ARGUMENTS", ".grok/config.toml": "enabled = false", ".grok/hooks/guard.json": "MODE"} {
		if got := readFile(t, filepath.Join(dir, path)); !strings.Contains(got, want) {
			t.Errorf("%s lost %q: %s", path, want, got)
		}
	}
	if output, err := exec.Command("git", "check-ignore", "--no-index", ".grok/rules/style.md", "backend/.grok/rules/database.md").CombinedOutput(); err == nil || len(output) > 0 {
		t.Fatalf("native rules ignored: %v %s", err, output)
	}
	execCLI(t, "sync", "--check")
	execCLI(t, "lint")
	first := snapshotEmitted(t, dir)
	execCLI(t, "import", "grok")
	execCLI(t, "sync", "-t", "grok")
	assertEmittedEqual(t, first, snapshotEmitted(t, dir))
	execCLI(t, "sync", "--check")
}
func TestImportGrok_EmittedProjectImportsIntoFreshSources(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	seedGrokNative(t, dir)
	execCLI(t, "import", "grok")
	execCLI(t, "sync", "-t", "grok")
	if err := os.RemoveAll(filepath.Join(dir, ".agnostic-ai")); err != nil {
		t.Fatal(err)
	}
	execCLI(t, "import", "grok")
	execCLI(t, "sync", "-t", "grok")
	execCLI(t, "sync", "--check")
	execCLI(t, "lint")
}
func TestImportGrok_DirectoryUnionNeverBroadensToRoot(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [grok]\ngitignore:\n  enabled: true\n  commit: [grok:instructions]\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai/rules/style.md"), "---\nname: style\nglobs: [a/**, b/**]\n---\nUse transactions.\n")
	execCLI(t, "sync", "-t", "grok")
	for _, path := range []string{"a/.grok/rules/style.md", "b/.grok/rules/style.md"} {
		if got := readFile(t, path); !strings.Contains(got, "Use transactions.") {
			t.Errorf("missing scoped rule %s", path)
		}
	}
	if _, err := os.Stat(".grok/rules/style.md"); !os.IsNotExist(err) {
		t.Fatalf("rule broadened to root %v", err)
	}
}

func TestImportGrok_ReleasesOnlyOwnedProjectConfig(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	seedGrokNative(t, dir)
	execCLI(t, "import", "grok")
	path := filepath.Join(dir, ".grok/config.toml")
	writeFile(t, path, "[plugins]\nkeep = true\n[mcp_servers.manual]\ncommand = \"manual\"\n[permission]\nallow = [\"Bash(manual)\"]\n")
	execCLI(t, "sync", "-t", "grok")
	if err := os.Remove(filepath.Join(dir, ".agnostic-ai/mcps/local.yaml")); err != nil {
		t.Fatal(err)
	}
	execCLI(t, "sync", "-t", "grok")
	if got := readFile(t, path); strings.Contains(got, "mcp_servers.local") {
		t.Errorf("removed MCP lingers %s", got)
	}
	if err := os.Remove(filepath.Join(dir, ".agnostic-ai/settings/grok.yaml")); err != nil {
		t.Fatal(err)
	}
	execCLI(t, "sync", "-t", "grok")
	got := readFile(t, path)
	for _, want := range []string{"plugins", "mcp_servers.manual", "Bash(manual)"} {
		if !strings.Contains(got, want) {
			t.Errorf("user config lost %q: %s", want, got)
		}
	}
	if strings.Contains(got, "Bash(git push *)") {
		t.Errorf("owned permission remained %s", got)
	}
	execCLI(t, "sync", "--check")
}
func TestImportGrok_SplitSettingsImports(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [grok]\noutputs:\n  grok:\n    conf-file: permissions.toml\n")
	writeFile(t, filepath.Join(dir, "permissions.toml"), "[permission]\ndeny = [\"Bash(git push *)\"]\n")
	execCLI(t, "import", "grok")
	got := readFile(t, filepath.Join(dir, ".agnostic-ai/settings/grok.yaml"))
	if !strings.Contains(got, "Bash(git push *)") {
		t.Fatalf("lost split permissions: %s", got)
	}
}

func TestImportGrok_BundledHookScriptReconstructsFromSources(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [grok]\n")
	script := "#!/bin/sh\necho safe\n"
	writeFile(t, filepath.Join(dir, ".grok/hook-scripts/guard.sh"), script)
	writeFile(t, filepath.Join(dir, ".grok/hooks/guard.json"), `{"hooks":{"PreToolUse":[{"hooks":[{"type":"command","command":".grok/hook-scripts/guard.sh"}]}]}}`)
	execCLI(t, "import", "grok")
	source := readFile(t, filepath.Join(dir, ".agnostic-ai/scripts/grok/guard.sh"))
	if source != script {
		t.Fatalf("script not captured %q", source)
	}
	if err := os.RemoveAll(filepath.Join(dir, ".grok")); err != nil {
		t.Fatal(err)
	}
	execCLI(t, "sync", "-t", "grok")
	if got := readFile(t, filepath.Join(dir, ".grok/hook-scripts/guard.sh")); got != script {
		t.Errorf("script body changed %q", got)
	}
	hook := readFile(t, filepath.Join(dir, ".grok/hooks/guard.json"))
	if !strings.Contains(hook, ".grok/hook-scripts/guard.sh") {
		t.Fatalf("hook no longer reaches script %s", hook)
	}
	execCLI(t, "sync", "--check")
}
func TestImportGrok_NestedHookScriptReconstructsFromSources(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [grok]\n")
	script := "#!/bin/sh\necho safe\n"
	writeFile(t, filepath.Join(dir, ".grok/hook-scripts/bin/guard.sh"), script)
	writeFile(t, filepath.Join(dir, ".grok/hooks/guard.json"), `{"hooks":{"PreToolUse":[{"hooks":[{"type":"command","command":".grok/hook-scripts/bin/guard.sh"}]}]}}`)
	execCLI(t, "import", "grok")
	source := readFile(t, filepath.Join(dir, ".agnostic-ai/scripts/grok/bin/guard.sh"))
	if source != script {
		t.Fatalf("script not captured %q", source)
	}
	if err := os.RemoveAll(filepath.Join(dir, ".grok")); err != nil {
		t.Fatal(err)
	}
	execCLI(t, "sync", "-t", "grok")
	if got := readFile(t, filepath.Join(dir, ".grok/hook-scripts/bin/guard.sh")); got != script {
		t.Errorf("script body changed %q", got)
	}
	hook := readFile(t, filepath.Join(dir, ".grok/hooks/guard.json"))
	if !strings.Contains(hook, ".grok/hook-scripts/bin/guard.sh") {
		t.Fatalf("hook no longer reaches script %s", hook)
	}
	execCLI(t, "sync", "--check")
}
func TestImportGrok_HookCredentialBecomesReference(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [grok]\n")
	writeFile(t, filepath.Join(dir, ".grok/hooks/guard.json"), `{"hooks":{"PreToolUse":[{"hooks":[{"type":"command","command":"echo safe","env":{"API_TOKEN":"private-token-value"}}]}]}}`)
	execCLI(t, "import", "grok")
	got := readFile(t, filepath.Join(dir, ".agnostic-ai/hooks/guard.yaml"))
	if strings.Contains(got, "private-token-value") || !strings.Contains(got, "${") {
		t.Fatalf("credential not filtered %s", got)
	}
}
func TestImportGrok_EmptyNativePermissionListNeverClaimsUserRules(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [grok]\n")
	path := filepath.Join(dir, ".grok/config.toml")
	before := "[permission]\ndeny = ['Read(.env)']\n"
	writeFile(t, path, before)
	source := filepath.Join(dir, ".agnostic-ai/settings/grok.yaml")
	writeFile(t, source, "x-grok:\n  permission:\n    deny: []\n")
	execCLI(t, "sync", "-t", "grok")
	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}
	execCLI(t, "sync", "-t", "grok")
	if got := readFile(t, path); got != before {
		t.Errorf("empty native list claimed user rules %q", got)
	}
}
func TestImportGrok_PortableAgentDropsUnrequestedMCPBridge(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [grok]\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai/agents/reviewer.md"), "---\nname: reviewer\ndescription: Review changes\ncan: [read]\n---\nReview files.\n")
	execCLI(t, "sync", "-t", "grok")
	got := readFile(t, filepath.Join(dir, ".grok/agents/reviewer.md"))
	for _, want := range []string{"- Read", "disallowedTools:", "- search_tool", "- use_tool"} {
		if !strings.Contains(got, want) {
			t.Errorf("portable restriction missing %q: %s", want, got)
		}
	}
}
