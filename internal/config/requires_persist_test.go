package config

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

const requiresTestSchema = "https://raw.githubusercontent.com/Chemaclass/agnostic-ai/v0.77.0/docs/schemas/config.schema.json"

func writeRequiresConfig(t *testing.T, root, name, content string) string {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.WriteFile(path, []byte(content), 0o640); err != nil {
		t.Fatal(err)
	}
	return path
}

func readRequiresConfig(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestPersistRequires_PreservesCommentsAndUnrelatedConfig(t *testing.T) {
	root := testutil.TempCwd(t)
	before := "# yaml-language-server: $schema=https://example.com/old.json custom=true\r\n# Project note\r\nversion: 1\r\nrequires: \"0.76.0\"  # Keep this note\r\ntargets: [claude]\r\nsources:\r\n  rules: shared/rules"
	path := writeRequiresConfig(t, root, ConfigFileName, before)
	changed, err := PersistRequires(root, "0.77.0", requiresTestSchema)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.ReplaceAll(strings.Replace(before, "https://example.com/old.json", requiresTestSchema, 1), "\"0.76.0\"", "\"0.77.0\"")
	if got := readRequiresConfig(t, path); got != want {
		t.Errorf("unrelated bytes changed:\n got %q\nwant %q", got, want)
	}
	if !reflect.DeepEqual(changed, []string{path}) {
		t.Errorf("changed = %v, want %v", changed, []string{path})
	}
	changed, err = PersistRequires(root, "0.77.0", requiresTestSchema)
	if err != nil || len(changed) != 0 {
		t.Errorf("second reconciliation = %v, %v; want no changes", changed, err)
	}
}

func TestPersistRequires_AddsMissingPinsToLegacyConfig(t *testing.T) {
	root := testutil.TempCwd(t)
	before := "# Project note\nversion: 1\ntargets: [claude]\n"
	path := writeRequiresConfig(t, root, LegacyConfigFileName, before)
	changed, err := PersistRequires(root, "0.77.0", requiresTestSchema)
	if err != nil {
		t.Fatal(err)
	}
	got := readRequiresConfig(t, path)
	if !strings.HasPrefix(got, "# yaml-language-server: $schema="+requiresTestSchema+"\n") || !strings.Contains(got, "requires: \"0.77.0\"\n") {
		t.Errorf("missing release pins:\n%s", got)
	}
	if strings.Replace(strings.TrimPrefix(got, "# yaml-language-server: $schema="+requiresTestSchema+"\n"), "requires: \"0.77.0\"\n", "", 1) != before {
		t.Errorf("unrelated legacy configuration changed:\n%s", got)
	}
	if !reflect.DeepEqual(changed, []string{path}) {
		t.Errorf("changed = %v, want legacy path", changed)
	}
	if _, err := os.Stat(filepath.Join(root, ConfigFileName)); !os.IsNotExist(err) {
		t.Errorf("primary config unexpectedly created: %v", err)
	}
}

func TestPersistRequires_UpdatesExistingLocalOverrides(t *testing.T) {
	for _, scalar := range []string{"'0.75.0'", "null", ""} {
		t.Run("scalar="+scalar, func(t *testing.T) {
			root := testutil.TempCwd(t)
			writeRequiresConfig(t, root, ConfigFileName, "version: 1\nrequires: \"0.76.0\"\ntargets: [claude]\n")
			before := "# yaml-language-server: $schema=https://example.com/local.json\nrequires: " + scalar + " # Local note\nsources:\n  rules: personal/rules\n"
			local := writeRequiresConfig(t, root, LocalOverrideFileName, before)
			changed, err := PersistRequires(root, "0.77.0", requiresTestSchema)
			if err != nil {
				t.Fatal(err)
			}
			cfg, err := Load(root)
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Requires != "0.77.0" {
				t.Errorf("effective requires = %q", cfg.Requires)
			}
			got := readRequiresConfig(t, local)
			if !strings.Contains(got, "$schema="+requiresTestSchema) || !strings.Contains(got, " # Local note\nsources:\n  rules: personal/rules\n") {
				t.Errorf("local schema or unrelated bytes lost:\n%s", got)
			}
			if len(changed) != 2 || changed[1] != local {
				t.Errorf("changed = %v, want base and local", changed)
			}
		})
	}
}

func TestPersistRequires_DoesNotAddLocalOverride(t *testing.T) {
	root := testutil.TempCwd(t)
	writeRequiresConfig(t, root, ConfigFileName, "version: 1\ntargets: [claude]\n")
	before := "# Local settings\nsources:\n  rules: personal/rules\n"
	local := writeRequiresConfig(t, root, LocalOverrideFileName, before)
	changed, err := PersistRequires(root, "0.77.0", requiresTestSchema)
	if err != nil {
		t.Fatal(err)
	}
	if got := readRequiresConfig(t, local); got != before {
		t.Errorf("local file changed without a requires override:\n%s", got)
	}
	if len(changed) != 1 {
		t.Errorf("changed = %v, want only base", changed)
	}
}

func TestPersistRequires_UpdatesLocalSchemaWithoutAddingRequires(t *testing.T) {
	root := testutil.TempCwd(t)
	writeRequiresConfig(t, root, ConfigFileName, "version: 1\nrequires: '0.76.0'\ntargets: [claude]\n")
	before := "# yaml-language-server: $schema=https://example.com/old.json\n# Local settings\nsources:\n  rules: personal/rules\n"
	local := writeRequiresConfig(t, root, LocalOverrideFileName, before)
	if _, err := PersistRequires(root, "0.77.0", requiresTestSchema); err != nil {
		t.Fatal(err)
	}
	if got, want := readRequiresConfig(t, local), strings.Replace(before, "https://example.com/old.json", requiresTestSchema, 1); got != want {
		t.Errorf("local schema did not align without adding an override:\n got %q\nwant %q", got, want)
	}
}

func TestPersistRequires_DoesNotAddLocalSchemaDirective(t *testing.T) {
	root := testutil.TempCwd(t)
	writeRequiresConfig(t, root, ConfigFileName, "version: 1\nrequires: '0.76.0'\n")
	local := writeRequiresConfig(t, root, LocalOverrideFileName, "# Local note\nrequires: '0.75.0' # Keep\n")
	if _, err := PersistRequires(root, "0.77.0", requiresTestSchema); err != nil {
		t.Fatal(err)
	}
	if got, want := readRequiresConfig(t, local), "# Local note\nrequires: '0.77.0' # Keep\n"; got != want {
		t.Errorf("local config = %q, want %q", got, want)
	}
}

func TestPersistRequires_RejectsUneditableLocalBeforeWritingBase(t *testing.T) {
	for _, localText := range []string{
		"requires: [broken\n",
		"{requires: '0.75.0'}\n",
		"requires: &release '0.75.0'\nsources:\n  rules: *release\n",
	} {
		t.Run(localText, func(t *testing.T) {
			root := testutil.TempCwd(t)
			baseText := "version: 1\nrequires: '0.76.0'\ntargets: [claude]\n"
			base := writeRequiresConfig(t, root, ConfigFileName, baseText)
			local := writeRequiresConfig(t, root, LocalOverrideFileName, localText)
			if _, err := PersistRequires(root, "0.77.0", requiresTestSchema); err == nil || !strings.Contains(err.Error(), local) {
				t.Errorf("error = %v, want local path", err)
			}
			if got := readRequiresConfig(t, base); got != baseText {
				t.Errorf("base partially updated:\n%s", got)
			}
			if got := readRequiresConfig(t, local); got != localText {
				t.Errorf("rejected local file changed:\n%s", got)
			}
		})
	}
}

func TestPersistRequires_PreservesFilePermissions(t *testing.T) {
	root := testutil.TempCwd(t)
	path := writeRequiresConfig(t, root, ConfigFileName, "version: 1\nrequires: 0.76.0\n")
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := PersistRequires(root, "0.77.0", requiresTestSchema); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if after.Mode().Perm() != before.Mode().Perm() {
		t.Errorf("permissions changed from %v to %v", before.Mode().Perm(), after.Mode().Perm())
	}
}

func TestPersistRequires_WritesThroughConfigSymlink(t *testing.T) {
	root := testutil.TempCwd(t)
	target := writeRequiresConfig(t, root, "shared.yaml", "version: 1\nrequires: '0.76.0'\n")
	alias := filepath.Join(root, ConfigFileName)
	if err := os.Symlink("shared.yaml", alias); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("file symlink privileges unavailable: %v", err)
		}
		t.Fatal(err)
	}
	changed, err := PersistRequires(root, "0.77.0", requiresTestSchema)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(alias)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Error("config symlink was replaced")
	}
	if got := readRequiresConfig(t, target); !strings.Contains(got, "requires: '0.77.0'") {
		t.Errorf("symlink target was not reconciled:\n%s", got)
	}
	if !reflect.DeepEqual(changed, []string{alias}) {
		t.Errorf("changed = %v, want lexical config path", changed)
	}
}

func TestPersistRequires_RestoresBaseWhenLocalWriteFails(t *testing.T) {
	root := testutil.TempCwd(t)
	baseText := "version: 1\nrequires: '0.76.0'\n"
	localText := "requires: '0.75.0'\n"
	base := writeRequiresConfig(t, root, ConfigFileName, baseText)
	local := writeRequiresConfig(t, root, LocalOverrideFileName, localText)
	write := func(path string, data []byte, mode fs.FileMode) error {
		if filepath.Base(path) == LocalOverrideFileName {
			return writeRequiresFile(filepath.Join(root, "missing", LocalOverrideFileName), data, mode)
		}
		return writeRequiresFile(path, data, mode)
	}
	if _, err := persistRequiresWith(root, "0.77.0", requiresTestSchema, write); err == nil || !strings.Contains(err.Error(), LocalOverrideFileName) {
		t.Errorf("error = %v, want local write failure", err)
	}
	if got := readRequiresConfig(t, base); got != baseText {
		t.Errorf("base not restored after failure:\n%s", got)
	}
	if got := readRequiresConfig(t, local); got != localText {
		t.Errorf("local changed after failure:\n%s", got)
	}
}
