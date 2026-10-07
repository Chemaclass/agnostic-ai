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

// A local settings layer that repeats a shared deny keeps it after the
// shared catch-all allow, so OpenCode's last match still denies, with
// repo memory on or off.
func TestSync_OpenCodeLocalSettingsLayerKeepsTheSharedOrder(t *testing.T) {
	for _, repo := range []bool{true, false} {
		t.Run(map[bool]string{true: "repo", false: "checkout"}[repo], func(t *testing.T) {
			parent := repoMemoryProject(t, true)
			if !repo {
				writeFile(t, "agnostic-ai.local.yaml", "memory:\n  personal: checkout\n")
			}
			writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [opencode]\nbuiltins: [memory]\ngitignore:\n  enabled: true\n")
			writeFile(t, ".agnostic-ai/settings/policy.yaml", "x-opencode:\n  permission:\n    external_directory:\n      \"*\": allow\n      /secret/**: deny\n")
			writeFile(t, ".agnostic-ai/local/settings/policy.yaml", "x-opencode:\n  permission:\n    external_directory:\n      /secret/**: deny\n")
			if err := runSync(t); err != nil {
				t.Fatal(err)
			}
			want := []string{"*", "/secret/**"}
			if repo {
				want = append(want, repoStore(t, parent, readText(t, "opencode.json"))+"/**")
			}
			if got := externalDirectoryOrder(t); !slices.Equal(got, want) {
				t.Errorf("external_directory order = %v, want %v", got, want)
			}
			if err := runSync(t, "--check"); err != nil {
				t.Errorf("sync --check: %v", err)
			}
		})
	}
}

// A spec's permissions with the store rule beside them stay the same
// across syncs, and leaving repo mode takes out the store rule alone.
func TestSync_SpecPermissionsWithTheStoreRuleStayStable(t *testing.T) {
	for _, tc := range []struct{ target, file, kept string }{
		{"opencode", "opencode.json", `"src/**": "allow"`},
		{"windsurf", filepath.Join(".devin", "config.json"), `"Read(src/**)"`},
	} {
		t.Run(tc.target, func(t *testing.T) {
			parent := repoMemoryProject(t, true)
			writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: ["+tc.target+"]\nbuiltins: [memory]\ngitignore:\n  enabled: true\n")
			writeFile(t, ".agnostic-ai/settings/policy.yaml", "permissions:\n  allow: [Read, Read(src/**)]\n")
			if err := runSync(t); err != nil {
				t.Fatal(err)
			}
			first := readText(t, tc.file)
			if !strings.Contains(first, tc.kept) || !strings.Contains(first, filepath.ToSlash(parent)) {
				t.Errorf("first sync:\n%s", first)
			}
			if err := runSync(t); err != nil {
				t.Fatal(err)
			}
			if second := readText(t, tc.file); second != first {
				t.Errorf("second sync changed the file:\n%s\nwant:\n%s", second, first)
			}
			if err := runSync(t, "--check"); err != nil {
				t.Errorf("sync --check: %v", err)
			}
			writeFile(t, "agnostic-ai.local.yaml", "memory:\n  personal: checkout\n")
			if err := runSync(t); err != nil {
				t.Fatal(err)
			}
			if text := readText(t, tc.file); !strings.Contains(text, tc.kept) || strings.Contains(text, filepath.ToSlash(parent)) {
				t.Errorf("after leaving repo mode:\n%s", text)
			}
			if err := runSync(t, "--check"); err != nil {
				t.Errorf("sync --check after leaving repo mode: %v", err)
			}
		})
	}
}

// Retiring a spec whose map the user only reordered behaves as it does
// without repo memory, plus the store rule: the value sum ignores key
// order, so the map counts as sync's and goes either way.
func TestSync_OpenCodeRetiredSpecAfterAnOrderOnlyEdit(t *testing.T) {
	retire := func(t *testing.T, repo bool) map[string]any {
		t.Helper()
		repoMemoryProject(t, true)
		if !repo {
			writeFile(t, "agnostic-ai.local.yaml", "memory:\n  personal: checkout\n")
		}
		writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [opencode]\nbuiltins: [memory]\ngitignore:\n  enabled: true\n")
		writeFile(t, ".agnostic-ai/settings/policy.yaml", "x-opencode:\n  permission:\n    external_directory:\n      /tmp/a/**: allow\n      \"*\": deny\n")
		if err := runSync(t); err != nil {
			t.Fatal(err)
		}
		before := externalDirectoryOrder(t)
		// Go maps encode sorted, so the round trip moves "*" ahead of
		// /tmp/a/** and changes nothing else.
		editJSON(t, "opencode.json", func(map[string]any) {})
		if after := externalDirectoryOrder(t); slices.Equal(after, before) || after[0] != "*" {
			t.Fatalf("the edit did not reorder: %v -> %v", before, after)
		}
		if err := os.Remove(".agnostic-ai/settings/policy.yaml"); err != nil {
			t.Fatal(err)
		}
		for range 2 {
			if err := runSync(t); err != nil {
				t.Fatal(err)
			}
		}
		if err := runSync(t, "--check"); err != nil {
			t.Errorf("sync --check: %v", err)
		}
		var doc map[string]any
		if err := json.Unmarshal([]byte(readText(t, "opencode.json")), &doc); err != nil {
			t.Fatal(err)
		}
		permission, _ := doc["permission"].(map[string]any)
		return permission
	}
	var without map[string]any
	t.Run("checkout", func(t *testing.T) { without = retire(t, false) })
	t.Run("repo", func(t *testing.T) {
		with := retire(t, true)
		directories, _ := with["external_directory"].(map[string]any)
		if len(directories) != 1 {
			t.Errorf("want the store rule alone, got %v", with)
		}
		delete(with, "external_directory")
		if len(with) != len(without) {
			t.Errorf("repo mode left %v beside the store rule; without it sync left %v", with, without)
		}
		writeFile(t, "agnostic-ai.local.yaml", "memory:\n  personal: checkout\n")
		if err := runSync(t); err != nil {
			t.Fatal(err)
		}
		if text := readText(t, "opencode.json"); strings.Contains(text, `"permission"`) {
			t.Errorf("the store rule stayed after repo mode:\n%s", text)
		}
	})
}

// editJSON decodes the JSON file at path, lets edit change it, and
// writes it back.
func editJSON(t *testing.T, path string, edit func(doc map[string]any)) {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal([]byte(readText(t, path)), &doc); err != nil {
		t.Fatal(err)
	}
	edit(doc)
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, path, string(data)+"\n")
}

// A value sync wrote whole from a settings spec and the user then edited
// stays theirs when the spec goes: sync keeps the edit, claims only the
// store rule, and takes only that out when repo mode ends.
func TestSync_RetiredSettingsKeepAUserEditBesideTheStoreRule(t *testing.T) {
	for _, tc := range []struct {
		target, file string
		edit         func(doc map[string]any)
		kept         []string
	}{
		{"opencode", "opencode.json", func(doc map[string]any) {
			doc["permission"].(map[string]any)["bash"] = "deny"
		}, []string{`"bash": "deny"`, `"src/**": "allow"`}},
		{"windsurf", filepath.Join(".devin", "config.json"), func(doc map[string]any) {
			permissions := doc["permissions"].(map[string]any)
			permissions["allow"] = append(permissions["allow"].([]any), "Exec(make)")
		}, []string{`"Exec(make)"`, `"Read(src/**)"`}},
	} {
		t.Run(tc.target, func(t *testing.T) {
			parent := repoMemoryProject(t, true)
			writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: ["+tc.target+"]\nbuiltins: [memory]\ngitignore:\n  enabled: true\n")
			writeFile(t, ".agnostic-ai/settings/policy.yaml", "permissions:\n  allow: [Read, Read(src/**)]\n")
			if err := runSync(t); err != nil {
				t.Fatal(err)
			}
			editJSON(t, tc.file, tc.edit)
			if err := os.Remove(".agnostic-ai/settings/policy.yaml"); err != nil {
				t.Fatal(err)
			}
			check := func(when string, wantStore bool) {
				t.Helper()
				text := readText(t, tc.file)
				for _, want := range tc.kept {
					if !strings.Contains(text, want) {
						t.Errorf("%s: lost the user's %s:\n%s", when, want, text)
					}
				}
				if got := strings.Contains(text, filepath.ToSlash(parent)); got != wantStore {
					t.Errorf("%s: store rule present = %v, want %v:\n%s", when, got, wantStore, text)
				}
			}
			for _, when := range []string{"after removing the spec", "on the next sync"} {
				if err := runSync(t); err != nil {
					t.Fatal(err)
				}
				check(when, true)
			}
			if err := runSync(t, "--check"); err != nil {
				t.Errorf("sync --check: %v", err)
			}

			writeFile(t, "agnostic-ai.local.yaml", "memory:\n  personal: checkout\n")
			if err := runSync(t); err != nil {
				t.Fatal(err)
			}
			check("after leaving repo mode", false)
			if err := runSync(t, "--check"); err != nil {
				t.Errorf("sync --check after leaving repo mode: %v", err)
			}
		})
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

// A bare `permission` action sets every permission in OpenCode. Repo mode
// keeps it as the catch-all key ahead of the store rule, and leaving repo
// mode keeps the user's policy.
func TestSync_OpenCodeRepoMemoryKeepsABarePermissionAction(t *testing.T) {
	for _, action := range []string{"deny", "ask"} {
		t.Run(action, func(t *testing.T) {
			parent := repoMemoryProject(t, true)
			writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [opencode]\nbuiltins: [memory]\ngitignore:\n  enabled: true\n")
			writeFile(t, "opencode.json", `{"permission": "`+action+`"}`)
			read := func() map[string]any {
				t.Helper()
				var doc struct {
					Permission map[string]any `json:"permission"`
				}
				if err := json.Unmarshal([]byte(readText(t, "opencode.json")), &doc); err != nil {
					t.Fatalf("permission is not an object: %v\n%s", err, readText(t, "opencode.json"))
				}
				return doc.Permission
			}
			for range 2 {
				if err := runSync(t); err != nil {
					t.Fatal(err)
				}
				store := repoStore(t, parent, readText(t, "opencode.json"))
				permission := read()
				directories, _ := permission["external_directory"].(map[string]any)
				if permission["*"] != action || directories[store+"/**"] != "allow" || len(permission) != 2 {
					t.Errorf("permission = %v", permission)
				}
				if text := readText(t, "opencode.json"); strings.Index(text, `"*"`) > strings.Index(text, `"external_directory"`) {
					t.Errorf("the catch-all should lead the store rule:\n%s", text)
				}
			}
			if err := runSync(t, "--check"); err != nil {
				t.Errorf("sync --check: %v", err)
			}

			writeFile(t, "agnostic-ai.local.yaml", "memory:\n  personal: checkout\n")
			if err := runSync(t); err != nil {
				t.Fatal(err)
			}
			if permission := read(); permission["*"] != action || len(permission) != 1 {
				t.Errorf("permission after leaving repo mode = %v, want the %s catch-all alone", permission, action)
			}
			if err := runSync(t, "--check"); err != nil {
				t.Errorf("sync --check after leaving repo mode: %v", err)
			}
		})
	}
}

// A rule for the store the user wrote before repo mode stays theirs:
// sync neither adds a copy nor takes it out when repo mode ends.
func TestSync_RepoMemoryKeepsTheUserStoreRule(t *testing.T) {
	for _, tc := range []struct {
		target, file, body, entry string
	}{
		{"windsurf", filepath.Join(".devin", "config.json"), `{"permissions": {"allow": ["%s"]}}`, "Write(%s/**)"},
		{"cursor", filepath.Join(".cursor", "cli.json"), `{"permissions": {"allow": ["%s"], "deny": []}}`, "Write(%s/**)"},
		{"qoder", filepath.Join(".qoder", "settings.json"), `{"permissions": {"additionalDirectories": ["%s"]}}`, "%s"},
	} {
		t.Run(tc.target, func(t *testing.T) {
			repoMemoryProject(t, true)
			writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: ["+tc.target+"]\nbuiltins: [memory]\ngitignore:\n  enabled: true\n")
			store := filepath.ToSlash(memoryPaths(t)["personal"])
			entry := strings.Replace(tc.entry, "%s", store, 1)
			writeFile(t, tc.file, strings.Replace(tc.body, "%s", entry, 1))
			for range 2 {
				if err := runSync(t); err != nil {
					t.Fatal(err)
				}
				if text := readText(t, tc.file); strings.Count(text, `"`+entry+`"`) != 1 {
					t.Errorf("want the user's rule once:\n%s", text)
				}
			}
			if err := runSync(t, "--check"); err != nil {
				t.Errorf("sync --check: %v", err)
			}

			writeFile(t, "agnostic-ai.local.yaml", "memory:\n  personal: checkout\n")
			if err := runSync(t); err != nil {
				t.Fatal(err)
			}
			if data, err := os.ReadFile(tc.file); err != nil || strings.Count(string(data), `"`+entry+`"`) != 1 {
				t.Errorf("the user's rule left with repo mode (%v):\n%s", err, data)
			}
			if err := runSync(t, "--check"); err != nil {
				t.Errorf("sync --check after leaving repo mode: %v", err)
			}
		})
	}
}
