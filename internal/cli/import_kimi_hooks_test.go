package cli

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/kimi"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func kimiCredentialHookCommands(t *testing.T) map[string]string {
	t.Helper()
	command := "curl --api-key fixtureKimiHook12345678 https://example.com"
	commands := map[string]string{"plain": command}
	for _, platform := range []string{"linux", "windows"} {
		records, err := kimi.New().UserHookRecordsForPlatform([]spec.Entry{{Kind: spec.KindHook, Name: "guard", Meta: map[string]any{"event": "Stop", "command": command}}}, ".kimi-code/hooks", platform)
		if err != nil {
			t.Fatal(err)
		}
		commands[platform] = records[0]["command"].(string)
	}
	return commands
}

func TestImportKimi_RejectsHookCredentialsBeforeWrites(t *testing.T) {
	for name, command := range kimiCredentialHookCommands(t) {
		t.Run(name, func(t *testing.T) {
			dir := testutil.TempCwd(t)
			silence(t)
			writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [kimi]\noutputs:\n  kimi:\n    conf-file: profile/config.toml\n")
			before := fmt.Sprintf("[[hooks]]\nevent='Stop'\ncommand='safe-hook'\n[[hooks]]\nevent='Stop'\ncommand=%q\n", command)
			writeFile(t, "profile/config.toml", before)
			out, err := runCLI(t, "import", "kimi")
			assertKimiHookCredentialRefused(t, filepath.Join(dir, ".agnostic-ai"), out, "profile/config.toml", err)
			if got := readFile(t, "profile/config.toml"); got != before {
				t.Error("native profile changed")
			}
		})
	}
}

func TestImportGlobal_KimiRejectsHookCredentialsBeforeWrites(t *testing.T) {
	for name, command := range kimiCredentialHookCommands(t) {
		t.Run(name, func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			root := filepath.Join(home, "kimi-current")
			t.Setenv("KIMI_CODE_HOME", root)
			path := filepath.Join(root, "config.toml")
			before := fmt.Sprintf("[[hooks]]\nevent='Stop'\ncommand='safe-hook'\n[[hooks]]\nevent='Stop'\ncommand=%q\n", command)
			mustWriteGlobalTest(t, path, before)
			out, warnings, err := runImportGlobalTest("kimi")
			assertKimiHookCredentialRefused(t, source, out+warnings, path, err)
			if got := readGlobalTest(t, path); got != before {
				t.Error("native profile changed")
			}
		})
	}
}

func assertKimiHookCredentialRefused(t *testing.T, source, output, path string, err error) {
	t.Helper()
	if err == nil {
		t.Error("credential-bearing hook imported")
	} else {
		output += err.Error()
		if !strings.Contains(err.Error(), path) {
			t.Error("credential error does not identify native path")
		}
	}
	if strings.Contains(output, "fixtureKimiHook12345678") || strings.Contains(output, "curl --api-key") || strings.Contains(output, "EncodedCommand") {
		t.Error("credential command appeared in diagnostics")
	}
	walkErr := filepath.WalkDir(source, func(path string, d fs.DirEntry, walkErr error) error {
		if os.IsNotExist(walkErr) {
			return nil
		}
		if walkErr != nil {
			return walkErr
		}
		if !d.IsDir() && !strings.HasPrefix(d.Name(), ".") {
			t.Errorf("failed import wrote source file %s", path)
		}
		return nil
	})
	if walkErr != nil {
		t.Fatal(walkErr)
	}
}

func TestImportKimi_HookCredentialReferencesStayReferences(t *testing.T) {
	dir := testutil.TempCwd(t)
	command := "curl --api-key ${API_KEY} https://example.com"
	records, err := kimi.New().UserHookRecordsForPlatform([]spec.Entry{{Kind: spec.KindHook, Name: "guard", Meta: map[string]any{"event": "Stop", "command": command}}}, ".kimi-code/hooks", "windows")
	if err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, "hooks")
	if err := importKimiHooks("profile/config.toml", records, dst); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dst)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d hook specs", len(entries))
	}
	got := readFile(t, filepath.Join(dst, entries[0].Name()))
	if !strings.Contains(got, command) || strings.Contains(got, "EncodedCommand") {
		t.Errorf("declared hook reference changed: %s", got)
	}
}

func TestImportKimi_ValidatesAllHooksBeforeCreatingSources(t *testing.T) {
	dir := testutil.TempCwd(t)
	dst := filepath.Join(dir, "hooks")
	records := []map[string]any{
		{"event": "Stop", "command": "safe-hook"},
		{"event": "Stop", "command": "curl --api-key fixtureKimiHook12345678 https://example.com"},
	}
	if err := importKimiHooks("profile/config.toml", records, dst); err == nil {
		t.Error("credential-bearing hook imported")
	}
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Errorf("failed validation created hook source directory: %v", err)
	}
}
