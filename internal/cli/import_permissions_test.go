package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
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
	n, err := importWindsurfPermissions(root, dst, adapters.GrantedRoots{})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if n != 1 {
		t.Fatalf("imported %d specs, want 1", n)
	}
	got := readFileString(t, filepath.Join(dst, "permissions.yaml"))
	for _, want := range []string{
		"read(src/**)",
		// Devin's Exec is a prefix matcher, so the portable spelling
		// has to be the prefix form, not the exact one.
		"shell(git:*)",
		"mcp:slack/post",
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
	if strings.Contains(got, "Bash(git)\n") || strings.Contains(got, "shell(git)\n") {
		t.Errorf("imported Devin's prefix rule as an exact command:\n%s", got)
	}
}

// A project with no Devin config, or no permissions in it, writes
// nothing rather than an empty spec.
func TestImportWindsurfPermissions_QuietWithoutAPolicy(t *testing.T) {
	root := t.TempDir()
	if n, err := importWindsurfPermissions(root, filepath.Join(root, "settings"), adapters.GrantedRoots{}); err != nil || n != 0 {
		t.Fatalf("absent config: got (%d, %v), want (0, nil)", n, err)
	}
	writeFile(t, filepath.Join(root, ".devin", "config.json"), `{"read_config_from": {"claude": true}}`)
	if n, err := importWindsurfPermissions(root, filepath.Join(root, "settings"), adapters.GrantedRoots{}); err != nil || n != 0 {
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
	if !strings.Contains(got, "- read\n") || !strings.Contains(got, "- write\n") {
		t.Errorf("spec missing the mappable rules:\n%s", got)
	}
	// The regex entry and the malformed bare-string entry both name
	// `terminal`, which maps to Bash. Neither may produce a rule.
	if strings.Contains(got, "Bash") || strings.Contains(got, "shell") {
		t.Errorf("imported a rule that cannot round-trip:\n%s", got)
	}
}

// Auggie reads settings.json as JSONC, so a comment or a trailing
// comma must not stop the import.
func TestImportAugmentPermissions_AcceptsJSONC(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".augment", "settings.json"), `{
	  // team policy
	  "defaultModel": "sonnet-4",
	  "toolPermissions": [
	    {"toolName": "read", "permission": {"type": "allow"},},
	  ],
	}`)

	dst := filepath.Join(root, "settings")
	n, err := importAugmentPermissions(root, dst)
	if err != nil || n != 1 {
		t.Fatalf("import: got (%d, %v), want (1, nil)", n, err)
	}
	got := readFileString(t, filepath.Join(dst, "permissions-augment.yaml"))
	if !strings.Contains(got, "- read\n") || !strings.Contains(got, "augment: sonnet-4") {
		t.Errorf("spec missing the commented file's content:\n%s", got)
	}
}

// defaultModel and defaultThinkingLevel come back as the portable
// model and effort, so a re-sync writes the same file.
func TestImportAugment_ReadsDefaultModelAndEffort(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	silence(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [augment]\n")
	native := `{"defaultModel":"sonnet-4","defaultThinkingLevel":"high"}`
	mustWriteFile(t, ".augment/settings.json", native)

	if _, err := runCLI(t, "import", "augment"); err != nil {
		t.Fatalf("import: %v", err)
	}
	got := readFile(t, ".agnostic-ai/settings/permissions-augment.yaml")
	if !strings.Contains(got, "augment: sonnet-4") || !strings.Contains(got, "effort: high") {
		t.Errorf("settings spec = %q, want model and effort", got)
	}
	runSyncOK(t)
	var doc map[string]any
	if err := json.Unmarshal([]byte(readFile(t, ".augment/settings.json")), &doc); err != nil {
		t.Fatal(err)
	}
	if doc["defaultModel"] != "sonnet-4" || doc["defaultThinkingLevel"] != "high" {
		t.Errorf("settings.json after sync = %v", doc)
	}
	if _, err := runCLI(t, "sync", "--check"); err != nil {
		t.Errorf("sync --check: %v", err)
	}
}

// A later settings spec that sets another model would win over the
// imported one, so the native value stays under x-augment.
func TestImportAugment_KeepsAConflictingModelUnderXAugment(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	silence(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [augment]\n")
	mustWriteFile(t, ".agnostic-ai/settings/team.yaml", "model:\n  augment: opus-4\n")
	mustWriteFile(t, ".augment/settings.json", `{"defaultModel":"sonnet-4"}`)

	if _, err := runCLI(t, "import", "augment"); err != nil {
		t.Fatalf("import: %v", err)
	}
	got := readFile(t, ".agnostic-ai/settings/permissions-augment.yaml")
	if !strings.Contains(got, "x-augment:") || !strings.Contains(got, "defaultModel: sonnet-4") || strings.Contains(got, "model:\n") {
		t.Errorf("settings spec = %q, want the model under x-augment", got)
	}
	runSyncOK(t)
	var doc map[string]any
	if err := json.Unmarshal([]byte(readFile(t, ".augment/settings.json")), &doc); err != nil {
		t.Fatal(err)
	}
	if doc["defaultModel"] != "sonnet-4" {
		t.Errorf("defaultModel after sync = %v, want the native sonnet-4", doc["defaultModel"])
	}
}

// A model other specs already resolve to is not written again.
func TestImportAugment_DropsAModelOtherSpecsAlreadySet(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	silence(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [augment]\n")
	mustWriteFile(t, ".agnostic-ai/settings/team.yaml", "model: sonnet-4\n")
	mustWriteFile(t, ".augment/settings.json", `{"defaultModel":"sonnet-4","toolPermissions":[{"toolName":"read","permission":{"type":"allow"}}]}`)

	if _, err := runCLI(t, "import", "augment"); err != nil {
		t.Fatalf("import: %v", err)
	}
	if got := readFile(t, ".agnostic-ai/settings/permissions-augment.yaml"); strings.Contains(got, "sonnet-4") {
		t.Errorf("settings spec = %q, want no repeated model", got)
	}
}

// An ask-user rule comes back as the portable ask list, and sync writes
// the same rule again.
func TestImportAugment_ReadsAskUserRulesBack(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	silence(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [augment]\n")
	mustWriteFile(t, ".augment/settings.json", `{"toolPermissions":[{"toolName":"write","permission":{"type":"ask-user"}},{"toolName":"read","permission":{"type":"allow"}}]}`)

	if _, err := runCLI(t, "import", "augment"); err != nil {
		t.Fatalf("import: %v", err)
	}
	got := readFile(t, ".agnostic-ai/settings/permissions-augment.yaml")
	if !strings.Contains(got, "ask:\n        - write\n") {
		t.Errorf("settings spec = %q, want write in the ask list", got)
	}
	runSyncOK(t)
	var doc struct {
		ToolPermissions []map[string]any `json:"toolPermissions"`
	}
	if err := json.Unmarshal([]byte(readFile(t, ".augment/settings.json")), &doc); err != nil {
		t.Fatal(err)
	}
	first := doc.ToolPermissions[0]
	if first["toolName"] != "write" || first["permission"].(map[string]any)["type"] != "ask-user" {
		t.Errorf("toolPermissions after sync = %v", doc.ToolPermissions)
	}
	if _, err := runCLI(t, "sync", "--check"); err != nil {
		t.Errorf("sync --check: %v", err)
	}
}

// Auggie applies the first rule that matches a tool, so a later rule
// for the same tool is not imported.
func TestImportAugmentPermissions_KeepsTheFirstRuleForATool(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".augment", "settings.json"), `{"toolPermissions":[
	  {"toolName": "write", "permission": {"type": "ask-user"}},
	  {"toolName": "write", "permission": {"type": "allow"}}
	]}`)
	dst := filepath.Join(root, "settings")
	if _, err := importAugmentPermissions(root, dst); err != nil {
		t.Fatalf("import: %v", err)
	}
	got := readFileString(t, filepath.Join(dst, "permissions-augment.yaml"))
	if !strings.Contains(got, "ask:") || strings.Contains(got, "allow:") {
		t.Errorf("spec = %q, want only the first rule", got)
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

// Sync's own Write grant for a store names one machine, so it stays out
// of the spec; a Write rule the user wrote stays in.
func TestImportWindsurfPermissions_DropsTheSyncGrantedStores(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".devin", "config.json"), `{
	  "permissions": {"allow": ["Write(/home/me/store/**)", "Write(/opt/scratch/**)", "Write(/elsewhere/.agnostic-ai/local/handoffs/**)", "Read(src/**)"]}
	}`)

	dst := filepath.Join(root, "settings")
	if _, err := importWindsurfPermissions(root, dst, adapters.GrantedRoots{Dirs: []string{"/home/me/store"}}); err != nil {
		t.Fatalf("import: %v", err)
	}
	got := readFileString(t, filepath.Join(dst, "permissions.yaml"))
	if strings.Contains(got, "/home/me/store") {
		t.Errorf("spec names the sync-granted store:\n%s", got)
	}
	for _, want := range []string{"Write(/opt/scratch/**)", "Write(/elsewhere/.agnostic-ai/local/handoffs/**)", "read(src/**)"} {
		if !strings.Contains(got, want) {
			t.Errorf("spec lost %q:\n%s", want, got)
		}
	}
}
