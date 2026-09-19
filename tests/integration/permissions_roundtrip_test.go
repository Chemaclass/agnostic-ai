package integration

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
	"gopkg.in/yaml.v3"
)

// TestPermissionsRoundTrip_SyncThenImportReadsBackThePolicy binds each
// target's permission emitter to its importer. The two sides are
// separate vocabulary tables with no compile-time link between them
// (augmentTool vs augmentBareTool, devinTool vs devinBareTool,
// kiloPermissionTool vs kiloToolToPortable), so a key renamed on one
// side leaves the other reading a spelling nobody writes. That is
// exactly how `import augment` shipped in v0.61.0 looking for
// `tool-name` where the emitter writes `toolName`: every rule was
// skipped and the unit test passed because its fixture carried the
// same wrong key.
//
// Nothing else guards this. There is no settings spec in the golden
// tree (tests/integration/fixtures/golden/.agnostic-ai/ has no
// settings/ directory), so no target's permission output is snapshotted
// anywhere, and the per-adapter unit tests each build their own input.
//
// Lossy edges are asserted, not tolerated. Where a vendor's vocabulary
// cannot express the portable rule, the documented widening is written
// down as an expectation so that changing it fails loudly.
func TestPermissionsRoundTrip_SyncThenImportReadsBackThePolicy(t *testing.T) {
	cases := []struct {
		target string
		// spec is the portable policy written before the first sync.
		spec map[string][]string
		// want is the policy the import must produce, after the
		// target's documented translation both ways.
		want map[string][]string
		// absent are rules that must not come back under any list,
		// because the vendor has no form for them.
		absent []string
	}{
		{
			// Augment gates read/edit/write/web-fetch as whole tools
			// with no path matcher, so a path-scoped rule is dropped
			// rather than widened onto every file (rulesUnmappableReason
			// in internal/adapters/augment/settings.go). `Bash(...)`
			// leaves as a shellInputRegex rule, which the importer
			// skips on the way back: reversing a regex would invent a
			// boundary the project never wrote.
			target: "augment",
			spec: map[string][]string{
				"allow": {"Read"},
				"deny":  {"Write", "Bash(go test:*)"},
			},
			want: map[string][]string{
				"allow": {"Read"},
				"deny":  {"Write"},
			},
			absent: []string{"Bash(go test:*)", "Bash"},
		},
		{
			// Kilo spells an exact command as its own pattern, and
			// every tool in the policy has a portable name, so this
			// row is a fixed point: what goes in comes back out.
			target: "kilo",
			spec: map[string][]string{
				"allow": {"Read(docs/*)", "Bash(npm run:*)"},
				"deny":  {"Bash(rm -rf /)"},
			},
			want: map[string][]string{
				"allow": {"Read(docs/*)", "Bash(npm run:*)"},
				"deny":  {"Bash(rm -rf /)"},
			},
		},
		{
			// Two documented widenings, asserted so that changing
			// either one fails here. Devin's Exec is a prefix matcher
			// with no exact-command form, so an exact deny comes back
			// as the prefix spelling rather than narrowing a rule the
			// project already applies more widely (devinScopeToPortable
			// in internal/cli/import_permissions.go). And Devin's
			// `write` covers both portable Write and Edit, so Edit
			// reads back as Write, the wider of the two.
			target: "windsurf",
			spec: map[string][]string{
				"allow": {"Read(**)", "Bash(go test:*)"},
				"deny":  {"Bash(rm:*)", "Bash(rm -rf /)"},
				"ask":   {"Write(.env*)", "Edit(src/**)"},
			},
			want: map[string][]string{
				"allow": {"Read(**)", "Bash(go test:*)"},
				"deny":  {"Bash(rm:*)", "Bash(rm -rf /:*)"},
				"ask":   {"Write(.env*)", "Write(src/**)"},
			},
			absent: []string{"Bash(rm -rf /)", "Edit(src/**)"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.target, func(t *testing.T) {
			dir := t.TempDir()
			testutil.Chdir(t, dir)
			seedPermissionsFixture(t, dir, tc.target, tc.spec)

			runCmd(t, "sync", "-t", tc.target)
			must(t, os.RemoveAll(filepath.Join(dir, ".agnostic-ai", "settings")))

			runCmd(t, "import", tc.target)

			got := readPermissionsSpecs(t, filepath.Join(dir, ".agnostic-ai", "settings"))
			for list, want := range tc.want {
				for _, rule := range want {
					if !slices.Contains(got[list], rule) {
						t.Errorf("%s: rule %q missing from imported %s list: %v",
							tc.target, rule, list, got[list])
					}
				}
			}
			for _, rule := range tc.absent {
				for list, rules := range got {
					if slices.Contains(rules, rule) {
						t.Errorf("%s: rule %q has no vendor form but came back under %s: %v",
							tc.target, rule, list, rules)
					}
				}
			}
		})
	}
}

// seedPermissionsFixture writes a config enabling one target and a
// single settings spec holding the portable policy.
func seedPermissionsFixture(t *testing.T, dir, target string, lists map[string][]string) {
	t.Helper()
	must(t, os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"),
		[]byte("version: 1\nsources:\n  settings: .agnostic-ai/settings\ntargets:\n  - "+
			target+"\ngitignore:\n  enabled: false\n"), 0o644))

	must(t, os.MkdirAll(filepath.Join(dir, ".agnostic-ai", "settings"), 0o755))
	body, err := yaml.Marshal(map[string]any{"permissions": lists})
	must(t, err)
	must(t, os.WriteFile(filepath.Join(dir, ".agnostic-ai", "settings", "policy.yaml"), body, 0o644))
}

// readPermissionsSpecs merges the permission lists of every settings
// spec under dir. The importer picks its own filename per target, so
// the test reads whatever landed rather than guessing it.
func readPermissionsSpecs(t *testing.T, dir string) map[string][]string {
	t.Helper()
	out := map[string][]string{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("no settings specs written to %s: %v", dir, err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		must(t, err)
		var doc struct {
			Permissions map[string][]string `yaml:"permissions"`
		}
		must(t, yaml.Unmarshal(data, &doc))
		for list, rules := range doc.Permissions {
			out[list] = append(out[list], rules...)
		}
	}
	return out
}
