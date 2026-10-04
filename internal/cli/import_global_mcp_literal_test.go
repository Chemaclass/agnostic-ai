package cli

import (
	"path/filepath"
	"strings"
	"testing"
)

// import --global keeps a plain setting and marks it !literal, so the
// home it writes lints clean, while a credential name without a
// credential shape stays unmarked for the user to decide.
func TestImportGlobal_MarksPlainSettingsLiteralAndRoundTrips(t *testing.T) {
	home, source := globalAgentTestHome(t)
	cursor := filepath.Join(home, ".cursor", "mcp.json")
	native := `{"mcpServers": {"app": {"command": "app-mcp", "env": {"API_KEY": "sk-live-abc", "NODE_ENV": "production"}}}}` + "\n"
	mustWriteGlobalTest(t, cursor, native)

	if _, warnings, err := runImportGlobalTest("cursor"); err != nil {
		t.Fatalf("import: %v\n%s", err, warnings)
	}
	got := readGlobalTest(t, filepath.Join(source, "mcps", "app.yaml"))
	if !strings.Contains(got, "NODE_ENV: !literal production\n") || !strings.Contains(got, "API_KEY: sk-live-abc\n") {
		t.Errorf("spec:\n%s\nwant NODE_ENV marked and API_KEY unmarked", got)
	}
	if _, w, err := runGlobalAgentTest("--only", "cursor"); err != nil {
		t.Fatalf("sync: %v\n%s", err, w)
	}
	if after := readGlobalTest(t, cursor); after != native {
		t.Errorf("cursor file must round-trip unchanged:\n%s", after)
	}
	out, _ := runCLI(t, "lint", "--global")
	if !strings.Contains(out, `"app": env API_KEY`) || strings.Contains(out, "env NODE_ENV") {
		t.Errorf("lint --global must ask only about API_KEY:\n%s", out)
	}
}
