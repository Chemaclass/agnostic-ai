package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// A native rule for a server whose name ends in `_` stays a native rule
// on import, so the next sync writes it back unchanged.
func TestImport_ClaudeKeepsBareRuleForTrailingUnderscoreServer(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\n")
	native := "{\n  \"permissions\": {\n    \"allow\": [\n      \"mcp__api_\",\n      \"mcp__reports__v2__*\"\n    ]\n  }\n}\n"
	settings := filepath.Join(dir, ".claude", "settings.json")
	mustWrite(t, settings, native)
	if out, err := runCLI(t, "import", "claude"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}
	spec, err := os.ReadFile(filepath.Join(dir, ".agnostic-ai", "settings", "permissions-claude.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(spec), "mcp:api_") {
		t.Errorf("import rewrote the bare rule to mcp:api_:\n%s", spec)
	}
	mustSync(t)
	got, err := os.ReadFile(settings)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), `"mcp__api_"`) || !strings.Contains(string(got), `"mcp__reports__v2__*"`) || strings.Contains(string(got), "mcp__api___*") {
		t.Errorf("sync changed the imported rules:\n%s", got)
	}
	if out, err := runCLI(t, "sync", "--check"); err != nil {
		t.Errorf("sync --check: %v\n%s", err, out)
	}
}

func TestSync_ClaudeSkillAllowedToolsWriteWildcardForDoubleUnderscoreServer(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\n")
	mustWrite(t, filepath.Join(dir, ".agnostic-ai", "skills", "rep", "SKILL.md"), "---\nname: rep\ndescription: Reports.\nallowed-tools: [mcp:github, mcp:reports__v2]\n---\n\nRun the report.\n")
	mustSync(t)
	got, err := os.ReadFile(filepath.Join(dir, ".claude", "skills", "rep", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "mcp__github") || !strings.Contains(string(got), "mcp__reports__v2__*") {
		t.Errorf("skill file:\n%s", got)
	}
	if strings.Contains(string(got), "mcp__reports__v2\n") {
		t.Errorf("skill still has the bare server rule:\n%s", got)
	}
}
