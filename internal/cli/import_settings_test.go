package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImportPortableSettings_TargetShapes(t *testing.T) {
	tests := []struct {
		name          string
		nativePath    string
		body          string
		nestedModel   bool
		permissions   bool
		wantFragments []string
	}{
		{name: "opencode", nativePath: "opencode.json", body: `{"model":"open/model","theme":"dark"}`, wantFragments: []string{"model: open/model"}},
		{name: "junie", nativePath: ".junie/config.json", body: `{"model":"junie-model","effort":"high"}`, wantFragments: []string{"model: junie-model"}},
		{name: "kilo jsonc", nativePath: "kilo.jsonc", body: "{\n // project default\n \"model\": \"kilo/model\"\n}", wantFragments: []string{"model: kilo/model"}},
		{
			name:        "qoder",
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
			count, err := importPortableSettings(dir, tt.nativePath, filepath.Join(dir, "settings"), tt.nestedModel, tt.permissions)
			if err != nil {
				t.Fatal(err)
			}
			if count != 1 {
				t.Fatalf("count = %d, want 1", count)
			}
			got, err := os.ReadFile(filepath.Join(dir, "settings", "imported.yaml"))
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
	count, err := importPortableSettings(dir, "config.json", filepath.Join(dir, "settings"), false, false)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("count = %d, want 0", count)
	}
}
