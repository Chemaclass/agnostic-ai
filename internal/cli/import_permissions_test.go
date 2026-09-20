package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Devin's committed project policy comes back as a Settings spec, with
// every rule in the vocabulary agnostic-ai emits translated back.
func TestImportWindsurfPermissions_ReversesDevinsVocabulary(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".devin", "config.json"), `{
	  "read_config_from": {"claude": true},
	  "permissions": {
	    "allow": ["Read(src/**)", "Exec(git)", "mcp__slack__post"],
	    "deny": ["Write(secrets/**)", "web_search"],
	    "ask": ["Fetch(https://example.com)"]
	  }
	}`)

	dst := filepath.Join(root, "settings")
	n, err := importWindsurfPermissions(root, dst)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if n != 1 {
		t.Fatalf("imported %d specs, want 1", n)
	}
	got := readFileString(t, filepath.Join(dst, "permissions.yaml"))
	for _, want := range []string{
		"Read(src/**)",
		// Devin's Exec is a prefix matcher, so the portable spelling
		// has to be the prefix form, not the exact one.
		"Bash(git:*)",
		"mcp__slack__post",
		"Write(secrets/**)",
		"WebFetch(https://example.com)",
		// Devin takes `web_search` in all three lists since
		// v3000.10.21, so a hand-written rule has to survive the
		// round trip (#951).
		"WebSearch",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("spec missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Bash(git)\n") {
		t.Errorf("imported Devin's prefix rule as an exact command:\n%s", got)
	}
}

// A project with no Devin config, or no permissions in it, writes
// nothing rather than an empty spec.
func TestImportWindsurfPermissions_QuietWithoutAPolicy(t *testing.T) {
	root := t.TempDir()
	if n, err := importWindsurfPermissions(root, filepath.Join(root, "settings")); err != nil || n != 0 {
		t.Fatalf("absent config: got (%d, %v), want (0, nil)", n, err)
	}
	writeFile(t, filepath.Join(root, ".devin", "config.json"), `{"read_config_from": {"claude": true}}`)
	if n, err := importWindsurfPermissions(root, filepath.Join(root, "settings")); err != nil || n != 0 {
		t.Fatalf("config without permissions: got (%d, %v), want (0, nil)", n, err)
	}
}

// Augment's object-form permissions map back onto the portable tool
// names; a regex rule has no portable spelling and is left alone
// rather than guessed at.
func TestImportAugmentPermissions_SkipsWhatCannotRoundTrip(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".augment", "settings.json"), `{
	  "toolPermissions": [
	    {"toolName": "terminal", "permission": {"type": "deny"}, "shellInputRegex": "^git push"},
	    {"toolName": "read", "permission": {"type": "allow"}},
	    {"toolName": "write", "permission": {"type": "deny"}},
	    {"toolName": "terminal", "permission": "deny"}
	  ]
	}`)

	dst := filepath.Join(root, "settings")
	if _, err := importAugmentPermissions(root, dst); err != nil {
		t.Fatalf("import: %v", err)
	}
	got := readFileString(t, filepath.Join(dst, "permissions-augment.yaml"))
	if !strings.Contains(got, "Read") || !strings.Contains(got, "Write") {
		t.Errorf("spec missing the mappable rules:\n%s", got)
	}
	// The regex entry and the malformed bare-string entry both name
	// `terminal`, which maps to Bash. Neither may produce a rule.
	if strings.Contains(got, "Bash") {
		t.Errorf("imported a rule that cannot round-trip:\n%s", got)
	}
}

func readFileString(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
