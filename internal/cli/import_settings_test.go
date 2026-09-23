package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestImportPortableSettings_TargetShapes(t *testing.T) {
	tests := []struct {
		name          string
		target        string
		nativePath    string
		body          string
		nestedModel   bool
		permissions   bool
		wantFragments []string
	}{
		{name: "opencode", target: "opencode", nativePath: "opencode.json", body: `{"model":"open/model","theme":"dark"}`, wantFragments: []string{"model: open/model"}},
		{name: "junie", target: "junie", nativePath: ".junie/config.json", body: `{"model":"junie-model","effort":"high"}`, wantFragments: []string{"model: junie-model"}},
		{name: "kilo jsonc", target: "kilo", nativePath: "kilo.jsonc", body: "{\n // project default\n \"model\": \"kilo/model\"\n}", wantFragments: []string{"model: kilo/model"}},
		{
			name:        "qoder",
			target:      "qoder",
			nativePath:  ".qoder/settings.json",
			body:        `{"model":{"name":"qoder-model"},"permissions":{"allow":["Read(**)"],"deny":["Bash(rm:*)"],"nativeOnly":true}}`,
			nestedModel: true,
			permissions: true,
			wantFragments: []string{
				"model: qoder-model", "permissions:", "allow:", "- Read(**)", "deny:", "- Bash(rm:*)",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, filepath.Join(dir, tt.nativePath), tt.body)
			count, err := importPortableSettings(dir, tt.nativePath, filepath.Join(dir, "settings"), portableSettingsShape{target: tt.target, nestedModel: tt.nestedModel, permissions: tt.permissions})
			if err != nil {
				t.Fatal(err)
			}
			if count != 1 {
				t.Fatalf("count = %d, want 1", count)
			}
			got, err := os.ReadFile(filepath.Join(dir, "settings", tt.target+".yaml"))
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range tt.wantFragments {
				if !strings.Contains(string(got), want) {
					t.Errorf("settings missing %q:\n%s", want, got)
				}
			}
			if strings.Contains(string(got), "theme") || strings.Contains(string(got), "effort") || strings.Contains(string(got), "nativeOnly") {
				t.Errorf("native-only keys leaked into portable settings:\n%s", got)
			}
		})
	}
}

func TestImportPortableSettings_NoPortableFields(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "config.json"), `{"theme":"dark"}`)
	count, err := importPortableSettings(dir, "config.json", filepath.Join(dir, "settings"), portableSettingsShape{target: "junie"})
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("count = %d, want 0", count)
	}
}

func TestImportAll_KeepsEachToolsSettings(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [copilot, gemini]\n")
	writeFile(t, filepath.Join(dir, ".github", "copilot-instructions.md"), "# Copilot\n")
	writeFile(t, filepath.Join(dir, "GEMINI.md"), "# Gemini\n")
	writeFile(t, filepath.Join(dir, ".github", "copilot", "settings.json"), `{"model":"copilot-model"}`)
	writeFile(t, filepath.Join(dir, ".gemini", "settings.json"), `{"model":{"name":"gemini-model"}}`)

	execCLI(t, "import", "all")

	for file, want := range map[string]string{"copilot.yaml": "model: copilot-model", "gemini.yaml": "model: gemini-model"} {
		got := readFile(t, filepath.Join(dir, ".agnostic-ai", "settings", file))
		if !strings.Contains(got, want) {
			t.Errorf("settings/%s must keep %q:\n%s", file, want, got)
		}
	}
}
