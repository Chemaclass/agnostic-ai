package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// OpenCode asks before a tool touches a path outside the project, so repo
// mode allows the store under permission.external_directory. The user's
// own rules stay, and the store rule leaves with repo mode.
func TestSync_OpenCodeRepoMemoryAllowsTheStoreAndKeepsUserRules(t *testing.T) {
	parent := repoMemoryProject(t, true)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [opencode]\nbuiltins: [memory]\ngitignore:\n  enabled: true\n")
	writeFile(t, "opencode.json", `{"permission": {"bash": "ask", "external_directory": {"~/notes/**": "allow"}}}`)
	if err := runSync(t); err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Permission struct {
			Bash              string            `json:"bash"`
			ExternalDirectory map[string]string `json:"external_directory"`
		} `json:"permission"`
	}
	read := func() {
		t.Helper()
		doc.Permission.Bash, doc.Permission.ExternalDirectory = "", nil
		if err := json.Unmarshal([]byte(readText(t, "opencode.json")), &doc); err != nil {
			t.Fatal(err)
		}
	}
	read()
	store := repoStore(t, parent, readText(t, "opencode.json"))
	if doc.Permission.ExternalDirectory[store+"/**"] != "allow" || doc.Permission.ExternalDirectory["~/notes/**"] != "allow" || doc.Permission.Bash != "ask" {
		t.Errorf("permission after sync = %+v", doc.Permission)
	}
	if err := runSync(t, "--check"); err != nil {
		t.Errorf("sync --check after sync: %v", err)
	}

	writeFile(t, "agnostic-ai.local.yaml", "memory:\n  personal: checkout\n")
	if err := runSync(t); err != nil {
		t.Fatal(err)
	}
	read()
	if _, kept := doc.Permission.ExternalDirectory[store+"/**"]; kept || doc.Permission.ExternalDirectory["~/notes/**"] != "allow" || doc.Permission.Bash != "ask" {
		t.Errorf("permission after leaving repo mode = %+v", doc.Permission)
	}
	if err := runSync(t, "--check"); err != nil {
		t.Errorf("sync --check after leaving repo mode: %v", err)
	}
}

// A permission map a settings spec produces is sync's whole, so the store
// rule joins it, and a native external_directory action stays the default.
func TestSync_OpenCodeRepoMemoryJoinsSettingsPermissions(t *testing.T) {
	parent := repoMemoryProject(t, true)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [opencode]\nbuiltins: [memory]\ngitignore:\n  enabled: true\n")
	writeFile(t, ".agnostic-ai/settings/policy.yaml", "permissions:\n  allow: [Read]\nx-opencode:\n  permission:\n    external_directory: deny\n")
	if err := runSync(t); err != nil {
		t.Fatal(err)
	}
	text := readText(t, "opencode.json")
	var doc struct {
		Permission struct {
			Read              string            `json:"read"`
			ExternalDirectory map[string]string `json:"external_directory"`
		} `json:"permission"`
	}
	if err := json.Unmarshal([]byte(text), &doc); err != nil {
		t.Fatal(err)
	}
	store := repoStore(t, parent, text)
	if doc.Permission.Read != "allow" || doc.Permission.ExternalDirectory["*"] != "deny" || doc.Permission.ExternalDirectory[store+"/**"] != "allow" {
		t.Errorf("permission = %+v", doc.Permission)
	}
	if err := runSync(t, "--check"); err != nil {
		t.Errorf("sync --check after sync: %v", err)
	}
}

// Devin asks before a write outside the workspace, so repo mode allows
// writes to the store in .devin/config.json. The user's own rules stay,
// and the rule leaves with repo mode.
func TestSync_WindsurfRepoMemoryAllowsWritesAndKeepsUserRules(t *testing.T) {
	parent := repoMemoryProject(t, true)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [windsurf]\nbuiltins: [memory]\ngitignore:\n  enabled: true\n")
	config := filepath.Join(".devin", "config.json")
	writeFile(t, config, `{"permissions": {"allow": ["Exec(make)"]}, "read_config_from": {"claude": true}}`)
	if err := runSync(t); err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Permissions struct {
			Allow []string `json:"allow"`
		} `json:"permissions"`
		ReadConfigFrom map[string]bool `json:"read_config_from"`
	}
	read := func() {
		t.Helper()
		doc.Permissions.Allow, doc.ReadConfigFrom = nil, nil
		if err := json.Unmarshal([]byte(readText(t, config)), &doc); err != nil {
			t.Fatal(err)
		}
	}
	read()
	store := repoStore(t, parent, readText(t, config))
	if want := []string{"Exec(make)", "Write(" + store + "/**)"}; !slices.Equal(doc.Permissions.Allow, want) || !doc.ReadConfigFrom["claude"] {
		t.Errorf("config = %+v, want allow %v", doc, want)
	}
	if err := runSync(t, "--check"); err != nil {
		t.Errorf("sync --check after sync: %v", err)
	}

	writeFile(t, "agnostic-ai.local.yaml", "memory:\n  personal: checkout\n")
	if err := runSync(t); err != nil {
		t.Fatal(err)
	}
	read()
	if want := []string{"Exec(make)"}; !slices.Equal(doc.Permissions.Allow, want) || !doc.ReadConfigFrom["claude"] {
		t.Errorf("config after leaving repo mode = %+v, want allow %v", doc, want)
	}
	if err := runSync(t, "--check"); err != nil {
		t.Errorf("sync --check after leaving repo mode: %v", err)
	}
}

// .devin/config.json takes settings alone, so committed settings keep the
// absolute store path out of it, while another committed kind does not.
func TestSync_WindsurfRepoMemoryRuleFollowsTheCommittedKinds(t *testing.T) {
	for _, tc := range []struct {
		commit string
		want   bool
	}{
		{"windsurf:hooks", true},
		{"windsurf:settings", false},
	} {
		t.Run(tc.commit, func(t *testing.T) {
			parent := repoMemoryProject(t, true)
			writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [windsurf]\nbuiltins: [memory]\ngitignore:\n  enabled: true\n  commit: ["+tc.commit+"]\n")
			if err := runSync(t); err != nil {
				t.Fatal(err)
			}
			text, _ := os.ReadFile(filepath.Join(".devin", "config.json"))
			if got := strings.Contains(string(text), "Write("+filepath.ToSlash(parent)); got != tc.want {
				t.Errorf("Write rule for the store = %v, want %v:\n%s", got, tc.want, text)
			}
			if err := runSync(t, "--check"); err != nil {
				t.Errorf("sync --check after sync: %v", err)
			}
		})
	}
}

// A config file sync created for the store rule alone leaves with it.
func TestSync_StoreRulesLeaveWithRepoMode(t *testing.T) {
	repoMemoryProject(t, true)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [windsurf, opencode]\nbuiltins: [memory]\ngitignore:\n  enabled: true\n")
	if err := runSync(t); err != nil {
		t.Fatal(err)
	}
	writeFile(t, "agnostic-ai.local.yaml", "memory:\n  personal: checkout\n")
	if err := runSync(t); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(".devin", "config.json")); !os.IsNotExist(err) {
		text, _ := os.ReadFile(filepath.Join(".devin", "config.json"))
		t.Errorf(".devin/config.json stayed after repo mode: %s", text)
	}
	if text := readText(t, "opencode.json"); strings.Contains(text, "external_directory") || strings.Contains(text, `"permission"`) {
		t.Errorf("opencode.json kept the store rule after repo mode:\n%s", text)
	}
	if err := runSync(t, "--check"); err != nil {
		t.Errorf("sync --check: %v", err)
	}
}

// Without the managed .gitignore block the files may be committed, so
// neither names the store.
func TestSync_StoreRulesStayOutOfCommittableFiles(t *testing.T) {
	parent := repoMemoryProject(t, false)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [windsurf, opencode]\nbuiltins: [memory]\n")
	if err := runSync(t); err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{"opencode.json", filepath.Join(".devin", "config.json")} {
		if data, err := os.ReadFile(file); err == nil && strings.Contains(string(data), filepath.ToSlash(parent)) {
			t.Errorf("%s names the repo store:\n%s", file, data)
		}
	}
}
