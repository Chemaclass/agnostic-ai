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

// externalDirectoryOrder returns the patterns of permission.external_directory
// in opencode.json in file order.
func externalDirectoryOrder(t *testing.T) []string {
	t.Helper()
	var doc struct {
		Permission struct {
			ExternalDirectory json.RawMessage `json:"external_directory"`
		} `json:"permission"`
	}
	if err := json.Unmarshal([]byte(readText(t, "opencode.json")), &doc); err != nil {
		t.Fatal(err)
	}
	dec := json.NewDecoder(strings.NewReader(string(doc.Permission.ExternalDirectory)))
	if _, err := dec.Token(); err != nil {
		t.Fatal(err)
	}
	var keys []string
	for dec.More() {
		key, err := dec.Token()
		if err != nil {
			t.Fatal(err)
		}
		keys = append(keys, key.(string))
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			t.Fatal(err)
		}
	}
	return keys
}

// OpenCode applies the last matching rule, so the user's entries keep
// their order and the store entry comes last, through every sync.
func TestSync_OpenCodeRepoMemoryKeepsTheUserOrder(t *testing.T) {
	for _, user := range [][]string{{"*", "/tmp/a/**"}, {"/tmp/a/**", "*"}} {
		t.Run(strings.Join(user, ","), func(t *testing.T) {
			parent := repoMemoryProject(t, true)
			writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [opencode]\nbuiltins: [memory]\ngitignore:\n  enabled: true\n")
			action := map[string]string{"*": "deny", "/tmp/a/**": "allow"}
			writeFile(t, "opencode.json", `{"permission": {"external_directory": {"`+user[0]+`": "`+action[user[0]]+`", "`+user[1]+`": "`+action[user[1]]+`"}}}`)
			for range 2 {
				if err := runSync(t); err != nil {
					t.Fatal(err)
				}
				store := repoStore(t, parent, readText(t, "opencode.json"))
				if got, want := externalDirectoryOrder(t), append(slices.Clone(user), store+"/**"); !slices.Equal(got, want) {
					t.Errorf("external_directory order = %v, want %v", got, want)
				}
			}
			if err := runSync(t, "--check"); err != nil {
				t.Errorf("sync --check: %v", err)
			}
		})
	}
}

// A spec that owns the permission map keeps its own external_directory
// order, with the store entry last.
func TestSync_OpenCodeRepoMemoryKeepsTheSpecOrder(t *testing.T) {
	parent := repoMemoryProject(t, true)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [opencode]\nbuiltins: [memory]\ngitignore:\n  enabled: true\n")
	writeFile(t, ".agnostic-ai/settings/policy.yaml", "x-opencode:\n  permission:\n    external_directory:\n      /tmp/a/**: allow\n      \"*\": deny\n")
	if err := runSync(t); err != nil {
		t.Fatal(err)
	}
	store := repoStore(t, parent, readText(t, "opencode.json"))
	if got, want := externalDirectoryOrder(t), []string{"/tmp/a/**", "*", store + "/**"}; !slices.Equal(got, want) {
		t.Errorf("external_directory order = %v, want %v", got, want)
	}
	if err := runSync(t, "--check"); err != nil {
		t.Errorf("sync --check: %v", err)
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
