package cli

import (
	"path/filepath"
	"strings"
	"testing"
)

// import --scope user keeps a plain setting and marks it !literal, so the
// home it writes lints clean, while a credential name without a
// credential shape stays unmarked for the user to decide.
func TestImportUser_MarksPlainSettingsLiteralAndRoundTrips(t *testing.T) {
	home, source := userAgentTestHome(t)
	cursor := filepath.Join(home, ".cursor", "mcp.json")
	native := `{"mcpServers": {"app": {"command": "app-mcp", "env": {"API_KEY": "sk-live-abc", "NODE_ENV": "production"}}}}` + "\n"
	writeFile(t, cursor, native)

	if _, warnings, err := runImportUserTest("cursor"); err != nil {
		t.Fatalf("import: %v\n%s", err, warnings)
	}
	got := readFile(t, filepath.Join(source, "mcps", "app.yaml"))
	if !strings.Contains(got, "NODE_ENV: !literal production\n") || !strings.Contains(got, "API_KEY: sk-live-abc\n") {
		t.Errorf("spec:\n%s\nwant NODE_ENV marked and API_KEY unmarked", got)
	}
	if _, w, err := runUserAgentTest("--only", "cursor"); err != nil {
		t.Fatalf("sync: %v\n%s", err, w)
	}
	if after := readFile(t, cursor); after != native {
		t.Errorf("cursor file must round-trip unchanged:\n%s", after)
	}
	out, _ := runCLI(t, "lint", "--scope", "user")
	if !strings.Contains(out, `"app": env API_KEY`) || strings.Contains(out, "env NODE_ENV") {
		t.Errorf("lint --scope user must ask only about API_KEY:\n%s", out)
	}
}

// Auggie reads ~/.augment/settings.json as JSONC, so import --scope user
// must accept a comment and a trailing comma there.
func TestImportUser_AugmentAcceptsJSONCSettings(t *testing.T) {
	home, source := userAgentTestHome(t)
	writeFile(t, filepath.Join(home, ".augment", "settings.json"), `{
  // personal servers
  "mcpServers": {"app": {"command": "app-mcp",},},
}
`)

	if _, warnings, err := runImportUserTest("augment"); err != nil {
		t.Fatalf("import: %v\n%s", err, warnings)
	}
	if got := readFile(t, filepath.Join(source, "mcps", "app.yaml")); !strings.Contains(got, "app-mcp") {
		t.Errorf("spec:\n%s\nwant the app server", got)
	}
}
